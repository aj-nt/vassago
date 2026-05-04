// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

// addMemory is a test helper that wraps the positional args into AddMemoryParams.
// This keeps test calls readable while the production API uses the params struct.
func addMemory(s *Store, target, category, key, content string, priority int32, sourceAgent string, expiresAt int64) (*MemoryEntry, error) {
	return s.AddMemory(AddMemoryParams{
		Target:      target,
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: sourceAgent,
		ExpiresAt:   expiresAt,
	})
}