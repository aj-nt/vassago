// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- Export Tests ---

func TestExportJSON_emptyStore(t *testing.T) {
	s := testStore(t)
	path := filepath.Join(t.TempDir(), "export.json")

	count, err := s.ExportJSON(path)
	if err != nil {
		t.Fatalf("ExportJSON empty store: %v", err)
	}
	if count != 0 {
		t.Errorf("ExportJSON empty store: got %d entries, want 0", count)
	}

	// Verify file contains valid JSON with empty array
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	var entries []MemoryExport
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("unmarshaled empty export: got %d entries, want 0", len(entries))
	}
}

func TestExportJSON_roundTrip(t *testing.T) {
	s := testStore(t)

	// Add memories
	m1, err := s.AddMemory(AddMemoryParams{
		Target: "memory", Category: "environment", Key: "test_export_1",
		Content: "First memory for export test", Priority: 4, SourceAgent: "test",
	})
	if err != nil {
		t.Fatalf("AddMemory 1: %v", err)
	}
	m2, err := s.AddMemory(AddMemoryParams{
		Target: "user", Category: "user", Key: "test_export_user",
		Content: "User memory for export test", Priority: 4, SourceAgent: "test",
	})
	if err != nil {
		t.Fatalf("AddMemory 2: %v", err)
	}

	// Add a memory with expiry
	s.AddMemory(AddMemoryParams{
		Target: "memory", Category: "observation", Key: "test_export_expiry",
		Content: "Observation with expiry", Priority: 2, SourceAgent: "test",
		ExpiresAt: 1893456000, // 2030-01-01
	})

	// Export
	path := filepath.Join(t.TempDir(), "export.json")
	count, err := s.ExportJSON(path)
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	if count != 3 {
		t.Errorf("ExportJSON: got %d entries, want 3", count)
	}

	// Verify the export file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	var entries []MemoryExport
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}

	// Verify entries contain expected data
	foundIDs := map[string]bool{m1.ID: true, m2.ID: true}
	expiryFound := false
	for _, e := range entries {
		if foundIDs[e.ID] {
			if e.Key == "test_export_1" && e.Target != "memory" {
				t.Errorf("entry %s: target = %q, want 'memory'", e.ID, e.Target)
			}
			if e.Key == "test_export_user" && e.Target != "user" {
				t.Errorf("entry %s: target = %q, want 'user'", e.ID, e.Target)
			}
		}
		if e.Key == "test_export_expiry" && e.ExpiresAt > 0 {
			expiryFound = true
		}
	}
	if !expiryFound {
		t.Error("export did not include entry with expires_at > 0")
	}
}

func TestImportJSON_freshStore(t *testing.T) {
	src := testStore(t)

	// Add memories to source
	src.AddMemory(AddMemoryParams{
		Target: "memory", Category: "environment", Key: "import_test_1",
		Content: "Memory to import", Priority: 3, SourceAgent: "test",
	})
	src.AddMemory(AddMemoryParams{
		Target: "user", Category: "user", Key: "import_test_user",
		Content: "User memory to import", Priority: 4, SourceAgent: "test",
	})

	// Export from source
	path := filepath.Join(t.TempDir(), "export.json")
	if _, err := src.ExportJSON(path); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	// Import into fresh store
	dst := testStore(t)
	result, err := dst.ImportJSON(path, false)
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
	if result.Imported != 2 {
		t.Errorf("ImportJSON: imported %d, want 2", result.Imported)
	}
	if result.Skipped != 0 {
		t.Errorf("ImportJSON: skipped %d, want 0", result.Skipped)
	}

	// Verify imported memories are searchable
	entries, err := dst.ListMemories("default", "", "", 0)
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("after import: got %d entries, want 2", len(entries))
	}
}

