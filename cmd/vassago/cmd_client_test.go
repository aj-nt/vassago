// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	memoclient "github.com/aj-nt/vassago-sdk/client"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// testMnemoClient wraps NullMnemo and provides controlled responses for testing.
type testMnemoClient struct {
	memoclient.NullMnemo
	memories      []*pb.MemoryEntry
	sessions      []memoclient.SessionInfo
	sessionDetail *memoclient.SessionDetail
}

func (t *testMnemoClient) AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error) {
	entry := &pb.MemoryEntry{
		Id:          "test-id-" + key,
		Target:      target,
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: sourceAgent,
	}
	t.memories = append(t.memories, entry)
	return entry, nil
}

func (t *testMnemoClient) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	for _, m := range t.memories {
		if m.Id == id {
			return m, nil
		}
	}
	return nil, os.ErrNotExist
}

func (t *testMnemoClient) SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error) {
	return t.memories, nil
}

func (t *testMnemoClient) ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	return t.memories, nil
}

func (t *testMnemoClient) RemoveMemory(ctx context.Context, id string) error {
	for i, m := range t.memories {
		if m.Id == id {
			t.memories = append(t.memories[:i], t.memories[i+1:]...)
			return nil
		}
	}
	return os.ErrNotExist
}

func (t *testMnemoClient) HealthCheck(ctx context.Context) error {
	return nil
}

func (t *testMnemoClient) ListRecentSessions(ctx context.Context, limit int32) ([]memoclient.SessionInfo, error) {
	return t.sessions, nil
}

func (t *testMnemoClient) GetSession(ctx context.Context, sessionID string) (*memoclient.SessionDetail, error) {
	if t.sessionDetail != nil {
		return t.sessionDetail, nil
	}
	return &memoclient.SessionDetail{
		ID:           sessionID,
		Title:        "Test Session",
		Source:       "test",
		MessageCount: 1,
	}, nil
}

func (t *testMnemoClient) Close() error {
	return nil
}

// --- Interface satisfaction tests ---

func TestTestMnemoClientSatisfiesInterface(t *testing.T) {
	// Compile-time check that testMnemoClient implements MnemoClient
	var _ memoclient.MnemoClient = &testMnemoClient{}
}

func TestNullMnemoSatisfiesInterface(t *testing.T) {
	// Compile-time check that NullMnemo implements MnemoClient
	var _ memoclient.MnemoClient = &memoclient.NullMnemo{}
}

// --- NullMnemo method tests ---

func TestNullMnemo_AddMemory(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	_, err := n.AddMemory(ctx, "memory", "environment", "key", "content", 3, "test")
	// NullMnemo returns "vassago not connected" for write operations
	if err == nil {
		t.Error("NullMnemo.AddMemory should return error when daemon unavailable")
	}
}

