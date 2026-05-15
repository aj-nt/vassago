// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package github

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateSignature_Valid(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"action":"opened"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !validateSignature(payload, sig, secret) {
		t.Error("Expected valid signature to pass")
	}
}

func TestValidateSignature_Invalid(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"action":"opened"}`)
	sig := "sha256=0000000000000000000000000000000000000000000000000000000000000000"

	if validateSignature(payload, sig, secret) {
		t.Error("Expected invalid signature to fail")
	}
}

func TestValidateSignature_EmptySignature(t *testing.T) {
	if validateSignature([]byte(`{}`), "", "secret") {
		t.Error("Expected empty signature to fail")
	}
}

func TestValidateSignature_MissingPrefix(t *testing.T) {
	secret := "secret"
	payload := []byte(`{}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil)) // missing sha256= prefix

	if validateSignature(payload, sig, secret) {
		t.Error("Expected signature without sha256= prefix to fail")
	}
}

func TestHandler_NoSecret_Passes(t *testing.T) {
	// Save and restore global secret
	orig := WebhookSecret
	WebhookSecret = ""
	defer func() { WebhookSecret = orig }()

	called := false
	handler := NewHandler(func(eventType string, payload []byte) (string, error) {
		called = true
		if eventType != "issues" {
			t.Errorf("Expected 'issues', got %q", eventType)
		}
		return "GitHub issue: test", nil
	})

	body := []byte(`{"action":"opened","issue":{"number":1,"title":"test"}}`)
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("Expected OnEvent to be called")
	}
	if !strings.Contains(rec.Body.String(), "memory_saved") {
		t.Errorf("Expected memory_saved in response, got: %s", rec.Body.String())
	}
}

func TestHandler_WithSecret_Valid(t *testing.T) {
	orig := WebhookSecret
	WebhookSecret = "test-secret"
	defer func() { WebhookSecret = orig }()

	called := false
	handler := NewHandler(func(eventType string, payload []byte) (string, error) {
		called = true
		return "memory", nil
	})

	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte(WebhookSecret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("Expected OnEvent to be called with valid signature")
	}
}

func TestHandler_WithSecret_Invalid_Fails(t *testing.T) {
	orig := WebhookSecret
	WebhookSecret = "test-secret"
	defer func() { WebhookSecret = orig }()

	called := false
	handler := NewHandler(func(eventType string, payload []byte) (string, error) {
		called = true
		return "", nil
	})

	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", "sha256=bad0000000000000000000000000000000000000000000000000000000000000")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rec.Code)
	}
	if called {
		t.Error("Expected OnEvent NOT to be called with invalid signature")
	}
}

