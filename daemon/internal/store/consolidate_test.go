// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func addTestMemory(t *testing.T, s *Store, target, category, key, content string, priority int32, expiresAt int64) *MemoryEntry {
	t.Helper()
	entry, err := s.AddMemory(AddMemoryParams{
		Target:      target,
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: "test-agent",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("add test memory: %v", err)
	}
	return entry
}

func TestGetConsolidationCandidates_Expired(t *testing.T) {
	s := testStore(t)

	// Add an expired memory (expires_at in the past)
	addTestMemory(t, s, "memory", "environment", "old-server", "old content", 3, 1000) // expires_at = 1000 (Jan 1 1970)

	// Add a non-expired memory (expires_at = 0 = no expiry)
	addTestMemory(t, s, "memory", "environment", "current-server", "current content", 3, 0)

	// Add a future-expired memory (expires_at far in future)
	addTestMemory(t, s, "memory", "environment", "future-server", "future content", 3, 9999999999)

	candidates, err := s.GetConsolidationCandidates(ConsolidateScopeExpired, "", 10)
	if err != nil {
		t.Fatalf("GetConsolidationCandidates expired: %v", err)
	}
	if len(candidates) != 1 {
		t.Errorf("expected 1 expired candidate, got %d", len(candidates))
	} else if candidates[0].Entry.Key != "old-server" {
		t.Errorf("expected key 'old-server', got %s", candidates[0].Entry.Key)
	}
}

func TestGetConsolidationCandidates_Idle(t *testing.T) {
	s := testStore(t)

	// Add a memory and set accessed_at to 31+ days ago via SQL
	entry := addTestMemory(t, s, "memory", "project", "old-project", "old stuff", 2, 0)
	_, err := s.DB().Exec("UPDATE memories SET accessed_at = ? WHERE id = ?", 1000, entry.ID) //nolint:errcheck
	if err != nil {
		t.Fatalf("update accessed_at: %v", err)
	}

	// Add a recently accessed memory (accessed_at is current time by default)
	addTestMemory(t, s, "memory", "project", "recent-project", "recent stuff", 2, 0)

	candidates, err := s.GetConsolidationCandidates(ConsolidateScopeIdle, "", 10)
	if err != nil {
		t.Fatalf("GetConsolidationCandidates idle: %v", err)
	}
	if len(candidates) < 1 {
		t.Errorf("expected at least 1 idle candidate, got %d", len(candidates))
	}
}

func TestGetConsolidationCandidates_Observations(t *testing.T) {
	s := testStore(t)

	// Observations with priority <= 2
	addTestMemory(t, s, "memory", "observation", "obs-1", "saw something", 2, 0)
	addTestMemory(t, s, "memory", "observation", "obs-2", "saw another thing", 1, 0)

	// Observation with priority 3 (should NOT be a candidate)
	addTestMemory(t, s, "memory", "observation", "obs-important", "important observation", 3, 0)

	// Non-observation (should NOT be a candidate)
	addTestMemory(t, s, "memory", "environment", "env-1", "env stuff", 1, 0)

	candidates, err := s.GetConsolidationCandidates(ConsolidateScopeObservations, "", 10)
	if err != nil {
		t.Fatalf("GetConsolidationCandidates observations: %v", err)
	}
	if len(candidates) != 2 {
		t.Errorf("expected 2 observation candidates, got %d", len(candidates))
	}
	for _, c := range candidates {
		if c.Entry.Category != "observation" {
			t.Errorf("expected observation category, got %s", c.Entry.Category)
		}
		if c.Entry.Priority > 2 {
			t.Errorf("expected priority <= 2, got %d", c.Entry.Priority)
		}
	}
}

