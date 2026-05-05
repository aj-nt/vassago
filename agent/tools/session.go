// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"fmt"
	"time"

	"github.com/aj-nt/mnemosyne/client"
)

// RegisterSessionTools adds session-related tool handlers to the registry.
func RegisterSessionTools(registry *ToolRegistry, mnemo *client.Client) {
	registry.Register("SearchSessions", makeSearchSessions(mnemo), SearchSessionsSchema)
	registry.Register("ListSessions", makeListSessions(mnemo), ListSessionsSchema)
	registry.Register("GetSession", makeGetSession(mnemo), GetSessionSchema)
}

// --- SearchSessions ---

func makeSearchSessions(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		query := strArg(args, "query", "")
		limit := intArg(args, "limit", 10)

		if query == "" {
			return "query is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()

		sessions, err := mnemo.SearchSessions(ctx, query, int32(limit))
		if err != nil {
			return fmt.Sprintf("SearchSessions failed: %v", err), true
		}
		if len(sessions) == 0 {
			return "No sessions found.", false
		}

		result := fmt.Sprintf("Found %d sessions:\n", len(sessions))
		for _, s := range sessions {
			title := s.Title
			if title == "" {
				title = "(untitled)"
			}
			result += fmt.Sprintf("- [%s] %s (%d messages, last active %s)\n",
				s.ID,
				title,
				s.MessageCount,
				formatTime(s.LastActive),
			)
		}
		return result, false
	}
}

// --- ListSessions ---

func makeListSessions(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		limit := intArg(args, "limit", 10)

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()

		sessions, err := mnemo.ListRecentSessions(ctx, int32(limit))
		if err != nil {
			return fmt.Sprintf("ListSessions failed: %v", err), true
		}
		if len(sessions) == 0 {
			return "No sessions found.", false
		}

		result := fmt.Sprintf("Recent sessions (%d):\n", len(sessions))
		for _, s := range sessions {
			title := s.Title
			if title == "" {
				title = "(untitled)"
			}
			result += fmt.Sprintf("- [%s] %s (%d messages, last active %s)\n",
				s.ID,
				title,
				s.MessageCount,
				formatTime(s.LastActive),
			)
		}
		return result, false
	}
}

// --- GetSession ---

func makeGetSession(mnemo *client.Client) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		sessionID := strArg(args, "session_id", "")
		includeMessages := boolArg(args, "include_messages", false)

		if sessionID == "" {
			return "session_id is required", true
		}

		ctx, cancel := withTimeout(DefaultTimeout)
		defer cancel()

		session, err := mnemo.GetSession(ctx, sessionID)
		if err != nil {
			return fmt.Sprintf("GetSession failed: %v", err), true
		}

		title := session.Title
		if title == "" {
			title = "(untitled)"
		}

		result := fmt.Sprintf("Session: %s\nTitle: %s\nSource: %s\nMessages: %d\nCreated: %s\nLast active: %s",
			session.ID,
			title,
			session.Source,
			len(session.Messages),
			formatTime(session.CreatedAt),
			formatTime(session.LastActiveAt),
		)

		if includeMessages && len(session.Messages) > 0 {
			result += "\n\nMessages:\n"
			for _, m := range session.Messages {
				content := m.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				result += fmt.Sprintf("[%s] %s\n", m.Role, content)
			}
		}

		return result, false
	}
}

// --- Schemas ---

var SearchSessionsSchema = map[string]interface{}{
	"name":        "SearchSessions",
	"description": "Search past conversation sessions by title. Returns matching session IDs, titles, and metadata. Use this to find what was discussed previously.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query to match against session titles",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 10)",
			},
		},
		"required": []string{"query"},
	},
}

var ListSessionsSchema = map[string]interface{}{
	"name":        "ListSessions",
	"description": "List recent conversation sessions. Returns the most recently active sessions with titles and metadata.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of sessions to return (default: 10)",
			},
		},
		"required": []string{},
	},
}

var GetSessionSchema = map[string]interface{}{
	"name":        "GetSession",
	"description": "Get details about a specific session, including its messages. Use this to review what was discussed in a past conversation.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "The session ID to look up",
			},
			"include_messages": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to include the session messages (default: false)",
			},
		},
		"required": []string{"session_id"},
	},
}

// --- Helpers ---

// formatTime converts a Unix timestamp to a human-readable relative time.
func formatTime(unixSec int64) string {
	if unixSec == 0 {
		return "unknown"
	}
	t := time.Unix(unixSec, 0)
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(duration.Hours()))
	case duration < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(duration.Hours()/24))
	default:
		return t.Format("Jan 2, 2006")
	}
}

// boolArg extracts a boolean argument from the args map, with a default value.
func boolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	val, ok := args[key]
	if !ok {
		return defaultVal
	}
	switch v := val.(type) {
	case bool:
		return v
	default:
		return defaultVal
	}
}