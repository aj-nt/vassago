// This file is part of Vassago.
// See LICENSE for license information.

package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aj-nt/vassago/daemon/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStats_returnsEmptyDashboard(t *testing.T) {
	s := testStore(t)
	h := New(s, "/tmp/test.db")
	ts := httptest.NewServer(http.HandlerFunc(h.handleStats))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET /api/stats: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var stats Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}

	if stats.TotalMemories != 0 {
		t.Errorf("TotalMemories = %d, want 0 (empty store)", stats.TotalMemories)
	}
	if stats.SchemaVersion != store.SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", stats.SchemaVersion, store.SchemaVersion)
	}
}

func TestStats_withMemories(t *testing.T) {
	s := testStore(t)

	// Add some memories
	s.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "env",
		Key: "go-path", Content: "/opt/go", Priority: 3, SourceAgent: "test",
	})
	s.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "env",
		Key: "py-path", Content: "/opt/py", Priority: 3, SourceAgent: "test",
	})
	s.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "bug",
		Key: "fts5", Content: "FTS5 fix", Priority: 4, SourceAgent: "test",
	})
	s.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "user", Category: "pref",
		Key: "editor", Content: "nvim", Priority: 3, SourceAgent: "test",
	})

	h := New(s, "/tmp/test.db")
	ts := httptest.NewServer(http.HandlerFunc(h.handleStats))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET /api/stats: %v", err)
	}
	defer resp.Body.Close()

	var stats Stats
	json.NewDecoder(resp.Body).Decode(&stats)

	if stats.MemoryCount != 3 {
		t.Errorf("MemoryCount = %d, want 3", stats.MemoryCount)
	}
	if stats.UserCount != 1 {
		t.Errorf("UserCount = %d, want 1", stats.UserCount)
	}
	if stats.ByCategory["env"] != 2 {
		t.Errorf("env count = %d, want 2", stats.ByCategory["env"])
	}
	if stats.ByCategory["bug"] != 1 {
		t.Errorf("bug count = %d, want 1", stats.ByCategory["bug"])
	}
}

func TestUI_returnsHTML(t *testing.T) {
	s := testStore(t)
	h := New(s, "/tmp/test.db")
	ts := httptest.NewServer(http.HandlerFunc(h.handleUI))
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}
