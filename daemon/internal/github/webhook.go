// This file is part of Vassago.
// See LICENSE for license information.

package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookSecret is the shared secret for HMAC-SHA256 validation.
// Set from daemon config. Empty means no validation (dev mode).
var WebhookSecret string

// Handler processes GitHub webhook events.
type Handler struct {
	// OnEvent is called when a validated GitHub event is received.
	// Returns the memory content to save (empty = skip) and an error.
	OnEvent func(eventType string, payload []byte) (memory string, err error)
}

// NewHandler creates a new GitHub webhook handler.
func NewHandler(onEvent func(string, []byte) (string, error)) *Handler {
	return &Handler{OnEvent: onEvent}
}

// ServeHTTP handles POST /github/webhook requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")

	// Validate HMAC signature
	if WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !validateSignature(body, sig, WebhookSecret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Process the event
	memory, err := h.OnEvent(eventType, body)
	if err != nil {
		http.Error(w, fmt.Sprintf("event processing failed: %v", err), http.StatusInternalServerError)
		return
	}

	if memory != "" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","memory_saved":true}`))
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","memory_saved":false}`))
	}
}

// validateSignature checks an HMAC-SHA256 signature.
func validateSignature(payload []byte, signature, secret string) bool {
	if signature == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// FormatIssueMemory formats a GitHub issue event as a memory.
func FormatIssueMemory(action, repo, title, body, url, user string, number int) string {
	prefix := "GitHub:"
	switch action {
	case "opened":
		prefix = "GitHub issue opened:"
	case "closed":
		prefix = "GitHub issue closed:"
	case "reopened":
		prefix = "GitHub issue reopened:"
	}
	summary := fmt.Sprintf("%s %s #%d — %s by %s\n%s", prefix, repo, number, title, user, url)
	if body != "" {
		summary += "\n" + truncateForMemory(body, 500)
	}
	return summary
}

// FormatPROMemory formats a PR event as a memory.
func FormatPROMemory(action, repo, title, body, url, user string, number int) string {
	prefix := "GitHub PR:"
	switch action {
	case "opened":
		prefix = "GitHub PR opened:"
	case "closed":
		prefix = "GitHub PR closed:"
	case "merged":
		return fmt.Sprintf("GitHub PR merged: %s #%d — %s by %s\n%s", repo, number, title, user, url)
	}
	summary := fmt.Sprintf("%s %s #%d — %s by %s\n%s", prefix, repo, number, title, user, url)
	if body != "" {
		summary += "\n" + truncateForMemory(body, 500)
	}
	return summary
}

// ParseIssueEvent parses a GitHub issues event payload into a memory string.
func ParseIssueEvent(payload []byte) (string, error) {
	var event struct {
		Action string `json:"action"`
		Issue  struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
			HTMLURL string `json:"html_url"`
		} `json:"issue"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", fmt.Errorf("parse issue event: %w", err)
	}
	if event.Issue.Number == 0 {
		return "", nil // skip events with no issue
	}
	return FormatIssueMemory(
		event.Action,
		event.Repository.FullName,
		event.Issue.Title,
		event.Issue.Body,
		event.Issue.HTMLURL,
		event.Issue.User.Login,
		event.Issue.Number,
	), nil
}

// ParsePREvent parses a GitHub pull_request event payload into a memory string.
func ParsePREvent(payload []byte) (string, error) {
	var event struct {
		Action string `json:"action"`
		PR     struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
			HTMLURL string `json:"html_url"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", fmt.Errorf("parse PR event: %w", err)
	}
	if event.PR.Number == 0 {
		return "", nil
	}
	return FormatPROMemory(
		event.Action,
		event.Repository.FullName,
		event.PR.Title,
		event.PR.Body,
		event.PR.HTMLURL,
		event.PR.User.Login,
		event.PR.Number,
	), nil
}

func truncateForMemory(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
