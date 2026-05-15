// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
	"time"
)

const testAgent = "test-agent-001"

func TestAddMemory(t *testing.T) {
	s := testStore(t)

	entry, err := addMemory(s, "memory", "environment", "test_key", "test content", 3, testAgent, 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	if entry.ID == "" {
		t.Error("entry.ID is empty")
	}
	if entry.Target != "memory" {
		t.Errorf("target = %q, want %q", entry.Target, "memory")
	}
	if entry.Category != "environment" {
		t.Errorf("category = %q, want %q", entry.Category, "environment")
	}
	if entry.Key != "test_key" {
		t.Errorf("key = %q, want %q", entry.Key, "test_key")
	}
	if entry.Content != "test content" {
		t.Errorf("content = %q, want %q", entry.Content, "test content")
	}
	if entry.Priority != 3 {
		t.Errorf("priority = %d, want 3", entry.Priority)
	}
	if entry.SourceAgent != testAgent {
		t.Errorf("source_agent = %q, want %q", entry.SourceAgent, testAgent)
	}
	if entry.CreatedAt == 0 {
		t.Error("created_at is zero")
	}
	if entry.UpdatedAt != entry.CreatedAt {
		t.Errorf("updated_at != created_at on creation")
	}
}

func TestAddMemory_upsert(t *testing.T) {
	s := testStore(t)

	// Add initial
	entry1, err := addMemory(s, "memory", "environment", "unique_key", "original", 2, testAgent, 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("add first: %v", err)
	}

	// Add with same target+key — should update
	entry2, err := addMemory(s, "memory", "environment", "unique_key", "updated", 4, "agent-002", 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("add second: %v", err)
	}

	if entry2.ID != entry1.ID {
		t.Errorf("upsert changed ID from %s to %s", entry1.ID, entry2.ID)
	}
	if entry2.Content != "updated" {
		t.Errorf("content = %q, want %q", entry2.Content, "updated")
	}
	if entry2.Priority != 4 {
		t.Errorf("priority = %d, want 4", entry2.Priority)
	}
	if entry2.UpdatedAt < entry1.CreatedAt {
		t.Error("updated_at should be >= created_at")
	}
}

func TestAddMemory_differentTargetsSameKey(t *testing.T) {
	s := testStore(t)

	// Same key, different targets — both should exist
	_, err := addMemory(s, "memory", "environment", "host", "agent notes", 3, testAgent, 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("add memory target: %v", err)
	}
	_, err = addMemory(s, "user", "user", "host", "user notes", 3, testAgent, 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("add user target: %v", err)
	}

	entries, err := s.ListMemories("default", "", "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestGetMemoryByID(t *testing.T) {
	s := testStore(t)

	added, err := addMemory(s, "memory", "quirk", "test", "content", 3, testAgent, 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	got, err := s.GetMemoryByID(added.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "content" {
		t.Errorf("content = %q, want %q", got.Content, "content")
	}
}

func TestGetMemoryByID_notFound(t *testing.T) {
	s := testStore(t)

	_, err := s.GetMemoryByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestGetMemoryByID_updatesAccessedAt(t *testing.T) {
	s := testStore(t)

	added, _ := addMemory(s, "memory", "environment", "test", "content", 3, testAgent, 0) //nolint:errcheck
	time.Sleep(10 * time.Millisecond)                                                     // ensure time passes

	got, _ := s.GetMemoryByID(added.ID)
	if got.AccessedAt < added.CreatedAt {
		t.Error("accessed_at should be updated on read")
	}
}

func TestRemoveMemory(t *testing.T) {
	s := testStore(t)

	entry, _ := addMemory(s, "memory", "environment", "test", "content", 3, testAgent, 0) //nolint:errcheck
	deleted, err := s.RemoveMemory(entry.ID, "default")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !deleted {
		t.Error("expected deleted = true")
	}

	_, err = s.GetMemoryByID(entry.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestRemoveMemory_notFound(t *testing.T) {
	s := testStore(t)

	deleted, err := s.RemoveMemory("nonexistent", "default")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if deleted {
		t.Error("expected deleted = false for nonexistent ID")
	}
}

func TestListMemories_filterByTarget(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "a1", "env1", 3, testAgent, 0) //nolint:errcheck
	addMemory(s, "memory", "quirk", "a2", "quirk1", 3, testAgent, 0)     //nolint:errcheck
	addMemory(s, "user", "user", "a3", "user1", 3, testAgent, 0)         //nolint:errcheck

	entries, err := s.ListMemories("default", "memory", "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 memory entries, got %d", len(entries))
	}
}

func TestListMemories_filterByCategory(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "b1", "env", 3, testAgent, 0) //nolint:errcheck
	addMemory(s, "memory", "quirk", "b2", "quirk", 3, testAgent, 0)     //nolint:errcheck
	addMemory(s, "memory", "project", "b3", "proj", 3, testAgent, 0)    //nolint:errcheck

	entries, err := s.ListMemories("default", "", "quirk", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 quirk entry, got %d", len(entries))
	}
}

func TestListMemories_filterByMinPriority(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "c1", "low", 1, testAgent, 0)  //nolint:errcheck
	addMemory(s, "memory", "environment", "c2", "mid", 3, testAgent, 0)  //nolint:errcheck
	addMemory(s, "memory", "environment", "c3", "high", 5, testAgent, 0) //nolint:errcheck

	entries, err := s.ListMemories("default", "", "", 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with priority >= 3, got %d", len(entries))
	}
}

func TestListMemories_orderedByPriority(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "low", "low", 1, testAgent, 0)   //nolint:errcheck
	addMemory(s, "memory", "environment", "high", "high", 5, testAgent, 0) //nolint:errcheck
	addMemory(s, "memory", "environment", "mid", "mid", 3, testAgent, 0)   //nolint:errcheck

	entries, _ := s.ListMemories("default", "", "", 0)
	if len(entries) < 3 {
		t.Fatalf("need at least 3 entries, got %d", len(entries))
	}
	if entries[0].Priority < entries[1].Priority {
		t.Errorf("entries not sorted by priority desc: %d before %d", entries[0].Priority, entries[1].Priority)
	}
}