func TestImportJSON_skipExisting(t *testing.T) {
	dst := testStore(t)

	// Add an existing memory with a specific key
	dst.AddMemory(AddMemoryParams{
		Target: "memory", Category: "environment", Key: "conflict_key",
		Content: "Original content", Priority: 3, SourceAgent: "original",
	})

	// Create an export with the same key but different content
	src := testStore(t)
	src.AddMemory(AddMemoryParams{
		Target: "memory", Category: "environment", Key: "conflict_key",
		Content: "New content from import", Priority: 4, SourceAgent: "import",
	})

	path := filepath.Join(t.TempDir(), "export.json")
	src.ExportJSON(path)

	// Import with skipExisting=true — should NOT overwrite
	result, err := dst.ImportJSON(path, true)
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
	if result.Skipped == 0 {
		t.Error("ImportJSON with skipExisting: expected skipped entries, got 0")
	}

	// Verify original content preserved
	entries, _ := dst.ListMemories("default", "memory", "", 0)
	for _, e := range entries {
		if e.Key == "conflict_key" && e.Content == "New content from import" {
			t.Error("ImportJSON skipExisting=true: content was overwritten")
		}
	}
}

func TestImportJSON_overwriteExisting(t *testing.T) {
	dst := testStore(t)

	// Add an existing memory
	dst.AddMemory(AddMemoryParams{
		Target: "memory", Category: "environment", Key: "conflict_key",
		Content: "Original content", Priority: 3, SourceAgent: "original",
	})

	// Create an export that overwrites
	src := testStore(t)
	src.AddMemory(AddMemoryParams{
		Target: "memory", Category: "environment", Key: "conflict_key",
		Content: "Updated content from import", Priority: 5, SourceAgent: "import",
	})

	path := filepath.Join(t.TempDir(), "export.json")
	src.ExportJSON(path)

	// Import with skipExisting=false — should overwrite
	result, err := dst.ImportJSON(path, false)
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}

	// Verify content was overwritten
	entries, _ := dst.ListMemories("default", "memory", "", 0)
	for _, e := range entries {
		if e.Key == "conflict_key" {
			if e.Content != "Updated content from import" {
				t.Errorf("ImportJSON overwrite: content = %q, want updated content", e.Content)
			}
			if e.Priority != 5 {
				t.Errorf("ImportJSON overwrite: priority = %d, want 5", e.Priority)
			}
		}
	}
	_ = result
}

func TestImportJSON_nonexistentFile(t *testing.T) {
	s := testStore(t)
	_, err := s.ImportJSON("/nonexistent/path/export.json", false)
	if err == nil {
		t.Error("ImportJSON nonexistent file: expected error, got nil")
	}
}

func TestExportJSON_invalidPath(t *testing.T) {
	s := testStore(t)
	_, err := s.ExportJSON("/nonexistent/dir/export.json")
	if err == nil {
		t.Error("ExportJSON invalid path: expected error, got nil")
	}
}

