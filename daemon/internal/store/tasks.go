// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// TaskStatus is the state machine for task lifecycle.
type TaskStatus string

const (
	TaskReady      TaskStatus = "ready"
	TaskClaimed    TaskStatus = "claimed"
	TaskDone       TaskStatus = "done"
	TaskFailed     TaskStatus = "failed"
	TaskDeadLetter TaskStatus = "dead_letter"
)

// TaskEntry represents an orchestrator task stored in the tasks table.
type TaskEntry struct {
	ID            string     `json:"task_id"`
	Owner         string     `json:"-"`
	ParentIDs     []string   `json:"parent_ids"`
	Status        TaskStatus `json:"status"`
	AgentType     string     `json:"agent_type"`
	AssignedAgent string     `json:"assigned_agent"`
	Goal          string     `json:"goal"`
	Context       string     `json:"context"` // JSON blob
	ResultKey     string     `json:"result_key"`
	Priority      int32      `json:"priority"`
	CreatedAt     int64      `json:"created_at"`
	ClaimedAt     int64      `json:"claimed_at"`
	CompletedAt   int64      `json:"completed_at"`
	TTLSeconds    int32      `json:"ttl_seconds"`
	RetryCount    int32      `json:"retry_count"`
	MaxRetries    int32      `json:"max_retries"`
}

// Validate returns an error if the task entry is invalid.
func (t *TaskEntry) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task id is required")
	}
	if t.Goal == "" {
		return fmt.Errorf("task goal is required")
	}
	if t.AgentType == "" {
		return fmt.Errorf("agent_type is required")
	}
	// Default status to ready
	if t.Status == "" {
		t.Status = TaskReady
	}
	switch t.Status {
	case TaskReady, TaskClaimed, TaskDone, TaskFailed, TaskDeadLetter:
	default:
		return fmt.Errorf("invalid status: %s", t.Status)
	}
	if t.TTLSeconds <= 0 {
		t.TTLSeconds = 300 // default 5 minutes
	}
	if t.Priority < 1 || t.Priority > 5 {
		t.Priority = 3 // default
	}
	if t.MaxRetries < 0 {
		t.MaxRetries = 2
	}
	return nil
}

// AddTask creates a new task row.
// Returns the created task with any defaults applied.
func (s *Store) AddTask(t *TaskEntry) (*TaskEntry, error) {
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("validate task: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if t.CreatedAt == 0 {
		t.CreatedAt = time.Now().Unix()
	}

	parentJSON, err := json.Marshal(t.ParentIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal parent_ids: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO tasks (id, owner, parent_ids, status, agent_type, assigned_agent,
			goal, context, result_key, priority, created_at, claimed_at, completed_at,
			ttl_seconds, retry_count, max_retries)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Owner, string(parentJSON), string(t.Status), t.AgentType, t.AssignedAgent,
		t.Goal, t.Context, t.ResultKey, t.Priority, t.CreatedAt, t.ClaimedAt,
		t.CompletedAt, t.TTLSeconds, t.RetryCount, t.MaxRetries)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	return t, nil
}

// GetTask retrieves a task by ID. Returns nil, nil if not found.
func (s *Store) GetTask(id string) (*TaskEntry, error) {
	t := &TaskEntry{}
	var parentJSON string
	err := s.db.QueryRow(`
		SELECT id, owner, parent_ids, status, agent_type, assigned_agent,
			goal, context, result_key, priority, created_at, claimed_at, completed_at,
			ttl_seconds, retry_count, max_retries
		FROM tasks WHERE id = ?
	`, id).Scan(&t.ID, &t.Owner, &parentJSON, &t.Status, &t.AgentType,
		&t.AssignedAgent, &t.Goal, &t.Context, &t.ResultKey, &t.Priority,
		&t.CreatedAt, &t.ClaimedAt, &t.CompletedAt, &t.TTLSeconds, &t.RetryCount,
		&t.MaxRetries)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if parentJSON != "" && parentJSON != "[]" {
		_ = json.Unmarshal([]byte(parentJSON), &t.ParentIDs)
	}
	return t, nil
}

// ClaimTask atomically claims a task if it's currently ready.
// Returns the updated task, or nil if the task was already claimed or doesn't exist.
// The caller gets nil when another agent beat them to it.
func (s *Store) ClaimTask(taskID, agentID string) (*TaskEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()
	result, err := s.db.Exec(`
		UPDATE tasks SET status = ?, assigned_agent = ?, claimed_at = ?
		WHERE id = ? AND status = ?
	`, string(TaskClaimed), agentID, now, taskID, string(TaskReady))
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, nil // task not found or not ready
	}

	return s.GetTask(taskID)
}

// CompleteTask marks a task as done and records the result key.
func (s *Store) CompleteTask(taskID, resultKey string) (*TaskEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(`
		UPDATE tasks SET status = ?, completed_at = ?, result_key = ?
		WHERE id = ?
	`, string(TaskDone), now, resultKey, taskID)
	if err != nil {
		return nil, fmt.Errorf("complete task: %w", err)
	}

	return s.GetTask(taskID)
}

