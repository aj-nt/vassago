// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"database/sql"
	"fmt"
	"time"

	pb "github.com/aj-nt/vassago/proto"
)

// AddCronJob creates a new cron job, scoped to owner (Phase 28).
func (s *Store) AddCronJob(owner, name, schedule, agentID, message string, enabled bool) (*pb.CronJob, error) {
	id := generateID()
	now := time.Now().Unix()
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	if owner == "" {
		owner = "default"
	}

	_, err := s.db.Exec(
		`INSERT INTO cron_jobs (id, owner, name, schedule, agent_id, message, enabled, created_at, updated_at, last_fire_at, next_fire_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		id, owner, name, schedule, agentID, message, enabledInt, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("AddCronJob: %w", err)
	}

	return &pb.CronJob{
		Id:         id,
		Name:       name,
		Schedule:   schedule,
		AgentId:    agentID,
		Message:    message,
		Enabled:    enabled,
		CreatedAt:  now,
		UpdatedAt:  now,
		LastFireAt: 0,
		NextFireAt: 0,
	}, nil
}

// GetCronJob retrieves a single cron job by ID (no owner scoping — used by cron engine).
func (s *Store) GetCronJob(id string) (*pb.CronJob, error) {
	var job pb.CronJob
	var enabled bool
	var lastFireAt sql.NullInt64
	var nextFireAt sql.NullInt64

	err := s.db.QueryRow(
		`SELECT id, name, schedule, agent_id, message, enabled, created_at, updated_at, last_fire_at, next_fire_at
		 FROM cron_jobs WHERE id = ?`, id,
	).Scan(&job.Id, &job.Name, &job.Schedule, &job.AgentId, &job.Message, &enabled,
		&job.CreatedAt, &job.UpdatedAt, &lastFireAt, &nextFireAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("GetCronJob: cron job %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("GetCronJob: %w", err)
	}

	job.Enabled = enabled
	if lastFireAt.Valid {
		job.LastFireAt = lastFireAt.Int64
	}
	if nextFireAt.Valid {
		job.NextFireAt = nextFireAt.Int64
	}
	return &job, nil
}

// ListCronJobs returns cron jobs, optionally filtered by agent and owner (Phase 28).
func (s *Store) ListCronJobs(owner, agentID string, includeDisabled bool) ([]*pb.CronJob, error) {
	if owner == "" {
		owner = "default"
	}

	query := `SELECT id, name, schedule, agent_id, message, enabled, created_at, updated_at, last_fire_at, next_fire_at
			FROM cron_jobs WHERE owner = ?`
	args := []interface{}{owner}

	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if !includeDisabled {
		query += " AND enabled = 1"
	}
	query += " ORDER BY created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListCronJobs: %w", err)
	}
	defer rows.Close()

	var jobs []*pb.CronJob
	for rows.Next() {
		var job pb.CronJob
		var enabled bool
		var lastFireAt sql.NullInt64
		var nextFireAt sql.NullInt64
		if err := rows.Scan(&job.Id, &job.Name, &job.Schedule, &job.AgentId, &job.Message, &enabled,
			&job.CreatedAt, &job.UpdatedAt, &lastFireAt, &nextFireAt); err != nil {
			return nil, fmt.Errorf("ListCronJobs scan: %w", err)
		}
		job.Enabled = enabled
		if lastFireAt.Valid {
			job.LastFireAt = lastFireAt.Int64
		}
		if nextFireAt.Valid {
			job.NextFireAt = nextFireAt.Int64
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}

// UpdateCronJob updates specific fields of a cron job, scoped to owner (Phase 28).
func (s *Store) UpdateCronJob(owner, id string, name, schedule, message *string, enabled *bool) (*pb.CronJob, error) {
	now := time.Now().Unix()
	if owner == "" {
		owner = "default"
	}

	setClauses := []string{"updated_at = ?"}
	args := []interface{}{now}

	if name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *name)
	}
	if schedule != nil {
		setClauses = append(setClauses, "schedule = ?")
		args = append(args, *schedule)
	}
	if message != nil {
		setClauses = append(setClauses, "message = ?")
		args = append(args, *message)
	}
	if enabled != nil {
		enabledInt := 0
		if *enabled {
			enabledInt = 1
		}
		setClauses = append(setClauses, "enabled = ?")
		args = append(args, enabledInt)
	}

	args = append(args, id, owner)
	query := fmt.Sprintf("UPDATE cron_jobs SET %s WHERE id = ? AND owner = ?", joinSetClauses(setClauses))

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("UpdateCronJob: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return nil, fmt.Errorf("UpdateCronJob: cron job %s not found", id)
	}

	return s.GetCronJob(id)
}

// DeleteCronJob removes a cron job, scoped to owner (Phase 28).
func (s *Store) DeleteCronJob(owner, id string) error {
	if owner == "" {
		owner = "default"
	}

	result, err := s.db.Exec(`DELETE FROM cron_jobs WHERE id = ? AND owner = ?`, id, owner)
	if err != nil {
		return fmt.Errorf("DeleteCronJob: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("DeleteCronJob: cron job %s not found", id)
	}
	return nil
}

// UpdateCronJobFireTimestamps updates last_fire_at and next_fire_at for a job (no owner scoping — cron engine).
func (s *Store) UpdateCronJobFireTimestamps(id string, lastFireAt, nextFireAt int64) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`UPDATE cron_jobs SET last_fire_at = ?, next_fire_at = ?, updated_at = ? WHERE id = ?`,
		lastFireAt, nextFireAt, now, id,
	)
	return err
}

// GetDueCronJobs returns all enabled cron jobs whose next_fire_at is <= now (no owner scoping — cron engine).
func (s *Store) GetDueCronJobs(now int64) ([]*pb.CronJob, error) {
	rows, err := s.db.Query(
		`SELECT id, name, schedule, agent_id, message, enabled, created_at, updated_at, last_fire_at, next_fire_at
		 FROM cron_jobs WHERE enabled = 1 AND next_fire_at > 0 AND next_fire_at <= ?
		 ORDER BY next_fire_at ASC`, now,
	)
	if err != nil {
		return nil, fmt.Errorf("GetDueCronJobs: %w", err)
	}
	defer rows.Close()

	var jobs []*pb.CronJob
	for rows.Next() {
		var job pb.CronJob
		var enabled bool
		var lastFireAt sql.NullInt64
		var nextFireAt sql.NullInt64
		if err := rows.Scan(&job.Id, &job.Name, &job.Schedule, &job.AgentId, &job.Message, &enabled,
			&job.CreatedAt, &job.UpdatedAt, &lastFireAt, &nextFireAt); err != nil {
			return nil, fmt.Errorf("GetDueCronJobs scan: %w", err)
		}
		job.Enabled = enabled
		if lastFireAt.Valid {
			job.LastFireAt = lastFireAt.Int64
		}
		if nextFireAt.Valid {
			job.NextFireAt = nextFireAt.Int64
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}

// joinSetClauses joins SET clause fragments with commas.
func joinSetClauses(clauses []string) string {
	result := clauses[0]
	for _, c := range clauses[1:] {
		result += ", " + c
	}
	return result
}
