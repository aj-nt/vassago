// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

// ============================================================================
// Phase 28: Namespace Isolation — TDD tests
// ============================================================================

func TestNamespace_SchemaVersionBumped(t *testing.T) {
	if SchemaVersion < 6 {
		t.Errorf("SchemaVersion = %d, want >= 6 (namespace migration)", SchemaVersion)
	}
}

func TestNamespace_OwnerColumnInSchema(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	rows, err := s.db.Query("PRAGMA table_info(memories)")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dfltValue *string
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "owner" {
			found = true
			if colType != "TEXT" {
				t.Errorf("owner type = %q, want TEXT", colType)
			}
			if notNull != 1 {
				t.Errorf("owner notnull = %d, want 1", notNull)
			}
		}
	}
	if !found {
		t.Error("owner column not found in memories table")
	}
}

func TestNamespace_OwnerDefaultsToDefault(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	e, err := addMemory(s, "memory", "test", "default-owner", "data", 3, "agent-1", 0)
	if err != nil {
		t.Fatalf("addMemory: %v", err)
	}
	if e.Owner != "default" {
		t.Errorf("owner = %q, want 'default'", e.Owner)
	}
}

func TestNamespace_AddMemoryUpsertWithinOwner(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	e1, err := addMemory(s, "memory", "test", "upsert-key", "v1", 3, "a1", 0)
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	e2, err := addMemory(s, "memory", "test", "upsert-key", "v2", 5, "a2", 0)
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	if e1.ID != e2.ID {
		t.Error("upsert within same owner should return same ID")
	}
	if e2.Content != "v2" {
		t.Errorf("content = %q, want v2", e2.Content)
	}
	if e2.Priority != 5 {
		t.Errorf("priority = %d, want 5", e2.Priority)
	}
}

func TestNamespace_SearchFiltersByOwner(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	addMemory(s, "memory", "alpha", "k1", "needle", 5, "a1", 0)

	results, err := s.Search("default", "needle", "memory", 10)
	if err != nil {
		t.Fatalf("search default: %v", err)
	}
	if len(results) == 0 {
		t.Error("should find memory with default owner")
	}

	results, err = s.Search("other-ns", "needle", "memory", 10)
	if err != nil {
		t.Fatalf("search other: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("other owner: want 0 results, got %d", len(results))
	}
}

func TestNamespace_SearchEmptyOwner(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	results, err := s.Search("", "test", "memory", 10)
	if err == nil && len(results) > 0 {
		t.Error("expected error or empty when owner is empty")
	}
}

func TestNamespace_RemoveMemoryOwnerScoping(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	e, _ := addMemory(s, "memory", "test", "remove-key", "data", 3, "a1", 0)

	deleted, err := s.RemoveMemory(e.ID, "wrong-owner")
	if err != nil {
		t.Fatalf("remove wrong owner: %v", err)
	}
	if deleted {
		t.Error("wrong owner should not be able to delete")
	}
	if _, err := s.GetMemoryByID(e.ID); err != nil {
		t.Fatalf("entry should still exist: %v", err)
	}

	deleted, err = s.RemoveMemory(e.ID, "default")
	if err != nil {
		t.Fatalf("remove correct owner: %v", err)
	}
	if !deleted {
		t.Error("correct owner should be able to delete")
	}
}

func TestNamespace_ListMemoriesOwnerFilter(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	addMemory(s, "memory", "test", "k1", "d1", 3, "a1", 0)
	addMemory(s, "memory", "test", "k2", "d2", 3, "a2", 0)

	r, _ := s.ListMemories("default", "memory", "test", 3)
	if len(r) != 2 {
		t.Errorf("default owner: want 2, got %d", len(r))
	}

	r, _ = s.ListMemories("other", "memory", "test", 3)
	if len(r) != 0 {
		t.Errorf("other owner: want 0, got %d", len(r))
	}
}

func TestNamespace_HotBlockOwnerScoping(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	addMemory(s, "memory", "env", "key1", "hot data", 5, "a1", 0)

	r, _ := s.ComputeHotBlock("default", "memory", HotBlockConfig{CharLimit: 4000})
	if r.EntryCount == 0 {
		t.Error("default owner should see entries")
	}

	r, _ = s.ComputeHotBlock("other", "memory", HotBlockConfig{CharLimit: 4000})
	if r.EntryCount != 0 {
		t.Errorf("other owner: want 0 entries, got %d", r.EntryCount)
	}
}

func TestNamespace_ConsolidateOwnerScoping(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	s.AddMemory(AddMemoryParams{
		Owner: "default", Target: "memory", Category: "test",
		Key: "exp-key", Content: "expired", Priority: 2,
		SourceAgent: "a1", ExpiresAt: 1,
	})

	r, _ := s.Consolidate("other", ConsolidateScopeExpired, "", 100)
	if r.Removed != 0 {
		t.Errorf("wrong owner: removed %d, want 0", r.Removed)
	}

	r, _ = s.Consolidate("default", ConsolidateScopeExpired, "", 100)
	if r.Removed != 1 {
		t.Errorf("correct owner: removed %d, want 1", r.Removed)
	}
}
