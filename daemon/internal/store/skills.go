// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SkillEntry represents a single skill record.
type SkillEntry struct {
	ID          string
	Name        string
	Description string
	Triggers    []string
	Category    string
	Content     string
	SourceAgent string
	Priority    int32
	CreatedAt   int64 // Unix timestamp
	UpdatedAt   int64
}

// AddSkillParams holds the parameters for AddSkill.
type AddSkillParams struct {
	Owner       string
	Name        string
	Description string
	Triggers    []string
	Category    string
	Content     string
	SourceAgent string
	Priority    int32
}

// AddSkill creates a new skill entry, scoped to owner (Phase 28).
func (s *Store) AddSkill(params AddSkillParams) (*SkillEntry, error) {
	if params.Name == "" || params.Description == "" || len(params.Triggers) == 0 ||
		params.Category == "" || params.Content == "" || params.SourceAgent == "" {
		return nil, fmt.Errorf("name, description, triggers, category, content, and source_agent are required")
	}
	if params.Priority < 1 || params.Priority > 5 {
		params.Priority = 3
	}
	if params.Owner == "" {
		params.Owner = "default"
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	triggersJSON, err := json.Marshal(params.Triggers)
	if err != nil {
		return nil, fmt.Errorf("marshal triggers: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id := uuid.New().String()
	sanitizedContent := sanitizeUTF8(params.Content)
	sanitizedDescription := sanitizeUTF8(params.Description)

	_, err = tx.Exec(`
		INSERT INTO skills (id, owner, name, description, triggers, category, content, source_agent, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.Owner, params.Name, sanitizedDescription, string(triggersJSON), params.Category, sanitizedContent,
		params.SourceAgent, params.Priority, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert skill: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return s.GetSkillByID(id)
}

// GetSkillByID retrieves a skill by its ID (no owner scoping — internal use).
func (s *Store) GetSkillByID(id string) (*SkillEntry, error) {
	var entry SkillEntry
	var triggersJSON string

	err := s.db.QueryRow(`
		SELECT id, name, description, triggers, category, content, source_agent, priority, created_at, updated_at
		FROM skills WHERE id = ?`, id).Scan(
		&entry.ID, &entry.Name, &entry.Description, &triggersJSON, &entry.Category,
		&entry.Content, &entry.SourceAgent, &entry.Priority, &entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("skill not found: %s", id)
		}
		return nil, fmt.Errorf("get skill: %w", err)
	}

	if err := json.Unmarshal([]byte(triggersJSON), &entry.Triggers); err != nil {
		return nil, fmt.Errorf("unmarshal triggers: %w", err)
	}

	return &entry, nil
}

// ListSkills retrieves skills with optional filtering, scoped to owner (Phase 28).
func (s *Store) ListSkills(owner, category, sourceAgent string, limit int) ([]*SkillEntry, error) {
	if owner == "" {
		owner = "default"
	}

	query := "SELECT id, name, description, triggers, category, content, source_agent, priority, created_at, updated_at FROM skills WHERE owner = ?"
	args := []interface{}{owner}

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if sourceAgent != "" {
		query += " AND source_agent = ?"
		args = append(args, sourceAgent)
	}

	query += " ORDER BY priority DESC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanSkills(rows)
}

// RemoveSkill deletes a skill by its ID, scoped to owner (Phase 28).
func (s *Store) RemoveSkill(owner, id string) (bool, error) {
	if owner == "" {
		owner = "default"
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec("DELETE FROM skills WHERE id = ? AND owner = ?", id, owner)
	if err != nil {
		return false, fmt.Errorf("remove skill: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

// SearchSkills searches skills using FTS5, scoped to owner (Phase 28).
func (s *Store) SearchSkills(owner, query, category string, limit int) ([]*SkillEntry, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if owner == "" {
		owner = "default"
	}

	ftsQuery := query + "*"
	args := []interface{}{ftsQuery, owner}

	whereClauses := []string{"skills_fts MATCH ?", "s.owner = ?"}
	if category != "" {
		whereClauses = append(whereClauses, "s.category = ?")
		args = append(args, category)
	}

	q := fmt.Sprintf(
		"SELECT s.id, s.name, s.description, s.triggers, s.category, s.content, s.source_agent, s.priority, s.created_at, s.updated_at "+
			"FROM skills s JOIN skills_fts ON s.rowid = skills_fts.rowid WHERE %s ORDER BY skills_fts.rank",
		strings.Join(whereClauses, " AND "),
	)

	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search skills: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanSkills(rows)
}

func scanSkills(rows *sql.Rows) ([]*SkillEntry, error) {
	var entries []*SkillEntry
	for rows.Next() {
		var entry SkillEntry
		var triggersJSON string

		if err := rows.Scan(
			&entry.ID, &entry.Name, &entry.Description, &triggersJSON, &entry.Category,
			&entry.Content, &entry.SourceAgent, &entry.Priority, &entry.CreatedAt, &entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}

		if err := json.Unmarshal([]byte(triggersJSON), &entry.Triggers); err != nil {
			return nil, fmt.Errorf("unmarshal triggers: %w", err)
		}

		entries = append(entries, &entry)
	}
	return entries, nil
}