// FailTask marks a task as failed and increments retry count.
// If retries remaining, resets to ready. Otherwise, dead_letter.
func (s *Store) FailTask(taskID string) (*TaskEntry, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(`
		UPDATE tasks SET
			status = CASE
				WHEN retry_count + 1 >= max_retries THEN ?
				ELSE ?
			END,
			retry_count = retry_count + 1,
			claimed_at = 0,
			assigned_agent = '',
			completed_at = ?
		WHERE id = ? AND status = ?
	`, string(TaskDeadLetter), string(TaskReady), now, taskID, string(TaskClaimed))
	if err != nil {
		return nil, fmt.Errorf("fail task: %w", err)
	}

	return s.GetTask(taskID)
}

// FindReadyTasks returns tasks with status=ready, filtered by agent_type if non-empty.
// Falls back to FTS5 prefix matching when exact agent_type match returns nothing.
func (s *Store) FindReadyTasks(agentType string, limit int) ([]*TaskEntry, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, owner, parent_ids, status, agent_type, assigned_agent,
		goal, context, result_key, priority, created_at, claimed_at, completed_at,
		ttl_seconds, retry_count, max_retries
		FROM tasks WHERE status = ?`
	args := []any{string(TaskReady)}

	if agentType != "" {
		query += " AND agent_type = ?"
		args = append(args, agentType)
	}

	query += " ORDER BY priority DESC, created_at ASC LIMIT ?"
	args = append(args, limit)

	tasks, err := s.queryTasks(query, args...)
	if err != nil {
		return nil, err
	}

	// FTS5 fuzzy fallback — if exact match returned nothing, try prefix search
	if len(tasks) == 0 && agentType != "" {
		ftsQuery := `SELECT id, owner, parent_ids, status, agent_type, assigned_agent,
			goal, context, result_key, priority, created_at, claimed_at, completed_at,
			ttl_seconds, retry_count, max_retries
			FROM tasks WHERE status = ? AND rowid IN (
				SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH ?
			)
			ORDER BY priority DESC, created_at ASC LIMIT ?`
		ftsArgs := []any{string(TaskReady), fmt.Sprintf(`agent_type:"%s"*`, agentType), limit}
		tasks, err = s.queryTasks(ftsQuery, ftsArgs...)
		if err != nil {
			slog.Debug("FTS5 fuzzy fallback failed, returning empty", "agent_type", agentType, "error", err)
			return nil, nil // original exact query already returned empty; propagate gracefully
		}
	}

	return tasks, nil
}

// ListTasksByStatus returns tasks filtered by status (or all if empty).
func (s *Store) ListTasksByStatus(status TaskStatus, limit int) ([]*TaskEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, owner, parent_ids, status, agent_type, assigned_agent,
		goal, context, result_key, priority, created_at, claimed_at, completed_at,
		ttl_seconds, retry_count, max_retries
		FROM tasks`
	args := []any{}

	if status != "" {
		query += " WHERE status = ?"
		args = append(args, string(status))
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	return s.queryTasks(query, args...)
}

// ResetTimedOutTasks resets claimed tasks past their TTL back to ready.
// Returns the list of IDs that were reset.
func (s *Store) ResetTimedOutTasks() ([]string, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	// Find timed-out tasks
	rows, err := s.db.Query(`
		SELECT id FROM tasks
		WHERE status = ? AND claimed_at > 0 AND claimed_at + ttl_seconds < ?
	`, string(TaskClaimed), now)
	if err != nil {
		return nil, fmt.Errorf("find timed-out tasks: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan timed-out task: %w", err)
		}
		ids = append(ids, id)
	}

	if len(ids) > 0 {
		_, err = s.db.Exec(`
			UPDATE tasks SET status = ?, claimed_at = 0, assigned_agent = ''
			WHERE status = ? AND claimed_at > 0 AND claimed_at + ttl_seconds < ?
		`, string(TaskReady), string(TaskClaimed), now)
		if err != nil {
			return nil, fmt.Errorf("reset timed-out tasks: %w", err)
		}
	}

	return ids, nil
}

// DeleteTask removes a task by ID.
func (s *Store) DeleteTask(id string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("delete task: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (s *Store) queryTasks(query string, args ...any) ([]*TaskEntry, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*TaskEntry
	for rows.Next() {
		t := &TaskEntry{}
		var parentJSON string
		if err := rows.Scan(&t.ID, &t.Owner, &parentJSON, &t.Status, &t.AgentType,
			&t.AssignedAgent, &t.Goal, &t.Context, &t.ResultKey, &t.Priority,
			&t.CreatedAt, &t.ClaimedAt, &t.CompletedAt, &t.TTLSeconds,
			&t.RetryCount, &t.MaxRetries); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		if parentJSON != "" && parentJSON != "[]" {
			_ = json.Unmarshal([]byte(parentJSON), &t.ParentIDs)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task rows: %w", err)
	}
	return tasks, nil
}
