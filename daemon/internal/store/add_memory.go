// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AddMemoryParams holds the parameters for AddMemory.
// Introduced to improve readability — 7 positional parameters is past the Fowler threshold.
type AddMemoryParams struct {
	Owner       string
	Target      string
	Category    string
	Key         string
	Content     string
	Priority    int32
	SourceAgent string
	ExpiresAt   int64
}

// MemoryPatch holds fields for partial memory update.
// Nil pointers mean "keep existing value"; non-nil means "update to this value".
type MemoryPatch struct {
	Category  *string
	Content   *string
	Priority  *int32
	ExpiresAt *int64
}

// AddMemory creates a new memory entry. If target+key already exists, it updates instead.
// The operation is atomic — the check-then-upsert is wrapped in a transaction.
func (s *Store) AddMemory(params AddMemoryParams) (*MemoryEntry, error) {
	if err := validateMemoryInput(params.Target, params.Category, params.Key, params.Content, params.Priority); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if params.SourceAgent == "" {
		return nil, fmt.Errorf("source_agent must not be empty")
	}
	if params.Owner == "" {
		params.Owner = "default"
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	// Transaction for atomic upsert
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Check if entry exists with this target+key
	var existingID string
	err = tx.QueryRow("SELECT id FROM memories WHERE target = ? AND key = ? AND owner = ?", params.Target, params.Key, params.Owner).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("check existing: %w", err)
	}

	if existingID != "" {
		// Update existing entry
		_, err = tx.Exec(`
			UPDATE memories SET
				content = ?,
				priority = ?,
				category = ?,
				source_agent = ?,
				updated_at = ?,
				expires_at = ?
			WHERE id = ?`,
			params.Content, params.Priority, params.Category, params.SourceAgent, now, params.ExpiresAt, existingID,
		)
		if err != nil {
			return nil, fmt.Errorf("update memory: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit update: %w", err)
		}
		return s.GetMemoryByID(existingID)
	}

	// Create new entry
	id := uuid.New().String()
	sanitizedContent := sanitizeUTF8(params.Content)
	_, err = tx.Exec(`
		INSERT INTO memories (id, owner, target, category, key, content, priority, source_agent, created_at, updated_at, accessed_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.Owner, params.Target, params.Category, params.Key, sanitizedContent, params.Priority, params.SourceAgent, now, now, now, params.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit insert: %w", err)
	}
	return s.GetMemoryByID(id)
}

// UpdateMemory applies a partial update to an existing memory entry.
// It reads the existing entry, merges the patch, and re-upserts.
func (s *Store) UpdateMemory(id, owner string, patch MemoryPatch) (*MemoryEntry, error) {
	existing, err := s.GetMemoryByID(id)
	if err != nil {
		return nil, err
	}

	// Apply partial updates
	category := existing.Category
	content := existing.Content
	priority := existing.Priority
	expiresAt := existing.ExpiresAt

	if patch.Category != nil {
		category = *patch.Category
	}
	if patch.Content != nil {
		content = *patch.Content
	}
	if patch.Priority != nil {
		priority = *patch.Priority
	}
	if patch.ExpiresAt != nil {
		expiresAt = *patch.ExpiresAt
	}

	return s.AddMemory(AddMemoryParams{
		Owner:       owner,
		Target:      existing.Target,
		Category:    category,
		Key:         existing.Key,
		Content:     content,
		Priority:    priority,
		SourceAgent: existing.SourceAgent,
		ExpiresAt:   expiresAt,
	})
}