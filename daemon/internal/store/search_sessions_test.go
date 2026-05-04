// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func TestSearchSessions(t *testing.T) {
	s := testStore(t)

	// Create sessions with known titles
	_, err := s.CreateSession("agent-1", "cli", "Kore Extraction Project")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, err = s.CreateSession("agent-1", "cli", "Memory System Debug")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	_, err = s.CreateSession("agent-2", "cron", "Nightly Backup")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Search for "memory"
	sessions, err := s.SearchSessions("memory", 10)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session matching 'memory', got %d", len(sessions))
	} else if sessions[0].Title != "Memory System Debug" {
		t.Errorf("expected title 'Memory System Debug', got %s", sessions[0].Title)
	}

	// Search for "project"
	sessions, err = s.SearchSessions("project", 10)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session matching 'project', got %d", len(sessions))
	}

	// Case-insensitive search
	sessions, err = s.SearchSessions("EXTRACTION", 10)
	if err != nil {
		t.Fatalf("SearchSessions case: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session matching 'EXTRACTION' (case-insensitive), got %d", len(sessions))
	}

	// Empty query should return error
	_, err = s.SearchSessions("", 10)
	if err == nil {
		t.Error("expected error for empty query")
	}

	// No matching sessions
	sessions, err = s.SearchSessions("nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchSessions no match: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for nonexistent query, got %d", len(sessions))
	}
}

func TestSearchSessions_Limit(t *testing.T) {
	s := testStore(t)

	// Create 5 sessions with the same keyword
	for i := 0; i < 5; i++ {
		_, err := s.CreateSession("agent-1", "cli", "Test Session "+string(rune('A'+i)))
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}

	// Limit to 3
	sessions, err := s.SearchSessions("Test", 3)
	if err != nil {
		t.Fatalf("SearchSessions limit: %v", err)
	}
	if len(sessions) > 3 {
		t.Errorf("expected at most 3 sessions with limit 3, got %d", len(sessions))
	}
}

func TestSearchSessions_SpecialChars(t *testing.T) {
	s := testStore(t)

	_, err := s.CreateSession("agent-1", "cli", "Test%Session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Searching for % should not cause SQL errors
	sessions, err := s.SearchSessions("%", 10)
	if err != nil {
		t.Fatalf("SearchSessions special chars: %v", err)
	}
	// Should return 0 or 1 — the % in the pattern is escaped
	// so "Test%Session" title doesn't literally contain "%"
	// but the LIKE pattern becomes %\%% which matches strings with % in them
	if len(sessions) != 1 {
		t.Errorf("expected 1 session with %% in title, got %d", len(sessions))
	}
}

func TestSanitizeLike(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal", "normal"},
		{"has%percent", "has\\%percent"},
		{"has_under", "has\\_under"},
		{"has\\backslash", "has\\\\backslash"},
		{"mix%of_special\\chars", "mix\\%of\\_special\\\\chars"},
	}
	for _, tt := range tests {
		result := sanitizeLike(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeLike(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}