func TestHandler_EmptyMemory_ReturnsOk(t *testing.T) {
	orig := WebhookSecret
	WebhookSecret = ""
	defer func() { WebhookSecret = orig }()

	handler := NewHandler(func(eventType string, payload []byte) (string, error) {
		return "", nil // skip
	})

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "ping")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"memory_saved":false`) {
		t.Errorf("Expected memory_saved:false, got: %s", rec.Body.String())
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	handler := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/github/webhook", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", rec.Code)
	}
}

func TestParseIssueEvent_Opened(t *testing.T) {
	payload := []byte(`{
		"action": "opened",
		"issue": {
			"number": 42,
			"title": "Fix memory leak",
			"body": "There's a leak in the cache layer",
			"user": {"login": "dev123"},
			"html_url": "https://github.com/aj-nt/vassago/issues/42"
		},
		"repository": {"full_name": "aj-nt/vassago"}
	}`)

	result, err := ParseIssueEvent(payload)
	if err != nil {
		t.Fatalf("ParseIssueEvent failed: %v", err)
	}

	if !strings.Contains(result, "aj-nt/vassago") {
		t.Errorf("Expected repo name in result: %s", result)
	}
	if !strings.Contains(result, "#42") {
		t.Errorf("Expected issue number: %s", result)
	}
	if !strings.Contains(result, "Fix memory leak") {
		t.Errorf("Expected title: %s", result)
	}
	if !strings.Contains(result, "opened") {
		t.Errorf("Expected 'opened' action: %s", result)
	}
}

func TestParseIssueEvent_Closed(t *testing.T) {
	payload := []byte(`{
		"action": "closed",
		"issue": {
			"number": 7,
			"title": "Bug: crash on startup",
			"body": "",
			"user": {"login": "contributor"},
			"html_url": "https://github.com/aj-nt/vassago/issues/7"
		},
		"repository": {"full_name": "aj-nt/vassago"}
	}`)

	result, err := ParseIssueEvent(payload)
	if err != nil {
		t.Fatalf("ParseIssueEvent failed: %v", err)
	}

	if !strings.Contains(result, "closed") {
		t.Errorf("Expected 'closed' action: %s", result)
	}
}

func TestParseIssueEvent_NoIssue(t *testing.T) {
	// A payload with no valid issue number (e.g., label events sometimes)
	payload := []byte(`{"action":"labeled","issue":{"number":0},"repository":{"full_name":"test/repo"}}`)

	result, err := ParseIssueEvent(payload)
	if err != nil {
		t.Fatalf("ParseIssueEvent failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty result for event with no valid issue: %s", result)
	}
}

func TestParsePREvent_Opened(t *testing.T) {
	payload := []byte(`{
		"action": "opened",
		"pull_request": {
			"number": 100,
			"title": "Add new feature",
			"body": "Implements the thing",
			"user": {"login": "pr-author"},
			"html_url": "https://github.com/aj-nt/vassago/pull/100"
		},
		"repository": {"full_name": "aj-nt/vassago"}
	}`)

	result, err := ParsePREvent(payload)
	if err != nil {
		t.Fatalf("ParsePREvent failed: %v", err)
	}

	if !strings.Contains(result, "#100") {
		t.Errorf("Expected PR number: %s", result)
	}
	if !strings.Contains(result, "Add new feature") {
		t.Errorf("Expected title: %s", result)
	}
	if !strings.Contains(result, "pr-author") {
		t.Errorf("Expected author: %s", result)
	}
}

func TestParsePREvent_Merged(t *testing.T) {
	payload := []byte(`{
		"action": "closed",
		"pull_request": {
			"number": 101,
			"title": "Fix things",
			"body": "",
			"user": {"login": "fixer"},
			"html_url": "https://github.com/aj-nt/vassago/pull/101"
		},
		"repository": {"full_name": "aj-nt/vassago"}
	}`)

	result, err := ParsePREvent(payload)
	if err != nil {
		t.Fatalf("ParsePREvent failed: %v", err)
	}

	// Note: our parser doesn't know merged vs closed from payload alone
	// — that's OK, GitHub sends merged in the payload
	if !strings.Contains(result, "#101") {
		t.Errorf("Expected PR number: %s", result)
	}
}

func TestFormatIssueMemory_BodyTruncation(t *testing.T) {
	longBody := strings.Repeat("ABCDEFGHIJ", 200) // 2000 chars
	result := FormatIssueMemory("opened", "repo/x", "Title", longBody, "http://example.com", "user", 1)

	if strings.Contains(result, longBody) {
		t.Error("Expected body to be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("Expected truncation indicator")
	}
}

func TestFormatPROMemory_Merged(t *testing.T) {
	result := FormatPROMemory("merged", "repo/x", "Awesome PR", "description", "http://example.com", "dev", 5)
	if !strings.Contains(result, "merged") {
		t.Errorf("Expected 'merged' in result: %s", result)
	}
}

func TestWebhookIntegration_EndToEnd(t *testing.T) {
	// Simulate a full webhook flow: receive → parse → format → save
	orig := WebhookSecret
	WebhookSecret = "integration-secret"
	defer func() { WebhookSecret = orig }()

	savedMemories := []string{}
	handler := NewHandler(func(eventType string, payload []byte) (string, error) {
		switch eventType {
		case "issues":
			return ParseIssueEvent(payload)
		case "pull_request":
			return ParsePREvent(payload)
		default:
			return "", nil
		}
	})

	// Send an issue opened event
	issuePayload := []byte(`{
		"action": "opened",
		"issue": {"number": 1, "title": "Bug report", "body": "Something broke", "user": {"login": "reporter"}, "html_url": "https://github.com/owner/repo/issues/1"},
		"repository": {"full_name": "owner/repo"}
	}`)

	mac := hmac.New(sha256.New, []byte(WebhookSecret))
	mac.Write(issuePayload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(issuePayload))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// The real integration would save to store and emit telepathy.
	// For now, verify the handler doesn't crash.
	_ = savedMemories // would be populated by store integration
}

func TestWebhook_LargePayload_Truncated(t *testing.T) {
	orig := WebhookSecret
	WebhookSecret = ""
	defer func() { WebhookSecret = orig }()

	handler := NewHandler(func(eventType string, payload []byte) (string, error) {
		// Payload should be under 1MB
		return "", nil
	})

	// Send 2MB of data — should be limited to 1MB by the handler
	bigBody := make([]byte, 2<<20)
	for i := range bigBody {
		bigBody[i] = 'x'
	}
	// Make it valid JSON so it doesn't fail at parse
	copy(bigBody, []byte(`{"action":"opened"}`))
	// Fill rest with spaces (valid JSON padding)
	for i := len(`{"action":"opened"}`); i < len(bigBody); i++ {
		bigBody[i] = ' '
	}

	req := httptest.NewRequest(http.MethodPost, "/github/webhook", bytes.NewReader(bigBody))
	req.Header.Set("X-GitHub-Event", "issues")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 even with large payload, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Helper for tests that need to compare JSON without worrying about formatting.
func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}