func TestRoundTrip_preservesAllFields(t *testing.T) {
	s := testStore(t)

	// Add a memory with all fields populated
	m, err := s.AddMemory(AddMemoryParams{
		Target: "memory", Category: "project", Key: "roundtrip_full",
		Content: "Full field test with special chars: é, 中文, 🎉", Priority: 5,
		SourceAgent: "agent-123", ExpiresAt: 1893456000,
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Export
	path := filepath.Join(t.TempDir(), "rt.json")
	if _, err := s.ExportJSON(path); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	// Import into fresh store
	dst := testStore(t)
	if _, err := dst.ImportJSON(path, false); err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}

	// Verify all fields preserved
	imported, err := dst.Search("default", "roundtrip_full", "memory", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("expected 1 result, got %d", len(imported))
	}
	got := imported[0]
	if got.Target != "memory" || got.Category != "project" || got.Key != "roundtrip_full" {
		t.Errorf("basic fields mismatch: target=%s category=%s key=%s", got.Target, got.Category, got.Key)
	}
	if got.Content != "Full field test with special chars: é, 中文, 🎉" {
		t.Errorf("content mismatch: %q", got.Content)
	}
	if got.Priority != 5 {
		t.Errorf("priority = %d, want 5", got.Priority)
	}
	if got.ExpiresAt != 1893456000 {
		t.Errorf("expires_at = %d, want 1893456000", got.ExpiresAt)
	}
	_ = m.ID // just to use m
}

// --- Search Validation Tests ---
// These verify that every entry in a populated store is findable via FTS5.

func TestSearchValidation_everyEntryFindable(t *testing.T) {
	s := testStore(t)

	// Add a diverse set of memories with different content patterns
	testEntries := []AddMemoryParams{
		{Target: "memory", Category: "environment", Key: "os_version", Content: "Ubuntu 22.04 LTS server", Priority: 3, SourceAgent: "test"},
		{Target: "memory", Category: "quirk", Key: "path_bug", Content: "Paths with colons break FTS5: C:\\Users\\test", Priority: 4, SourceAgent: "test"},
		{Target: "memory", Category: "project", Key: "mnemosyne_status", Content: "Mnemosyne Phase 20: safety hardening", Priority: 4, SourceAgent: "test"},
		{Target: "user", Category: "user", Key: "name", Content: "AJ prefers concise answers", Priority: 4, SourceAgent: "test"},
		{Target: "memory", Category: "observation", Key: "weather_today", Content: "Temperature 72°F, sunny, light breeze", Priority: 2, SourceAgent: "test"},
		{Target: "memory", Category: "quirk", Key: "unicode_test", Content: "Special chars: αβγ δζη ٩٠١ 🔧⚙️", Priority: 3, SourceAgent: "test"},
		{Target: "memory", Category: "environment", Key: "api_endpoint", Content: "https://api.example.com:8443/v2/endpoint", Priority: 3, SourceAgent: "test"},
	}

	for _, p := range testEntries {
		if _, err := s.AddMemory(p); err != nil {
			t.Fatalf("AddMemory(%s): %v", p.Key, err)
		}
	}

	// Validate: search for each entry by a distinctive word from its content
	searchTests := []struct {
		query string
		key   string // expected key of the match
	}{
		{"Ubuntu", "os_version"},
		{"colons", "path_bug"},
		{"hardening", "mnemosyne_status"},
		{"concise", "name"},
		{"72°F", "weather_today"},
		{"αβγ", "unicode_test"},
		{"api.example.com", "api_endpoint"},
	}

	for _, st := range searchTests {
		results, err := s.Search("default", st.query, "", 10)
		if err != nil {
			t.Errorf("Search(%q): error: %v", st.query, err)
			continue
		}
		found := false
		for _, r := range results {
			if r.Key == st.key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Search(%q): expected to find key=%s, got %d results", st.query, st.key, len(results))
		}
	}
}

func TestSearchValidation_keyOnlySearch(t *testing.T) {
	s := testStore(t)

	s.AddMemory(AddMemoryParams{
		Target: "memory", Category: "project", Key: "unique_keyphrase_xyz",
		Content: "Content that does not mention unique_keyphrase_xyz", Priority: 3, SourceAgent: "test",
	})

	// FTS5 indexes both content and key — searching by key should find it
	results, err := s.Search("default", "unique_keyphrase_xyz", "", 10)
	if err != nil {
		t.Fatalf("Search by key: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search by key: expected at least 1 result, got 0")
	}
}

func TestSearchValidation_categorySearch(t *testing.T) {
	s := testStore(t)

	s.AddMemory(AddMemoryParams{
		Target: "memory", Category: "quirk", Key: "category_test",
		Content: "This is a quirk entry about debugging", Priority: 3, SourceAgent: "test",
	})

	// Category is indexed in FTS5 too
	results, err := s.Search("default", "quirk", "", 10)
	if err != nil {
		t.Fatalf("Search by category: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search by category: expected at least 1 result")
	}
}

// --- Mutation Rate Limit Tests ---
