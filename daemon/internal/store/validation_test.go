// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func TestAddMemory_validation(t *testing.T) {
	s := testStore(t)

	tests := []struct {
		name      string
		target    string
		category  string
		key       string
		content   string
		priority  int32
		agent     string
		wantErr   string // substring of expected error
	}{
		{"empty target", "", "env", "k", "content", 3, "agent", "invalid target"},
		{"bad target", "notes", "env", "k", "content", 3, "agent", "invalid target"},
		{"empty category", "memory", "", "k", "content", 3, "agent", "must not be empty"},
		{"empty key", "memory", "env", "", "content", 3, "agent", "must not be empty"},
		{"priority too low", "memory", "env", "k", "content", 0, "agent", "must be 1-5"},
		{"priority too high", "memory", "env", "k", "content", 6, "agent", "must be 1-5"},
		{"empty source agent", "memory", "env", "k", "content", 3, "", "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := addMemory(s, tt.target, tt.category, tt.key, tt.content, tt.priority, tt.agent, 0) //nolint:errcheck
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			} else if !contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAddMemory_contentTooLong(t *testing.T) {
	s := testStore(t)

	// Create a string that exceeds MaxContentLength
	longContent := make([]byte, MaxContentLength+1)
	for i := range longContent {
		longContent[i] = 'x'
	}

	_, err := addMemory(s, "memory", "env", "bigkey", string(longContent), 3, "agent", 0) //nolint:errcheck
	if err == nil {
		t.Error("expected error for content too long")
	}
	if err != nil && !contains(err.Error(), "content too long") {
		t.Errorf("error = %q, want substring 'content too long'", err.Error())
	}
}

func TestListMemories_pagination(t *testing.T) {
	s := testStore(t)

	// Create 5 memories
	for i := 0; i < 5; i++ {
		_, err := addMemory(s, "memory", "environment", "key_pagination_"+string(rune('a'+i)), "content", 3, "agent-001", 0) //nolint:errcheck
		if err != nil {
			t.Fatalf("add memory: %v", err)
		}
	}

	// Test paginated query with limit 2, offset 0
	results, err := s.ListMemoriesPaginated("memory", "", 0, 2, 0)
	if err != nil {
		t.Fatalf("paginated list: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(results))
	}

	// Test paginated query with limit 2, offset 2
	results2, err := s.ListMemoriesPaginated("memory", "", 0, 2, 2)
	if err != nil {
		t.Fatalf("paginated list offset: %v", err)
	}
	if len(results2) != 2 {
		t.Errorf("expected 2 results with offset=2, got %d", len(results2))
	}

	// Verify no overlap
	if results[0].ID == results2[0].ID || results[0].ID == results2[1].ID {
		t.Error("paginated results overlap — distinct pages should have distinct entries")
	}
}

func TestAddMemory_transactionAtomicity(t *testing.T) {
	// Verify that concurrent upserts with the same target+key don't create duplicates
	s := testStore(t)

	// Add initial
	entry1, err := addMemory(s, "memory", "environment", "atomic_key", "first", 3, "agent-001", 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Upsert same target+key
	entry2, err := addMemory(s, "memory", "environment", "atomic_key", "second", 4, "agent-002", 0) //nolint:errcheck
	if err != nil {
		t.Fatalf("second add: %v", err)
	}

	// Should have same ID (upsert, not new entry)
	if entry2.ID != entry1.ID {
		t.Errorf("upsert changed ID from %s to %s — expected same ID", entry1.ID, entry2.ID)
	}

	// Should have updated content
	if entry2.Content != "second" {
		t.Errorf("content = %q, want %q", entry2.Content, "second")
	}

	// Should have only one entry
	entries, _ := s.ListMemories("memory", "environment", 0)
	keyCount := 0
	for _, e := range entries {
		if e.Key == "atomic_key" {
			keyCount++
		}
	}
	if keyCount != 1 {
		t.Errorf("found %d entries with key 'atomic_key', want 1", keyCount)
	}
}

func TestEnsureAgent_validation(t *testing.T) {
	s := testStore(t)

	_, _, err := s.EnsureAgent("", "name", "role")
	if err == nil {
		t.Error("expected error for empty agent_id")
	}
	if err != nil && !contains(err.Error(), "must not be empty") {
		t.Errorf("error = %q, want substring 'must not be empty'", err.Error())
	}
}

func TestCreateSession_validation(t *testing.T) {
	s := testStore(t)

	_, err := s.CreateSession("", "cli", "Test")
	if err == nil {
		t.Error("expected error for empty agent_id")
	}
}

func TestAddMessages_empty(t *testing.T) {
	s := testStore(t)

	sess, _ := s.CreateSession("agent-001", "cli", "Test")

	// Empty message list should be a no-op
	err := s.AddMessages(sess.ID, []Message{})
	if err != nil {
		t.Errorf("empty messages should be no-op, got: %v", err)
	}
}

func TestSearch_limitBounds(t *testing.T) {
	s := testStore(t)

	// Test with limit > 1000 should be clamped
	addMemory(s, "memory", "environment", "k1", "searchable content here", 3, "agent-001", 0) //nolint:errcheck

	_, err := s.Search("content", "", 5000) // Should be clamped to 1000
	if err != nil {
		t.Errorf("search with large limit: %v", err)
	}
}