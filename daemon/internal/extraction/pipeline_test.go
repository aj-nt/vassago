// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package extraction

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildExtractionPrompt(t *testing.T) {
	msgs := []SessionMessage{
		{Role: "user", Content: "I prefer short commit messages"},
		{Role: "assistant", Content: "Got it. Will keep commits concise."},
		{Role: "user", Content: "My wife's name is Cait"},
		{Role: "assistant", Content: "Noted. I'll remember that."},
	}

	prompt := buildExtractionPrompt(msgs)

	// Must contain the formatted messages
	if !strings.Contains(prompt, "short commit messages") {
		t.Error("prompt should contain user message content")
	}
	if !strings.Contains(prompt, "Cait") {
		t.Error("prompt should contain factual user content")
	}
	// Must contain instructions
	if !strings.Contains(prompt, "memory") && !strings.Contains(prompt, "user") {
		t.Error("prompt should include target instructions (memory vs user)")
	}
	// Must ask for JSON output
	if !strings.Contains(prompt, "json") && !strings.Contains(prompt, "JSON") {
		t.Error("prompt should request JSON output format")
	}
}

func TestParseExtractionResponse_Valid(t *testing.T) {
	json := `{
		"memories": [
			{"key": "user_prefers_short_commits", "content": "User prefers short commit messages", "category": "preference", "target": "user", "priority": 3},
			{"key": "wife_name_cait", "content": "User's wife is named Cait", "category": "personal", "target": "user", "priority": 4}
		]
	}`

	candidates, err := parseExtractionResponse(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Key != "user_prefers_short_commits" {
		t.Errorf("first candidate key = %q", candidates[0].Key)
	}
	if candidates[1].Target != "user" {
		t.Errorf("second candidate target = %q", candidates[1].Target)
	}
}

func TestParseExtractionResponse_Invalid(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"empty json", `{}`},
		{"no memories key", `{"other": []}`},
		{"invalid json", `not json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseExtractionResponse(tt.json)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestParseExtractionResponse_EmptyMemories(t *testing.T) {
	// The prompt explicitly tells the LLM: "If nothing durable is found,
	// return an empty memories array." Empty is valid, not an error.
	candidates, err := parseExtractionResponse(`{"memories": []}`)
	if err != nil {
		t.Fatalf("unexpected error for empty memories: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestParseExtractionResponse_InvalidCandidate(t *testing.T) {
	// LLM produces a candidate missing required fields
	json := `{
		"memories": [
			{"key": "valid", "content": "ok", "category": "test", "target": "memory", "priority": 2},
			{"key": "", "content": "no key", "category": "test", "target": "memory", "priority": 2}
		]
	}`

	candidates, err := parseExtractionResponse(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return only the valid candidate, skipping the invalid one
	if len(candidates) != 1 {
		t.Fatalf("expected 1 valid candidate, got %d", len(candidates))
	}
}

func TestDeduplicateCandidates(t *testing.T) {
	existing := map[string]bool{
		"user_prefers_short_commits": true,
	}

	candidates := []Candidate{
		{Key: "user_prefers_short_commits", Content: "updated", Category: "pref", Target: "user", Priority: 3},
		{Key: "new_fact", Content: "Something new", Category: "quirk", Target: "memory", Priority: 2},
		{Key: "new_fact", Content: "duplicate candidate", Category: "quirk", Target: "memory", Priority: 3},
	}

	result := deduplicateCandidates(candidates, existing)

	if len(result) != 1 {
		t.Fatalf("expected 1 new candidate, got %d", len(result))
	}
	if result[0].Key != "new_fact" {
		t.Errorf("expected new_fact, got %q", result[0].Key)
	}
	// Highest priority version should win among duplicates
	if result[0].Priority != 3 {
		t.Errorf("expected priority 3, got %d", result[0].Priority)
	}
}

func TestDeduplicateCandidates_AllExisting(t *testing.T) {
	existing := map[string]bool{
		"a": true, "b": true,
	}

	candidates := []Candidate{
		{Key: "a", Content: "x", Category: "t", Target: "memory", Priority: 2},
		{Key: "b", Content: "x", Category: "t", Target: "memory", Priority: 2},
	}

	result := deduplicateCandidates(candidates, existing)
	if len(result) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(result))
	}
}

func TestExtractionPipeline_ExtractFromMessages(t *testing.T) {
	// This test exercises the full pipeline with a mock LLM client.
	// We simulate a session where the user shares personal information.

	msgs := []SessionMessage{
		{Role: "user", Content: "I use vim for all my editing"},
		{Role: "assistant", Content: "Noted. Any particular plugin preferences?"},
		{Role: "user", Content: "I prefer minimal config, no heavy plugins"},
	}

	// Mock LLM returns pre-baked extraction
	mockResponse := `{"memories": [{"key": "uses_vim", "content": "User uses vim for editing", "category": "preference", "target": "user", "priority": 3}, {"key": "prefers_minimal_vim", "content": "User prefers minimal vim config", "category": "preference", "target": "user", "priority": 2}]}`

	client := &mockLLMClient{response: mockResponse}
	pipeline := NewPipeline(DefaultConfig(), nil)
	pipeline.SetClient(client)

	candidates, err := pipeline.ExtractFromMessages(msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	// Verify the prompt was built and sent
	if client.calledWith == "" {
		t.Error("expected LLM client to be called with a prompt")
	}
	if !strings.Contains(client.calledWith, "vim") {
		t.Error("prompt should contain user message content")
	}
}

func TestExtractionPipeline_EmptyMessages(t *testing.T) {
	pipeline := NewPipeline(DefaultConfig(), nil)
	_, err := pipeline.ExtractFromMessages(nil)
	if err == nil {
		t.Error("expected error for empty messages")
	}
}

func TestExtractionPipeline_PartialPersist(t *testing.T) {
	// Simulate the case where the LLM returns 3 candidates, but
	// AddMemory fails for one of them. The pipeline should still
	// persist the other two and report the correct count.
	store := &mockMemoryStore{
		keys: []string{},
		addErrors: map[string]error{
			"bad_key": fmt.Errorf("constraint violation"),
		},
	}
	mockResp := `{"memories": [
		{"key": "good_one", "content": "ok 1", "category": "quirk", "target": "memory", "priority": 2},
		{"key": "bad_key", "content": "will fail", "category": "quirk", "target": "memory", "priority": 2},
		{"key": "good_two", "content": "ok 2", "category": "quirk", "target": "memory", "priority": 2}
	]}`

	cfg := DefaultConfig()
	cfg.Model = "test"
	pipeline := NewPipeline(cfg, store)
	pipeline.SetClient(&mockLLMClient{response: mockResp})

	msgs := []SessionMessage{
		{Role: "user", Content: "I use vim"},
	}

	persisted, err := pipeline.ExtractAndPersist("test-owner", "test-agent", msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if persisted != 2 {
		t.Errorf("expected 2 persisted, got %d (bad_key should be skipped, not block others)", persisted)
	}

	// Verify all three were attempted
	if len(store.addCalls) != 3 {
		t.Errorf("expected 3 AddMemory calls (one failed), got %d", len(store.addCalls))
	}
}

// ---- Mock LLM Client ----

type mockLLMClient struct {
	response   string
	calledWith string
}

func (m *mockLLMClient) Complete(prompt string) (string, error) {
	m.calledWith = prompt
	return m.response, nil
}

// ---- Mock Memory Store ----

type mockMemoryStore struct {
	keys      []string
	addErrors map[string]error // key → error to return when key matches
	addCalls  []string         // track keys that were persisted
}

func (m *mockMemoryStore) AddMemory(owner, target, category, key, content string, priority int32, sourceAgent string) (string, error) {
	m.addCalls = append(m.addCalls, key)
	if err, ok := m.addErrors[key]; ok {
		return "", err
	}
	return "id-" + key, nil
}

func (m *mockMemoryStore) ListMemoryKeys(owner string) ([]string, error) {
	return m.keys, nil
}
