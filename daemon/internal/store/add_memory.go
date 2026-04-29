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
	Target      string
	Category    string
	Key         string
	Content     string
	Priority    int32
	SourceAgent string
	ExpiresAt   int64
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
	err = tx.QueryRow("SELECT id FROM memories WHERE target = ? AND key = ?", params.Target, params.Key).Scan(&existingID)
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
	_, err = tx.Exec(`
		INSERT INTO memories (id, target, category, key, content, priority, source_agent, created_at, updated_at, accessed_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, params.Target, params.Category, params.Key, params.Content, params.Priority, params.SourceAgent, now, now, now, params.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit insert: %w", err)
	}
	return s.GetMemoryByID(id)
}