// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"github.com/aj-nt/mnemosyne/daemon/internal/extraction"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

// extractionStoreAdapter adapts store.Store to extraction.MemoryStore.
type extractionStoreAdapter struct {
	store *store.Store
}

func newExtractionStore(st *store.Store) extraction.MemoryStore {
	return &extractionStoreAdapter{store: st}
}

func (a *extractionStoreAdapter) AddMemory(owner, target, category, key, content string, priority int32, sourceAgent string) (string, error) {
	entry, err := a.store.AddMemory(store.AddMemoryParams{
		Owner:       owner,
		Target:      target,
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: sourceAgent,
	})
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

func (a *extractionStoreAdapter) ListMemoryKeys(owner string) ([]string, error) {
	// List all memories for this owner, then extract just the keys
	entries, err := a.store.ListMemories(owner, "", "", 1)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(entries))
	for i, e := range entries {
		keys[i] = e.Key
	}
	return keys, nil
}
