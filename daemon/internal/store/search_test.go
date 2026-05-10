// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func TestSearch_basic(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "host1", "The server runs on port 8080", 3, testAgent, 0) //nolint:errcheck
	addMemory(s, "memory", "environment", "host2", "The database is PostgreSQL", 3, testAgent, 0)     //nolint:errcheck
	addMemory(s, "memory", "project", "proj1", "Migration scripts needed", 2, testAgent, 0)          //nolint:errcheck

	results, err := s.Search("default", "server port", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results for 'server port', got 0")
	}
}

func TestSearch_ORsemantics(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "k1", "Apple banana cherry", 3, testAgent, 0)  //nolint:errcheck
	addMemory(s, "memory", "environment", "k2", "Dog elephant fox", 3, testAgent, 0)      //nolint:errcheck

	// Multi-word with no operators should use OR — match any term
	results, err := s.Search("default", "apple elephant", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("OR semantics: expected >= 2 results, got %d", len(results))
	}
}

func TestSearch_colonHandling(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "path", "Config at /usr/local/bin:tool", 3, testAgent, 0) //nolint:errcheck

	// Colons should not crash (they're quoted by sanitizeFTS5)
	results, err := s.Search("default", "local:tool", "", 10)
	if err != nil {
		t.Fatalf("search with colon: %v", err)
	}
	// May or may not match — the key is it doesn't crash
	_ = results
}

func TestSearch_targetFilter(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "t1", "Agent environment note", 3, testAgent, 0) //nolint:errcheck
	addMemory(s, "user", "user", "t2", "User environment note", 3, testAgent, 0) //nolint:errcheck

	results, err := s.Search("default", "environment", "memory", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, r := range results {
		if r.Target != "memory" {
			t.Errorf("got target %q, want %q", r.Target, "memory")
		}
	}
}

func TestSearch_emptyQuery(t *testing.T) {
	s := testStore(t)

	addMemory(s, "memory", "environment", "t1", "Some content", 3, testAgent, 0) //nolint:errcheck

	// Empty query should not crash
	_, err := s.Search("default", "", "", 10)
	if err != nil {
		t.Fatalf("empty search: %v", err)
	}
}

func TestSanitizeFTS5(t *testing.T) {

	tests := []struct {
		input string
		want  string // substring check
	}{
		{"hello", "hello"},
		{"hello world", "hello OR world"},           // OR by default
		{"hello AND world", "AND"},                   // AND preserved
		{"config.yaml", `"config.yaml"`},             // dot tokens quoted
		{"path:to:file", `"path:to:file"`},           // colon tokens quoted
		{"NOT stale", "NOT"},                          // NOT preserved
		{"hello NEAR world", "NEAR"},                  // NEAR preserved
		{"", `""`},                                     // empty → empty match
	}

	for _, tt := range tests {
		got := sanitizeFTS5(tt.input)
		if !contains(got, tt.want) {
			t.Errorf("sanitizeFTS5(%q) = %q, want substring %q", tt.input, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}