func TestGetConsolidationCandidates_Category(t *testing.T) {
	s := testStore(t)

	addTestMemory(t, s, "memory", "environment", "env-1", "env stuff", 2, 0)
	addTestMemory(t, s, "memory", "environment", "env-2", "more env", 1, 0)
	addTestMemory(t, s, "memory", "environment", "env-important", "important env", 4, 0) // priority > 2, should NOT be candidate
	addTestMemory(t, s, "memory", "project", "proj-1", "project stuff", 2, 0)            // wrong category

	candidates, err := s.GetConsolidationCandidates(ConsolidateScopeCategory, "environment", 10)
	if err != nil {
		t.Fatalf("GetConsolidationCandidates category: %v", err)
	}
	if len(candidates) != 2 {
		t.Errorf("expected 2 environment candidates, got %d", len(candidates))
	}
}

func TestGetConsolidationCandidates_All(t *testing.T) {
	s := testStore(t)

	addTestMemory(t, s, "memory", "environment", "low-prio", "low priority", 1, 0)
	addTestMemory(t, s, "memory", "environment", "high-prio", "high priority", 5, 0)

	candidates, err := s.GetConsolidationCandidates(ConsolidateScopeAll, "", 10)
	if err != nil {
		t.Fatalf("GetConsolidationCandidates all: %v", err)
	}
	if len(candidates) != 1 {
		t.Errorf("expected 1 low-priority candidate, got %d", len(candidates))
	}
}

func TestGetConsolidationCandidates_CategoryRequired(t *testing.T) {
	s := testStore(t)

	_, err := s.GetConsolidationCandidates(ConsolidateScopeCategory, "", 10)
	if err == nil {
		t.Error("expected error for CATEGORY scope without category")
	}
}

func TestGetConsolidationCandidates_BatchSize(t *testing.T) {
	s := testStore(t)

	// Add 5 expired memories
	for i := 0; i < 5; i++ {
		addTestMemory(t, s, "memory", "environment", "batch-test-"+string(rune('A'+i)), "content", 2, 1000)
	}

	candidates, err := s.GetConsolidationCandidates(ConsolidateScopeExpired, "", 3)
	if err != nil {
		t.Fatalf("GetConsolidationCandidates batch size: %v", err)
	}
	if len(candidates) > 3 {
		t.Errorf("expected at most 3 candidates with batch size 3, got %d", len(candidates))
	}
}

