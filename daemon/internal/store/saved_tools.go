// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SavedToolRow is the database representation of a saved tool.
type SavedToolRow struct {
	ID                        string `json:"id"`
	Owner                     string `json:"owner"`
	Name                      string `json:"name"`
	Description               string `json:"description"`
	Type                      string `json:"type"`
	Parameters                string `json:"parameters"`
	Script                    string `json:"script"`
	Category                  string `json:"category"`
	Triggers                  string `json:"triggers"` // JSON array
	Author                    string `json:"author"`
	Priority                  int    `json:"priority"`
	Version                   int    `json:"version"`
	RequiresApprovalEveryCall bool   `json:"requires_approval_every_call"`
	CreatedAt                 int64  `json:"created_at"`
	UpdatedAt                 int64  `json:"updated_at"`
}

// AddSavedTool inserts a new saved tool, scoped to owner (Phase 28).
func (s *Store) AddSavedTool(owner, name, description, toolType, parameters, script, category string, triggers []string, author string, priority int) (*SavedToolRow, error) {
	id := uuid.New().String()
	now := time.Now().Unix()
	if owner == "" {
		owner = "default"
	}

	triggersJSON, err := json.Marshal(triggers)
	if err != nil {
		return nil, fmt.Errorf("marshal triggers: %w", err)
	}

	row := &SavedToolRow{
		ID:                        id,
		Owner:                     owner,
		Name:                      name,
		Description:               description,
		Type:                      toolType,
		Parameters:                parameters,
		Script:                    script,
		Category:                  category,
		Triggers:                  string(triggersJSON),
		Author:                    author,
		Priority:                  priority,
		Version:                   1,
		RequiresApprovalEveryCall: true, // safe default
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}

	_, err = s.db.Exec(
		`INSERT INTO saved_tools (id, owner, name, description, type, parameters, script, category, triggers, author, priority, version, requires_approval_every_call, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.Owner, row.Name, row.Description, row.Type, row.Parameters, row.Script,
		row.Category, row.Triggers, row.Author, row.Priority, row.Version,
		boolToInt(row.RequiresApprovalEveryCall), row.CreatedAt, row.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert saved_tool: %w", err)
	}

	return row, nil
}

// GetSavedTool retrieves a saved tool by ID (no owner scoping — internal use).
func (s *Store) GetSavedTool(id string) (*SavedToolRow, error) {
	row := &SavedToolRow{}
	var requiresInt int
	err := s.db.QueryRow(
		`SELECT id, name, description, type, parameters, script, category, triggers, author, priority, version, requires_approval_every_call, created_at, updated_at
		 FROM saved_tools WHERE id = ?`, id,
	).Scan(&row.ID, &row.Name, &row.Description, &row.Type, &row.Parameters, &row.Script,
		&row.Category, &row.Triggers, &row.Author, &row.Priority, &row.Version,
		&requiresInt, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get saved_tool %s: %w", id, err)
	}
	row.RequiresApprovalEveryCall = requiresInt != 0
	return row, nil
}

// ListSavedTools returns saved tools, optionally filtered by category or author, scoped to owner (Phase 28).
func (s *Store) ListSavedTools(owner, category, author string, limit int) ([]SavedToolRow, error) {
	if owner == "" {
		owner = "default"
	}

	query := `SELECT id, name, description, type, parameters, script, category, triggers, author, priority, version, requires_approval_every_call, created_at, updated_at
		FROM saved_tools WHERE owner = ?`
	args := []interface{}{owner}

	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if author != "" {
		query += " AND author = ?"
		args = append(args, author)
	}
	query += " ORDER BY priority DESC, created_at DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list saved_tools: %w", err)
	}
	defer rows.Close()

	var result []SavedToolRow
	for rows.Next() {
		var row SavedToolRow
		var requiresInt int
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &row.Type, &row.Parameters, &row.Script,
			&row.Category, &row.Triggers, &row.Author, &row.Priority, &row.Version,
			&requiresInt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan saved_tool: %w", err)
		}
		row.RequiresApprovalEveryCall = requiresInt != 0
		result = append(result, row)
	}
	return result, rows.Err()
}

// SearchSavedTools performs FTS5 search on saved tools, scoped to owner (Phase 28).
func (s *Store) SearchSavedTools(owner, query, category string, limit int) ([]SavedToolRow, error) {
	if owner == "" {
		owner = "default"
	}

	sql := `SELECT st.id, st.name, st.description, st.type, st.parameters, st.script, st.category, st.triggers, st.author, st.priority, st.version, st.requires_approval_every_call, st.created_at, st.updated_at
		FROM saved_tools st
		JOIN saved_tools_fts fts ON st.rowid = fts.rowid
		WHERE saved_tools_fts MATCH ? AND st.owner = ?`
	args := []interface{}{query, owner}

	if category != "" {
		sql += " AND st.category = ?"
		args = append(args, category)
	}
	sql += " ORDER BY rank LIMIT ?"
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit)

	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search saved_tools: %w", err)
	}
	defer rows.Close()

	var result []SavedToolRow
	for rows.Next() {
		var row SavedToolRow
		var requiresInt int
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &row.Type, &row.Parameters, &row.Script,
			&row.Category, &row.Triggers, &row.Author, &row.Priority, &row.Version,
			&requiresInt, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan saved_tool: %w", err)
		}
		row.RequiresApprovalEveryCall = requiresInt != 0
		result = append(result, row)
	}
	return result, rows.Err()
}

// UpdateSavedTool updates fields on a saved tool, scoped to owner (Phase 28). Increments version.
func (s *Store) UpdateSavedTool(owner, id string, updates map[string]interface{}) (*SavedToolRow, error) {
	if owner == "" {
		owner = "default"
	}

	current, err := s.GetSavedTool(id)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	newVersion := current.Version + 1

	setClauses := []string{"version = ?", "updated_at = ?"}
	args := []interface{}{newVersion, now}

	if v, ok := updates["name"].(string); ok && v != "" {
		setClauses = append(setClauses, "name = ?")
		args = append(args, v)
		current.Name = v
	}
	if v, ok := updates["description"].(string); ok && v != "" {
		setClauses = append(setClauses, "description = ?")
		args = append(args, v)
		current.Description = v
	}
	if v, ok := updates["parameters"].(string); ok && v != "" {
		setClauses = append(setClauses, "parameters = ?")
		args = append(args, v)
		current.Parameters = v
	}
	if v, ok := updates["script"].(string); ok && v != "" {
		setClauses = append(setClauses, "script = ?")
		args = append(args, v)
		current.Script = v
	}
	if v, ok := updates["category"].(string); ok && v != "" {
		setClauses = append(setClauses, "category = ?")
		args = append(args, v)
		current.Category = v
	}
	if v, ok := updates["triggers"].([]string); ok {
		triggersJSON, _ := json.Marshal(v)
		setClauses = append(setClauses, "triggers = ?")
		args = append(args, string(triggersJSON))
		current.Triggers = string(triggersJSON)
	}
	if v, ok := updates["priority"].(int); ok && v != 0 {
		setClauses = append(setClauses, "priority = ?")
		args = append(args, v)
		current.Priority = v
	}
	if v, ok := updates["requires_approval_every_call"].(bool); ok {
		setClauses = append(setClauses, "requires_approval_every_call = ?")
		args = append(args, boolToInt(v))
		current.RequiresApprovalEveryCall = v
	}

	if len(setClauses) == 2 {
		return current, nil
	}

	sql := "UPDATE saved_tools SET " + joinClauses(setClauses) + " WHERE id = ? AND owner = ?"
	args = append(args, id, owner)

	if _, err := s.db.Exec(sql, args...); err != nil {
		return nil, fmt.Errorf("update saved_tool %s: %w", id, err)
	}

	current.Version = newVersion
	current.UpdatedAt = now
	return current, nil
}

// RemoveSavedTool deletes a saved tool by ID, scoped to owner (Phase 28).
func (s *Store) RemoveSavedTool(owner, id string) (bool, error) {
	if owner == "" {
		owner = "default"
	}

	result, err := s.db.Exec("DELETE FROM saved_tools WHERE id = ? AND owner = ?", id, owner)
	if err != nil {
		return false, fmt.Errorf("remove saved_tool %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func joinClauses(clauses []string) string {
	result := clauses[0]
	for _, c := range clauses[1:] {
		result += ", " + c
	}
	return result
}
