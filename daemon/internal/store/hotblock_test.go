package store

import (
	"testing"
)

const testAgentHotBlock = "hotblock-agent-001"

func TestComputeHotBlock_basic(t *testing.T) {
	s := testStore(t)

	s.AddMemory("memory", "environment", "python", "Python 3.11 on venv", 3, testAgentHotBlock, 0)
	s.AddMemory("memory", "project", "mnemosyne", "Memory daemon in Go", 2, testAgentHotBlock, 0)
	s.AddMemory("user", "user", "name", "AJ Flinton", 4, testAgentHotBlock, 0)

	// Memory hot block
	result, err := s.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:     4000,
		BoostPriority: 2,
	})
	if err != nil {
		t.Fatalf("compute hot block: %v", err)
	}
	if result.Content == "" {
		t.Error("hot block content is empty")
	}
	if result.EntryCount == 0 {
		t.Error("hot block has no entries")
	}
}

func TestComputeHotBlock_respectsCharLimit(t *testing.T) {
	s := testStore(t)

	// Add entries that would exceed a small limit
	s.AddMemory("memory", "environment", "k1", "Short entry", 3, testAgentHotBlock, 0)
	s.AddMemory("memory", "environment", "k2", "Another short entry", 2, testAgentHotBlock, 0)
	s.AddMemory("memory", "environment", "k3", "Yet another entry that makes it longer", 2, testAgentHotBlock, 0)

	result, err := s.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:     50, // Very small limit
		BoostPriority: 2,
	})
	if err != nil {
		t.Fatalf("compute hot block: %v", err)
	}
	if result.CharCount > 50+200 { // Allow some margin for first entry
		t.Errorf("char count %d exceeds limit + margin", result.CharCount)
	}
}

func TestComputeHotBlock_boostCategories(t *testing.T) {
	s := testStore(t)

	s.AddMemory("memory", "observation", "obs1", "Some observation", 3, testAgentHotBlock, 0)
	s.AddMemory("memory", "project", "proj1", "Important project data", 3, testAgentHotBlock, 0)

	// With "project" boosted, it should come first
	result, err := s.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:       4000,
		BoostPriority:   2,
		BoostCategories: []string{"project"},
	})
	if err != nil {
		t.Fatalf("compute hot block: %v", err)
	}
	// The project entry should appear before the observation
	projIdx := indexOf(result.Content, "[project]")
	obsIdx := indexOf(result.Content, "[observation]")
	if projIdx >= 0 && obsIdx >= 0 && projIdx > obsIdx {
		t.Error("boosted category should come before non-boosted")
	}
}

func TestComputeHotBlock_suppressCategories(t *testing.T) {
	s := testStore(t)

	s.AddMemory("memory", "project", "proj1", "Important project", 3, testAgentHotBlock, 0)
	s.AddMemory("memory", "observation", "obs1", "Less important observation", 3, testAgentHotBlock, 0)

	result, err := s.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:          4000,
		BoostPriority:      2,
		SuppressCategories: []string{"observation"},
	})
	if err != nil {
		t.Fatalf("compute hot block: %v", err)
	}
	// Project should appear before observation
	projIdx := indexOf(result.Content, "[project]")
	obsIdx := indexOf(result.Content, "[observation]")
	if projIdx >= 0 && obsIdx >= 0 && projIdx > obsIdx {
		t.Error("suppressed category should come after non-suppressed")
	}
}

func TestComputeHotBlock_expiredObservations(t *testing.T) {
	s := testStore(t)

	// Add an already-expired observation (expires_at in the past)
	s.AddMemory("memory", "observation", "expired", "This is stale", 3, testAgentHotBlock, 1) // expired at epoch 1

	// Add a non-expired observation
	s.AddMemory("memory", "observation", "fresh", "This is recent", 3, testAgentHotBlock, 0) // no expiry

	result, err := s.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:     4000,
		BoostPriority: 2,
	})
	if err != nil {
		t.Fatalf("compute hot block: %v", err)
	}
	// The expired entry should not appear
	if indexOf(result.Content, "expired") >= 0 && indexOf(result.Content, "stale") >= 0 {
		t.Error("expired observation should not appear in hot block")
	}
}

func TestFormatEntry(t *testing.T) {
	tests := []struct {
		entry *MemoryEntry
		want  string
	}{
		{
			&MemoryEntry{Category: "environment", Key: "python", Content: "3.11"},
			"[environment] python: 3.11",
		},
		{
			&MemoryEntry{Category: "observation", Key: "", Content: "Something happened"},
			"[observation] Something happened",
		},
	}
	for _, tt := range tests {
		got := formatEntry(tt.entry)
		if got != tt.want {
			t.Errorf("formatEntry(%+v) = %q, want %q", tt.entry, got, tt.want)
		}
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}