func TestConsolidate_Expired(t *testing.T) {
	s := testStore(t)

	// Add an expired memory
	addTestMemory(t, s, "memory", "environment", "expired", "old content", 3, 1000)
	// Add a non-expired memory
	addTestMemory(t, s, "memory", "environment", "not-expired", "current content", 3, 0)

	result, err := s.Consolidate(ConsolidateScopeExpired, "", 0)
	if err != nil {
		t.Fatalf("Consolidate expired: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Removed)
	}

	// Verify the expired one is gone and non-expired remains
	entries, err := s.ListMemories("memory", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 remaining memory, got %d", len(entries))
	}
}

func TestConsolidate_Idle(t *testing.T) {
	s := testStore(t)

	// Add a memory and set its accessed_at to 31 days ago
	entry := addTestMemory(t, s, "memory", "project", "old-project", "old stuff", 2, 0)
	_ = entry
	// Update accessed_at via raw SQL to simulate an idle memory
	_, err := s.DB().Exec("UPDATE memories SET accessed_at = ? WHERE id = ?", 1000, entry.ID) //nolint:errcheck
	if err != nil {
		t.Fatalf("update accessed_at: %v", err)
	}

	// Add a recently accessed memory
	addTestMemory(t, s, "memory", "project", "recent-project", "recent stuff", 2, 0)

	result, err := s.Consolidate(ConsolidateScopeIdle, "", 0)
	if err != nil {
		t.Fatalf("Consolidate idle: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Removed)
	}
}

func TestConsolidate_Observations(t *testing.T) {
	s := testStore(t)

	// Add an old observation (created 8 days ago, priority 2)
	entry := addTestMemory(t, s, "memory", "observation", "old-obs", "old observation", 2, 0)
	// Set created_at to 8 days ago
	weekAgo := uint64(8 * 24 * 60 * 60) // 8 days in seconds from epoch 0
	_, err := s.DB().Exec("UPDATE memories SET created_at = ? WHERE id = ?", int64(weekAgo), entry.ID) //nolint:errcheck
	if err != nil {
		t.Fatalf("update created_at: %v", err)
	}

	// Add a recent observation (should NOT be removed)
	addTestMemory(t, s, "memory", "observation", "recent-obs", "recent observation", 2, 0)

	result, err := s.Consolidate(ConsolidateScopeObservations, "", 0)
	if err != nil {
		t.Fatalf("Consolidate observations: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("expected 1 removed (old observation), got %d", result.Removed)
	}
}

func TestConsolidate_Category(t *testing.T) {
	s := testStore(t)

	addTestMemory(t, s, "memory", "environment", "env-low", "low env", 2, 0)
	addTestMemory(t, s, "memory", "environment", "env-high", "high env", 4, 0) // priority > 2, should survive
	addTestMemory(t, s, "memory", "project", "proj-low", "low proj", 2, 0)    // wrong category, should survive

	result, err := s.Consolidate(ConsolidateScopeCategory, "environment", 0)
	if err != nil {
		t.Fatalf("Consolidate category: %v", err)
	}
	if result.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", result.Removed)
	}

	entries, _ := s.ListMemories("memory", "", 0)
	if len(entries) != 2 {
		t.Errorf("expected 2 remaining memories, got %d", len(entries))
	}
}

func TestConsolidate_CategoryRequired(t *testing.T) {
	s := testStore(t)

	_, err := s.Consolidate(ConsolidateScopeCategory, "", 0)
	if err == nil {
		t.Error("expected error for CATEGORY scope without category")
	}
}

func TestConsolidate_All(t *testing.T) {
	s := testStore(t)

	// Add expired memory
	addTestMemory(t, s, "memory", "environment", "expired", "old", 3, 1000)
	// Add idle memory
	entry := addTestMemory(t, s, "memory", "project", "idle", "idle stuff", 2, 0)
	s.DB().Exec("UPDATE memories SET accessed_at = ? WHERE id = ?", 1000, entry.ID) //nolint:errcheck

	result, err := s.Consolidate(ConsolidateScopeAll, "", 0)
	if err != nil {
		t.Fatalf("Consolidate all: %v", err)
	}
	if result.Removed < 1 {
		t.Errorf("expected at least 1 removed, got %d", result.Removed)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestExpireStaleMemories(t *testing.T) {
	s := testStore(t)

	// Add stale memory (expires_at in the past)
	addTestMemory(t, s, "memory", "environment", "stale", "stale content", 3, 1000)
	// Add fresh memory (no expiry)
	addTestMemory(t, s, "memory", "environment", "fresh", "fresh content", 3, 0)

	removed, err := s.ExpireStaleMemories()
	if err != nil {
		t.Fatalf("ExpireStaleMemories: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 expired memory removed, got %d", removed)
	}

	// Verify only the fresh one remains
	entries, _ := s.ListMemories("memory", "", 0)
	if len(entries) != 1 {
		t.Errorf("expected 1 remaining memory, got %d", len(entries))
	}
}

func TestConsolidate_EmptyDB(t *testing.T) {
	s := testStore(t)

	// All scopes should work on empty DB
	scopes := []ConsolidationScope{
		ConsolidateScopeExpired,
		ConsolidateScopeIdle,
		ConsolidateScopeObservations,
		ConsolidateScopeAll,
	}
	for _, scope := range scopes {
		result, err := s.Consolidate(scope, "", 0)
		if err != nil {
			t.Errorf("Consolidate scope %d on empty DB: %v", scope, err)
		}
		if result.Removed != 0 {
			t.Errorf("expected 0 removed on empty DB for scope %d, got %d", scope, result.Removed)
		}
	}
}



