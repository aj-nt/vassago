// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"encoding/json"
	"fmt"
	"os"
)

// MemoryExport is the JSON-serializable format for a single memory entry.
// Used for backup/export and import/restore operations.
type MemoryExport struct {
	ID          string `json:"id"`
	Owner       string `json:"owner"`
	Target      string `json:"target"`
	Category    string `json:"category"`
	Key         string `json:"key"`
	Content     string `json:"content"`
	Priority    int32  `json:"priority"`
	SourceAgent string `json:"source_agent"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	AccessedAt  int64  `json:"accessed_at"`
	ExpiresAt   int64  `json:"expires_at"`
}

// ImportResult summarizes the result of an import operation.
type ImportResult struct {
	Imported int
	Skipped  int
	Errors   []string
}

// ExportJSON writes all memories to a JSON file.
// Returns the number of entries exported.
func (s *Store) ExportJSON(path string) (int, error) {
	entries, err := s.ListMemories("default", "", "", 0)
	if err != nil {
		return 0, fmt.Errorf("list memories for export: %w", err)
	}

	exports := make([]MemoryExport, len(entries))
	for i, e := range entries {
		exports[i] = MemoryExport{
			ID:          e.ID,
			Owner:       e.Owner,
			Target:      e.Target,
			Category:    e.Category,
			Key:          e.Key,
			Content:     e.Content,
			Priority:    e.Priority,
			SourceAgent: e.SourceAgent,
			CreatedAt:   e.CreatedAt,
			UpdatedAt:   e.UpdatedAt,
			AccessedAt:  e.AccessedAt,
			ExpiresAt:   e.ExpiresAt,
		}
	}

	data, err := json.MarshalIndent(exports, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshal export: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return 0, fmt.Errorf("write export file: %w", err)
	}

	return len(exports), nil
}

// ImportJSON reads memories from a JSON file and imports them into the store.
// If skipExisting is true, entries with the same target+key are skipped.
// If skipExisting is false, existing entries are updated (upsert semantics).
func (s *Store) ImportJSON(path string, skipExisting bool) (*ImportResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read import file: %w", err)
	}

	var exports []MemoryExport
	if err := json.Unmarshal(data, &exports); err != nil {
		return nil, fmt.Errorf("parse import file: %w", err)
	}

	result := &ImportResult{}

	// If skipExisting, build a set of existing target+key pairs
	var existingKeys map[string]bool
	if skipExisting {
		existingKeys = make(map[string]bool)
		entries, err := s.ListMemories("default", "", "", 0)
		if err != nil {
			return nil, fmt.Errorf("list existing memories for skip check: %w", err)
		}
		for _, e := range entries {
			existingKeys[e.Target+"|"+e.Key] = true
		}
	}

	for _, e := range exports {
		// Validate required fields
		if e.Key == "" || e.Content == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("skipped entry with id=%s: empty key or content", e.ID))
			result.Skipped++
			continue
		}

		// Skip if existing and requested
		if skipExisting && existingKeys[e.Target+"|"+e.Key] {
			result.Skipped++
			continue
		}

		// Use AddMemory's upsert semantics
		_, err := s.AddMemory(AddMemoryParams{
			Owner:       e.Owner,
			Target:      e.Target,
			Category:    e.Category,
			Key:          e.Key,
			Content:     e.Content,
			Priority:    e.Priority,
			SourceAgent: e.SourceAgent,
			ExpiresAt:   e.ExpiresAt,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("import entry id=%s key=%s: %v", e.ID, e.Key, err))
			result.Skipped++
			continue
		}
		result.Imported++
	}

	return result, nil
}