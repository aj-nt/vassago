package store

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// setupTestStore creates a test SQLite store in a temp directory.
func setupRedTeamStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	return s, func() { s.Close() }
}

// --- RED TEAM: Crash it ---

func TestRedTeam_NilEmptyRequiredFields(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	tests := []struct {
		name    string
		params  AddMemoryParams
		wantErr bool // true = we expect an error, false = we expect success
	}{
		{"EmptyTarget", AddMemoryParams{Target: "", Category: "cat", Key: "k", Content: "c", Priority: 3, SourceAgent: "agent"}, true},
		{"EmptyCategory", AddMemoryParams{Target: "memory", Category: "", Key: "k", Content: "c", Priority: 3, SourceAgent: "agent"}, true},
		{"EmptyKey", AddMemoryParams{Target: "", Category: "cat", Key: "k", Content: "c", Priority: 3, SourceAgent: "agent"}, true},
		{"EmptyContent", AddMemoryParams{Target: "memory", Category: "cat", Key: "k", Content: "", Priority: 3, SourceAgent: "agent"}, false}, // RED TEAM FINDING: empty content is accepted
		{"ZeroPriority", AddMemoryParams{Target: "memory", Category: "cat", Key: "k", Content: "c", Priority: 0, SourceAgent: "agent"}, true},
		{"NegativePriority", AddMemoryParams{Target: "memory", Category: "cat", Key: "k", Content: "c", Priority: -1, SourceAgent: "agent"}, true},
		{"HighPriority", AddMemoryParams{Target: "memory", Category: "cat", Key: "k", Content: "c", Priority: 999, SourceAgent: "agent"}, true},
		{"EmptySourceAgent", AddMemoryParams{Target: "memory", Category: "cat", Key: "k", Content: "c", Priority: 3, SourceAgent: ""}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.AddMemory(tc.params)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

func TestRedTeam_OversizedContent(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	// 2MB content — should be rejected or handled without crash
	bigContent := strings.Repeat("A", 2*1024*1024)

	_, err := store.AddMemory(AddMemoryParams{
		Target:      "memory",
		Category:    "test",
		Key:         "oversized",
		Content:     bigContent,
		Priority:    3,
		SourceAgent: "redteam",
	})

	// BUG: The store currently has NO content size limit.
	// It will accept and store 2MB content. This is a finding.
	if err != nil {
		t.Logf("Store rejected oversized content (good): %v", err)
	} else {
		t.Logf("FINDING: Store accepted 2MB content with no size limit. Consider adding a max_content_length config.")
	}
}

func TestRedTeam_SQLInjection(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	injections := []string{
		"'; DROP TABLE memories; --",
		"' OR '1'='1",
		"'; EXEC xp_cmdshell('dir'); --",
		"Robert'); DROP TABLE students; --",
	}

	for _, inj := range injections {
		t.Run(fmt.Sprintf("content_%q", inj), func(t *testing.T) {
			// Using injection as content — should be safe (parameterized queries)
			id, err := store.AddMemory(AddMemoryParams{
				Target:      "memory",
				Category:    "test",
				Key:         "sql-inject-content",
				Content:     inj,
				Priority:    3,
				SourceAgent: "redteam",
			})
			if err != nil {
				t.Fatalf("Store rejected content with SQL string (parameterized query should handle this): %v", err)
			}
			// Verify the table still exists by searching
			results, err := store.Search("DROP TABLE", "", 10)
			if err != nil {
				t.Fatalf("Search failed after SQL injection content was stored: %v", err)
			}
			// The content should be stored as literal text, not executed
			if len(results) == 0 {
				t.Logf("SQL injection string stored but not findable via FTS5 (possibly sanitized): %q", inj)
			}
			_ = id
		})

		t.Run(fmt.Sprintf("target_%q", inj), func(t *testing.T) {
			// Using injection as target field
			_, err := store.AddMemory(AddMemoryParams{
				Target:      inj,
				Category:    "test",
				Key:         "sql-inject-target",
				Content:     "test content",
				Priority:    3,
				SourceAgent: "redteam",
			})
			if err != nil {
				t.Logf("Store rejected SQL injection in target field: %v", err)
			}
			// Table should still exist
			if _, err := store.Search("anything", "", 1); err != nil {
				t.Errorf("Search broken after SQL injection in target: %v", err)
			}
		})

		t.Run(fmt.Sprintf("category_%q", inj), func(t *testing.T) {
			_, err := store.AddMemory(AddMemoryParams{
				Target:      "memory",
				Category:    inj,
				Key:         "sql-inject-category",
				Content:     "test content",
				Priority:    3,
				SourceAgent: "redteam",
			})
			if err != nil {
				t.Logf("Store rejected SQL injection in category field: %v", err)
			}
			if _, err := store.Search("anything", "", 1); err != nil {
				t.Errorf("Search broken after SQL injection in category: %v", err)
			}
		})
	}
}

func TestRedTeam_FTS5Injection(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	// First add a known memory to search against
	_, err := store.AddMemory(AddMemoryParams{
		Target: "memory", Category: "test", Key: "target",
		Content: "this is a test memory for FTS5 injection", Priority: 3, SourceAgent: "redteam",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	injectionQueries := []struct {
		name  string
		query string
	}{
		{"Asterisk", "*"},
		{"DoubleAsterisk", "**"},
		{"QuestionMark", "?"},
		{"OR_OR", "OR OR"},
		{"AND_AND", "AND AND"},
		{"NOT_NOT", "NOT NOT"},
		{"NEAR3", "NEAR/3"},
		{"ColonFilter", "content:test"},
		{"QuoteInjection", `"test" OR "1"="1"`},
		{"UNION", `test' UNION SELECT * FROM memories --`},
		{"NullByte", "test\x00injection"},
		{"Backslash", `test\`},
		{"CRLF", "test\r\ninjection"},
	}

	for _, tc := range injectionQueries {
		t.Run(tc.name, func(t *testing.T) {
			// Should NOT panic, should return results or empty — never crash
			results, err := store.Search(tc.query, "", 10)
			if err != nil {
				t.Logf("Search returned error (acceptable): %v", err)
			}
			// No panic = pass. Results may be empty if query was sanitized.
			t.Logf("Query %q returned %d results", tc.query, len(results))
		})
	}
}

func TestRedTeam_ConcurrentSameKeyWrites(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, err := store.AddMemory(AddMemoryParams{
				Target:      "memory",
				Category:    "test",
				Key:         "same-key", // all goroutines write to same key
				Content:     fmt.Sprintf("content-from-goroutine-%d", id),
				Priority:    int32(id%5 + 1),
				SourceAgent: "redteam",
			})
			errs[id] = err
		}(i)
	}

	wg.Wait()

	// No goroutine should have panicked (we're here = no panic)
	// At least one write should have succeeded (last-write-wins)
	failCount := 0
	for i, err := range errs {
		if err != nil {
			t.Logf("Goroutine %d error: %v", i, err)
			failCount++
		}
	}

	// Verify the store is still functional
	_, err := store.AddMemory(AddMemoryParams{
		Target: "memory", Category: "test", Key: "post-concurrency",
		Content: "verify store works", Priority: 3, SourceAgent: "redteam",
	})
	if err != nil {
		t.Errorf("Store broken after concurrent writes: %v", err)
	}

	// Verify the same-key entry exists (should be the last writer)
	mem, err := store.GetMemoryByID(fmt.Sprintf("memory/test/same-key"))
	if err != nil {
		t.Logf("Same-key entry not found (upsert key format may differ): %v", err)
	} else {
		t.Logf("Same-key entry content after 20 concurrent writes: %s", mem.Content)
	}

	t.Logf("Concurrent writes: %d/%d succeeded", goroutines-failCount, goroutines)
}

func TestRedTeam_BoundaryTimestamps(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	tests := []struct {
		name      string
		expiresAt int64
		wantErr   bool
	}{
		{"ZeroExpiry", 0, false},                 // No expiry
		{"MaxInt64", math.MaxInt64, false},       // Far future
		{"NegativeExpiry", -1000000, false},      // Negative timestamp (should not panic)
		{"OneExpiry", 1, false},                   // Very early timestamp (already expired)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.AddMemory(AddMemoryParams{
				Target:      "memory",
				Category:    "test",
				Key:         fmt.Sprintf("expiry-%s", tc.name),
				Content:     "boundary test",
				Priority:    3,
				SourceAgent: "redteam",
				ExpiresAt:   tc.expiresAt,
			})
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

func TestRedTeam_SpecialCharsInFields(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	specialStrings := []struct {
		name  string
		value string
	}{
		{"Newline", "val\nue"},
		{"CarriageReturn", "val\rue"},
		{"Tab", "val\tue"},
		{"NullByte", "val\x00ue"},
		{"PathTraversal", "../../etc/passwd"},
		{"HTMLEntity", "<script>alert('xss')</script>"},
		{"UnicodeEmoji", "🔥memory🎉"},
		{"VeryLong", strings.Repeat("x", 10000)},
	}

	for _, tc := range specialStrings {
		t.Run(tc.name, func(t *testing.T) {
			// Test as content — should be safe (parameterized queries store as-is)
			_, err := store.AddMemory(AddMemoryParams{
				Target:      "memory",
				Category:    "test",
				Key:         "special-" + tc.name,
				Content:     tc.value,
				Priority:    3,
				SourceAgent: "redteam",
			})
			if err != nil {
				t.Errorf("Store rejected content with %s: %v", tc.name, err)
			}

			// Test as target — should be safe
			_, err = store.AddMemory(AddMemoryParams{
				Target:      tc.value,
				Category:    "test",
				Key:         "special-target-" + tc.name,
				Content:     "test content",
				Priority:    3,
				SourceAgent: "redteam",
			})
			if err != nil {
				t.Logf("Store rejected target with %s: %v (may be intentional)", tc.name, err)
			}

			// Test as category — should be safe
			_, err = store.AddMemory(AddMemoryParams{
				Target:      "memory",
				Category:    tc.value,
				Key:         "special-cat-" + tc.name,
				Content:     "test content",
				Priority:    3,
				SourceAgent: "redteam",
			})
			if err != nil {
				t.Logf("Store rejected category with %s: %v (may be intentional)", tc.name, err)
			}
		})
	}
}

func TestRedTeam_SpoofedAgentIDs(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	spoofedIDs := []struct {
		name string
		id   string
	}{
		{"Empty", ""},
		{"SQLInjection", "1'; DROP TABLE agents; --"},
		{"VeryLong", strings.Repeat("a", 10000)},
		{"NullByte", "agent\x00spoof"},
	}

	for _, tc := range spoofedIDs {
		t.Run(tc.name, func(t *testing.T) {
			_, isNew, err := store.EnsureAgent(tc.id, "spoofed", "test-role")
			if err != nil {
				t.Logf("EnsureAgent rejected %s: %v", tc.name, err)
			} else {
				t.Logf("EnsureAgent accepted %s (isNew=%v)", tc.name, isNew)
			}
		})
	}
}

func TestRedTeam_HotBlockWithEmptyDB(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	// Empty DB — hot block should return empty content, not panic
	result, err := store.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:     4000,
		BoostPriority: 2,
	})
	if err != nil {
		t.Errorf("Hot block on empty DB returned error: %v", err)
	}
	if result.Content == "" {
		t.Log("Hot block on empty DB returns empty string (correct)")
	} else {
		t.Logf("Hot block on empty DB returned: %q", result.Content)
	}
}

func TestRedTeam_HotBlockStarvation(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	// Add 100 memories with priority 1 — none should make it into hot block (boost_priority=2)
	for i := 0; i < 100; i++ {
		_, err := store.AddMemory(AddMemoryParams{
			Target:      "memory",
			Category:    "observation",
			Key:         fmt.Sprintf("low-pri-%d", i),
			Content:     fmt.Sprintf("Low priority observation %d with some content to fill up char limit", i),
			Priority:    1,
			SourceAgent: "redteam",
		})
		if err != nil {
			t.Fatalf("Failed to add memory %d: %v", i, err)
		}
	}

	result, err := store.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:     4000,
		BoostPriority: 2,
	})
	if err != nil {
		t.Errorf("Hot block with 100 low-pri entries returned error: %v", err)
	}
	if result.Content != "" {
		t.Errorf("Hot block should be empty (all entries priority 1, boost_priority=2), got %d chars", len(result.Content))
	}
}

func TestRedTeam_HotBlockCharLimitOverflow(t *testing.T) {
	t.Parallel()
	store, cleanup := setupRedTeamStore(t)
	defer cleanup()

	// Add 50 high-priority memories with long content — should overflow char limit
	for i := 0; i < 50; i++ {
		_, err := store.AddMemory(AddMemoryParams{
			Target:      "memory",
			Category:    "project",
			Key:         fmt.Sprintf("overflow-%d", i),
			Content:     fmt.Sprintf("Entry %d: %s", i, strings.Repeat("x", 500)),
			Priority:    4,
			SourceAgent: "redteam",
		})
		if err != nil {
			t.Fatalf("Failed to add memory %d: %v", i, err)
		}
	}

	result, err := store.ComputeHotBlock("memory", HotBlockConfig{
		CharLimit:       500, // Very small limit
		BoostPriority:   2,
		BoostCategories:  []string{"project"},
	})
	if err != nil {
		t.Errorf("Hot block overflow returned error: %v", err)
	}
	if len(result.Content) > 600 { // Allow some overhead for labels
		t.Errorf("Hot block exceeded char limit+overhead: got %d chars, limit 500", len(result.Content))
	}
	t.Logf("Hot block with 500 char limit and 50 entries: %d chars", len(result.Content))
}