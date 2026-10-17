// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"testing"
)

// ============================================================================
// Phase 28: Namespace Isolation — TDD tests (store layer)
// ============================================================================

// --- Memory namespace isolation ---

func TestNamespace_MemoryIsolation(t *testing.T) {
	s := testStore(t)

	// Alice adds a memory
	_, err := s.AddMemory(AddMemoryParams{
		Owner:       "alice",
		Target:      "memory",
		Category:    "test",
		Key:         "secret",
		Content:     "alice's secret",
		Priority:    3,
		SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("alice AddMemory: %v", err)
	}

	// Bob should NOT find Alice's memory by content search
	bobResults, err := s.Search("bob", "alice", "", 10)
	if err != nil {
		t.Fatalf("bob Search: %v", err)
	}
	if len(bobResults) != 0 {
		t.Errorf("bob found %d memories matching 'alice', want 0 (cross-namespace leak)", len(bobResults))
	}

	// Alice should find her own memory
	aliceResults, err := s.Search("alice", "alice", "", 10)
	if err != nil {
		t.Fatalf("alice Search: %v", err)
	}
	if len(aliceResults) != 1 {
		t.Errorf("alice found %d memories, want 1", len(aliceResults))
	}
}

func TestNamespace_MemoryRemoveScoped(t *testing.T) {
	s := testStore(t)

	// Alice adds a memory
	entry, err := s.AddMemory(AddMemoryParams{
		Owner:       "alice",
		Target:      "memory",
		Category:    "test",
		Key:         "item",
		Content:     "alice item",
		Priority:    3,
		SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Bob tries to delete Alice's memory (wrong owner)
	removed, err := s.RemoveMemory(entry.ID, "bob")
	if err != nil {
		t.Fatalf("RemoveMemory by bob: %v", err)
	}
	if removed {
		t.Error("bob was able to remove alice's memory (cross-namespace delete)")
	}

	// Alice should still be able to find her memory
	found, err := s.GetMemoryByID(entry.ID)
	if err != nil {
		t.Fatalf("alice GetMemoryByID after bob's attempt: %v", err)
	}
	if found == nil || found.Content != "alice item" {
		t.Error("alice's memory was lost after bob's delete attempt")
	}

	// Alice deletes her own memory
	removed, err = s.RemoveMemory(entry.ID, "alice")
	if err != nil {
		t.Fatalf("RemoveMemory by alice: %v", err)
	}
	if !removed {
		t.Error("alice could not remove her own memory")
	}
}

func TestNamespace_ListMemoriesScoped(t *testing.T) {
	s := testStore(t)

	// Alice adds two memories
	s.AddMemory(AddMemoryParams{Owner: "alice", Target: "memory", Category: "test", Key: "a1", Content: "a1", Priority: 3, SourceAgent: "test"})
	s.AddMemory(AddMemoryParams{Owner: "alice", Target: "memory", Category: "test", Key: "a2", Content: "a2", Priority: 3, SourceAgent: "test"})
	// Bob adds one memory
	s.AddMemory(AddMemoryParams{Owner: "bob", Target: "memory", Category: "test", Key: "b1", Content: "b1", Priority: 3, SourceAgent: "test"})

	// Alice lists — should only see her 2
	aliceList, err := s.ListMemories("alice", "memory", "", 0)
	if err != nil {
		t.Fatalf("alice ListMemories: %v", err)
	}
	if len(aliceList) != 2 {
		t.Errorf("alice sees %d memories, want 2", len(aliceList))
	}

	// Bob lists — should only see his 1
	bobList, err := s.ListMemories("bob", "memory", "", 0)
	if err != nil {
		t.Fatalf("bob ListMemories: %v", err)
	}
	if len(bobList) != 1 {
		t.Errorf("bob sees %d memories, want 1", len(bobList))
	}
}

// --- API key to owner mapping ---

func TestAPIKeyOwnerMapping(t *testing.T) {
	s := testStore(t)

	// Map an API key to an owner
	err := s.SetAPIKeyOwner("sk-test-key-123", "alice")
	if err != nil {
		t.Fatalf("SetAPIKeyOwner: %v", err)
	}

	// Look up the owner
	owner, err := s.GetOwnerForAPIKey("sk-test-key-123")
	if err != nil {
		t.Fatalf("GetOwnerForAPIKey: %v", err)
	}
	if owner != "alice" {
		t.Errorf("GetOwnerForAPIKey = %q, want %q", owner, "alice")
	}

	// Unknown API key returns default
	owner, err = s.GetOwnerForAPIKey("nonexistent")
	if err != nil {
		t.Fatalf("GetOwnerForAPIKey(nonexistent): %v", err)
	}
	if owner != "default" {
		t.Errorf("GetOwnerForAPIKey(nonexistent) = %q, want %q", owner, "default")
	}
}
