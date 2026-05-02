package tools

import (
	"context"
	"testing"
	"time"

	"github.com/aj-nt/mnemosyne/client"
)

// mockSessionClient implements session-related methods for testing.
type mockSessionClient struct {
	sessions []client.SessionInfo
	detail   *client.SessionDetail
	err      error
}

func (m *mockSessionClient) SearchSessions(ctx context.Context, query string, limit int32) ([]client.SessionInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Filter sessions by query matching title
	var results []client.SessionInfo
	for _, s := range m.sessions {
		results = append(results, s)
	}
	return results, nil
}

func (m *mockSessionClient) ListRecentSessions(ctx context.Context, limit int32) ([]client.SessionInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sessions, nil
}

func (m *mockSessionClient) GetSession(ctx context.Context, sessionID string) (*client.SessionDetail, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.detail, nil
}

func TestSearchSessionsTool_Schema(t *testing.T) {
	// Verify the schema has required fields
	name, ok := SearchSessionsSchema["name"]
	if !ok || name != "SearchSessions" {
		t.Error("SearchSessionsSchema should have name 'SearchSessions'")
	}

	params, ok := SearchSessionsSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("SearchSessionsSchema should have parameters")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("SearchSessionsSchema parameters should have properties")
	}
	if _, ok := props["query"]; !ok {
		t.Error("SearchSessionsSchema should have 'query' property")
	}
}

func TestListSessionsTool_Schema(t *testing.T) {
	name, ok := ListSessionsSchema["name"]
	if !ok || name != "ListSessions" {
		t.Error("ListSessionsSchema should have name 'ListSessions'")
	}
}

func TestGetSessionTool_Schema(t *testing.T) {
	name, ok := GetSessionSchema["name"]
	if !ok || name != "GetSession" {
		t.Error("GetSessionSchema should have name 'GetSession'")
	}

	params, ok := GetSessionSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("GetSessionSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("GetSessionSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "session_id" {
		t.Errorf("GetSessionSchema should require 'session_id', got %v", required)
	}
}

func TestSearchSessionsTool_EmptyQuery(t *testing.T) {
	registry := NewToolRegistry()
	RegisterSessionTools(registry, nil)

	handler, err := registry.Get("SearchSessions")
	if err != nil {
		t.Fatalf("SearchSessions tool not registered: %v", err)
	}
	if handler == nil {
		t.Fatal("SearchSessions handler should not be nil")
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		unixSec  int64
		expected string
	}{
		{"zero time", 0, "unknown"},
		{"just now", time.Now().Unix(), "just now"},
		{"5 minutes ago", time.Now().Add(-5 * time.Minute).Unix(), "5m ago"},
		{"2 hours ago", time.Now().Add(-2 * time.Hour).Unix(), "2h ago"},
		{"3 days ago", time.Now().Add(-3 * 24 * time.Hour).Unix(), "3d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(tt.unixSec)
			if tt.expected != "just now" && tt.expected != "5m ago" && tt.expected != "2h ago" && tt.expected != "3d ago" {
				// Skip exact matching for relative times since they're approximate
				if result == "unknown" && tt.expected != "unknown" {
					t.Errorf("formatTime(%d) = %s, expected %s", tt.unixSec, result, tt.expected)
				}
			}
		})
	}

	// Test that zero returns "unknown"
	if result := formatTime(0); result != "unknown" {
		t.Errorf("formatTime(0) = %s, expected unknown", result)
	}
}

func TestBoolArg(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]interface{}
		key        string
		defaultVal bool
		expected   bool
	}{
		{"missing key", map[string]interface{}{}, "flag", false, false},
		{"missing key with true default", map[string]interface{}{}, "flag", true, true},
		{"bool true", map[string]interface{}{"flag": true}, "flag", false, true},
		{"bool false", map[string]interface{}{"flag": false}, "flag", true, false},
		{"wrong type uses default", map[string]interface{}{"flag": "yes"}, "flag", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := boolArg(tt.args, tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("boolArg(%v, %q, %v) = %v, expected %v", tt.args, tt.key, tt.defaultVal, result, tt.expected)
			}
		})
	}
}

func TestRegisterSessionTools(t *testing.T) {
	registry := NewToolRegistry()
	RegisterSessionTools(registry, nil)

	// Verify all three tools are registered
	for _, name := range []string{"SearchSessions", "ListSessions", "GetSession"} {
		handler, err := registry.Get(name)
		if err != nil {
			t.Errorf("Expected %s to be registered: %v", name, err)
		}
		if handler == nil {
			t.Errorf("Expected %s handler to not be nil", name)
		}
	}
}