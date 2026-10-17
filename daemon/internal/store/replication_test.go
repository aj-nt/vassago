// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"testing"
	"time"
)

// ============================================================================
// Peer Replication: GetChangesSince — TDD tests
// ============================================================================

func TestGetChangesSince_returnsNewerEntries(t *testing.T) {
	s := testStore(t)

	e1, err := s.AddMemory(AddMemoryParams{
		Owner: "alice", Target: "memory", Category: "test",
		Key: "k1", Content: "old", Priority: 3, SourceAgent: "agent-a",
	})
	if err != nil {
		t.Fatalf("AddMemory k1: %v", err)
	}

	since := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)

	e2, err := s.AddMemory(AddMemoryParams{
		Owner: "alice", Target: "memory", Category: "test",
		Key: "k2", Content: "new", Priority: 3, SourceAgent: "agent-a",
	})
	if err != nil {
		t.Fatalf("AddMemory k2: %v", err)
	}

	changes, err := s.GetChangesSince("alice", since)
	if err != nil {
		t.Fatalf("GetChangesSince: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1", len(changes))
	}
	if changes[0].ID != e2.ID {
		t.Errorf("got entry %s, want %s", changes[0].ID, e2.ID)
	}
	_ = e1
}

func TestGetChangesSince_namespaceIsolation(t *testing.T) {
	s := testStore(t)

	since := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)

	s.AddMemory(AddMemoryParams{
		Owner: "alice", Target: "memory", Category: "test",
		Key: "a1", Content: "alice", Priority: 3, SourceAgent: "agent-a",
	})
	s.AddMemory(AddMemoryParams{
		Owner: "bob", Target: "memory", Category: "test",
		Key: "b1", Content: "bob", Priority: 3, SourceAgent: "agent-b",
	})

	aliceChanges, err := s.GetChangesSince("alice", since)
	if err != nil {
		t.Fatalf("alice GetChangesSince: %v", err)
	}
	if len(aliceChanges) != 1 {
		t.Errorf("alice got %d changes, want 1", len(aliceChanges))
	}

	bobChanges, err := s.GetChangesSince("bob", since)
	if err != nil {
		t.Fatalf("bob GetChangesSince: %v", err)
	}
	if len(bobChanges) != 1 {
		t.Errorf("bob got %d changes, want 1", len(bobChanges))
	}
}

func TestGetChangesSince_includesUpdates(t *testing.T) {
	s := testStore(t)

	s.AddMemory(AddMemoryParams{
		Owner: "alice", Target: "memory", Category: "test",
		Key: "k1", Content: "v1", Priority: 3, SourceAgent: "agent-a",
	})

	since := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)

	updated, err := s.AddMemory(AddMemoryParams{
		Owner: "alice", Target: "memory", Category: "test",
		Key: "k1", Content: "v2", Priority: 3, SourceAgent: "agent-a",
	})
	if err != nil {
		t.Fatalf("update k1: %v", err)
	}

	changes, err := s.GetChangesSince("alice", since)
	if err != nil {
		t.Fatalf("GetChangesSince: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("got %d changes, want 1 (the update)", len(changes))
	}
	if changes[0].Content != "v2" {
		t.Errorf("got content %q, want %q", changes[0].Content, "v2")
	}
	_ = updated
}

// ============================================================================
// Peer Replication: ReplicationState — TDD tests
// ============================================================================

func TestReplicationState_setAndGet(t *testing.T) {
	s := testStore(t)

	err := s.SetReplicationState("studio", "default", 1234567890)
	if err != nil {
		t.Fatalf("SetReplicationState: %v", err)
	}

	state, err := s.GetReplicationState("studio", "default")
	if err != nil {
		t.Fatalf("GetReplicationState: %v", err)
	}

	if state.LastSyncAt != 1234567890 {
		t.Errorf("LastSyncAt = %d, want 1234567890", state.LastSyncAt)
	}
}

func TestReplicationState_defaultZero(t *testing.T) {
	s := testStore(t)

	state, err := s.GetReplicationState("new-peer", "default")
	if err != nil {
		t.Fatalf("GetReplicationState: %v", err)
	}

	if state.LastSyncAt != 0 {
		t.Errorf("new peer should have LastSyncAt=0, got %d", state.LastSyncAt)
	}
}

func TestReplicationState_updateExisting(t *testing.T) {
	s := testStore(t)

	err := s.SetReplicationState("studio", "default", 100)
	if err != nil {
		t.Fatalf("SetReplicationState first: %v", err)
	}

	err = s.SetReplicationState("studio", "default", 200)
	if err != nil {
		t.Fatalf("SetReplicationState second: %v", err)
	}

	state, err := s.GetReplicationState("studio", "default")
	if err != nil {
		t.Fatalf("GetReplicationState: %v", err)
	}

	if state.LastSyncAt != 200 {
		t.Errorf("LastSyncAt = %d, want 200", state.LastSyncAt)
	}
}
