// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

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
	Name        string
	Description string
	Triggers    []string
	Category    string
	Content     string
	SourceAgent string
	Priority    int32
}

// AddSkill creates a new skill entry.
func (s *Store) AddSkill(params AddSkillParams) (*SkillEntry, error) {
	if params.Name == "" || params.Description == "" || len(params.Triggers) == 0 || 
	   params.Category == "" || params.Content == "" || params.SourceAgent == "" {
		return nil, fmt.Errorf("name, description, triggers, category, content, and source_agent are required")
	}

	if params.Priority < 1 || params.Priority > 5 {
		params.Priority = 3 // Default priority
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	// Convert triggers to JSON
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
		INSERT INTO skills (id, name, description, triggers, category, content, source_agent, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.Name, sanitizedDescription, string(triggersJSON), params.Category, sanitizedContent, 
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

// GetSkillByID retrieves a skill by its ID.
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

	// Parse triggers JSON
	if err := json.Unmarshal([]byte(triggersJSON), &entry.Triggers); err != nil {
		return nil, fmt.Errorf("unmarshal triggers: %w", err)
	}

	return &entry, nil
}

// ListSkills retrieves skills with optional filtering.
func (s *Store) ListSkills(category, sourceAgent string, limit int) ([]*SkillEntry, error) {
	query := "SELECT id, name, description, triggers, category, content, source_agent, priority, created_at, updated_at FROM skills"
	var args []interface{}

	whereClauses := []string{}
	if category != "" {
		whereClauses = append(whereClauses, "category = ?")
		args = append(args, category)
	}
	if sourceAgent != "" {
		whereClauses = append(whereClauses, "source_agent = ?")
		args = append(args, sourceAgent)
	}

	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()

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

		// Parse triggers JSON
		if err := json.Unmarshal([]byte(triggersJSON), &entry.Triggers); err != nil {
			return nil, fmt.Errorf("unmarshal triggers: %w", err)
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// RemoveSkill deletes a skill by its ID.
func (s *Store) RemoveSkill(id string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec("DELETE FROM skills WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("remove skill: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

// SearchSkills searches skills using FTS5.
func (s *Store) SearchSkills(query, category string, limit int) ([]*SkillEntry, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	ftsQuery := query + "*" // Add wildcard for prefix matching
	args := []interface{}{ftsQuery}

	whereClauses := []string{"skills_fts MATCH ?"}
	if category != "" {
		whereClauses = append(whereClauses, "s.category = ?")
		args = append(args, category)
	}

	q := fmt.Sprintf(
		"SELECT s.id, s.name, s.description, s.triggers, s.category, s.content, s.source_agent, s.priority, s.created_at, s.updated_at " +
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
	defer rows.Close()

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

		// Parse triggers JSON
		if err := json.Unmarshal([]byte(triggersJSON), &entry.Triggers); err != nil {
			return nil, fmt.Errorf("unmarshal triggers: %w", err)
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}