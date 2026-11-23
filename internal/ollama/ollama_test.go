// This file is part of Vassago.
// See LICENSE for license information.

package ollama

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient_SetsDefaults(t *testing.T) {
	c := NewClient(Config{
		BaseURL:     "http://localhost:11434/v1",
		Model:       "test-model",
		MaxTokens:   512,
		Temperature: 0.5,
		TimeoutSec:  30,
	})
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.model != "test-model" {
		t.Errorf("model = %q, want %q", c.model, "test-model")
	}
	if c.maxTokens != 512 {
		t.Errorf("maxTokens = %d, want 512", c.maxTokens)
	}
	if c.temperature != 0.5 {
		t.Errorf("temperature = %f, want 0.5", c.temperature)
	}
	if c.baseURL != "http://localhost:11434/v1" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://localhost:11434/v1")
	}
}

func TestNewClient_ZeroTimeoutDefaults(t *testing.T) {
	c := NewClient(Config{
		BaseURL:    "http://localhost:11434/v1",
		Model:      "test",
		TimeoutSec: 0,
	})
	// TimeoutSec=0 should still create a client (caller sets defaults first)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestChat_MissingChoices(t *testing.T) {
	c := NewClient(Config{
		BaseURL:    "http://localhost:11434/v1",
		Model:      "test",
		MaxTokens:  100,
		TimeoutSec: 10,
	})
	// This will fail with a real HTTP call — unit test, not integration.
	// The test documents the expected API shape.
	_ = c
}

// The client had no behavioural tests: the two copies that existed (here and cmd/worker) were
// never exercised by anything but a live model, so every rule below lived only in the code.
// It is now the single LLM path for the worker, so the rules are pinned.

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(Config{BaseURL: srv.URL, Model: "m", MaxTokens: 7, Temperature: 0.25, TimeoutSec: 5})
}

func TestChatSendsTheConfiguredRequest(t *testing.T) {
	var got chatRequest
	var gotPath, gotCT string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotCT = r.URL.Path, r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	})

	out, err := c.Chat([]Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out != "hello" {
		t.Fatalf("content = %q, want hello", out)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if got.Model != "m" || got.MaxTokens != 7 || got.Temperature != 0.25 || len(got.Messages) != 1 {
		t.Errorf("request body = %+v", got)
	}
}

// The model puts its answer in a separate field and leaves content empty. Returning the error
// instead would make a working model look like a broken one.
func TestChatFallsBackToTheReasoningField(t *testing.T) {
	for _, key := range []string{"reasoning", "reasoning_content"} {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","` + key + `":"thought"}}]}`))
		})
		out, err := c.Chat([]Message{{Role: "user", Content: "hi"}})
		if err != nil {
			t.Fatalf("%s: Chat: %v", key, err)
		}
		if out != "thought" {
			t.Fatalf("%s: content = %q, want thought", key, out)
		}
	}
}

func TestChatRefusesResponsesItCannotUse(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"no choices", `{"choices":[]}`, "no choices in response"},
		{"empty content", `{"choices":[{"message":{"content":""}}]}`, "empty response content"},
		{"not json", `not json`, "unmarshal response"},
	}
	for _, tc := range cases {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(tc.body))
		})
		if _, err := c.Chat(nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

// A non-200 has to say what came back, because "http 500" alone has never diagnosed anything.
func TestChatReportsTheStatusAndBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"model not loaded"}`))
	})
	_, err := c.Chat(nil)
	if err == nil || !strings.Contains(err.Error(), "http 502") || !strings.Contains(err.Error(), "model not loaded") {
		t.Fatalf("err = %v", err)
	}
}

// The worker's config defaults timeout_sec to 120, so this fallback is what stands between an
// unset timeout and no timeout at all — an LLM call that hangs instead of failing.
func TestNewClientZeroTimeoutIsNotNoTimeout(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://x", Model: "m"})
	if c.timeout != 120*time.Second {
		t.Fatalf("timeout = %v, want 120s", c.timeout)
	}
	if c.client.Timeout != 120*time.Second {
		t.Fatalf("http client timeout = %v, want 120s", c.client.Timeout)
	}
}