func TestNullMnemo_SearchMemories(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	entries, err := n.SearchMemories(ctx, "query", "memory", 10)
	if err != nil {
		t.Errorf("NullMnemo.SearchMemories returned unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("NullMnemo.SearchMemories should return nil, got %v", entries)
	}
}

func TestNullMnemo_HealthCheck(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	err := n.HealthCheck(ctx)
	// NullMnemo returns "vassago not connected" — there's no daemon to check
	if err == nil {
		t.Error("NullMnemo.HealthCheck should return error when daemon unavailable")
	}
}

func TestNullMnemo_ListRecentSessions(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	sessions, err := n.ListRecentSessions(ctx, 10)
	if err != nil {
		t.Errorf("NullMnemo.ListRecentSessions returned unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("NullMnemo.ListRecentSessions should return nil, got %v", sessions)
	}
}

func TestNullMnemo_GetSession(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	detail, err := n.GetSession(ctx, "test-id")
	if err != nil {
		t.Errorf("NullMnemo.GetSession returned unexpected error: %v", err)
	}
	if detail != nil {
		t.Errorf("NullMnemo.GetSession should return nil, got %v", detail)
	}
}

func TestNullMnemo_Close(t *testing.T) {
	n := &memoclient.NullMnemo{}
	err := n.Close()
	if err != nil {
		t.Errorf("NullMnemo.Close should return nil, got %v", err)
	}
}

// ── UUID non-truncation tests ──────────────────────────────────────────────

func TestMemSearchShowsFullUUID(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "orchestrator_tasks", "task-1", `{"status":"ready"}`, 3)

	// Capture stdout from memSearch
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memSearch(context.Background(), client, []string{"test-query", "memory"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	fullID := "test-id-task-1"
	if !strings.Contains(output, fullID) {
		t.Errorf("search output should contain full UUID %q, got:\n%s", fullID, output)
	}
	if strings.Contains(output, "test-id-") {
		// Verify it's not just the prefix — the full ID should appear in brackets
		if !strings.Contains(output, "["+fullID+"]") {
			t.Errorf("search output should show full UUID in brackets [%s], got:\n%s", fullID, output)
		}
	}
}

func TestMemListShowsFullUUID(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "orchestrator_tasks", "task-x", `{"status":"ready"}`, 4)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memList(context.Background(), client, []string{"memory"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	fullID := "test-id-task-x"
	if !strings.Contains(output, fullID) {
		t.Errorf("list output should contain full UUID %q, got:\n%s", fullID, output)
	}
}

// ── --json output tests ────────────────────────────────────────────────────

func TestMemGetJSON(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "environment", "test-key", "json content", 5)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memGet(context.Background(), client, []string{"--json", "test-id-test-key"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("get --json output should be valid JSON: %v\nOutput: %s", err, output)
	}

	if result["id"] != "test-id-test-key" {
		t.Errorf("expected id=%q, got %v", "test-id-test-key", result["id"])
	}
	if result["content"] != "json content" {
		t.Errorf("expected content=%q, got %v", "json content", result["content"])
	}
	if result["priority"] != float64(5) {
		t.Errorf("expected priority=5, got %v", result["priority"])
	}
}

func TestMemGetJSONIncludesAllFields(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "project", "full-key", "full content", 4)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memGet(context.Background(), client, []string{"--json", "test-id-full-key"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}

	required := []string{"id", "target", "category", "key", "content", "priority", "source", "created_at", "updated_at"}
	for _, field := range required {
		if _, ok := result[field]; !ok {
			t.Errorf("JSON output missing required field: %q", field)
		}
	}
}

func TestMemSearchJSON(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "orchestrator_tasks", "search-1", `{"status":"ready"}`, 3)
	_ = addTestMemory(client, "memory", "orchestrator_tasks", "search-2", `{"status":"claimed"}`, 4)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memSearch(context.Background(), client, []string{"--json", "orchestrator", "memory"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	var results []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("search --json should output valid JSON array: %v\nOutput: %s", err, output)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Check full UUIDs are present
	for i, r := range results {
		id, ok := r["id"].(string)
		if !ok || id == "" {
			t.Errorf("result[%d]: missing or empty 'id' field", i)
		}
	}
}

func TestMemSearchJSONEmpty(t *testing.T) {
	client := &testMnemoClient{}
	// No memories added

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memSearch(context.Background(), client, []string{"--json", "nothing-here", "memory"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := strings.TrimSpace(buf.String())

	if output != "[]" {
		t.Errorf("empty search --json should output '[]', got: %s", output)
	}
}

func TestMemGetDefaultStillWorks(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "environment", "default-test", "default content", 3)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memGet(context.Background(), client, []string{"test-id-default-test"}) // no --json
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "ID:") {
		t.Error("default get output should contain 'ID:' field label")
	}
	if !strings.Contains(output, "Content:") {
		t.Error("default get output should contain 'Content:' field label")
	}
	if !strings.Contains(output, "default content") {
		t.Error("default get output should contain the content value")
	}
}

func TestMemSearchDefaultStillWorks(t *testing.T) {
	client := &testMnemoClient{}
	_ = addTestMemory(client, "memory", "orchestrator_tasks", "def-search", `{"status":"ready"}`, 3)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	memSearch(context.Background(), client, []string{"def-search", "memory"})
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "Found") {
		t.Error("default search output should contain 'Found' prefix")
	}
	if !strings.Contains(output, "test-id-def-search") {
		t.Error("default search output should contain full UUID")
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func addTestMemory(client *testMnemoClient, target, category, key, content string, priority int32) *pb.MemoryEntry {
	entry, _ := client.AddMemory(context.Background(), target, category, key, content, priority, "test-agent")
	return entry
}
