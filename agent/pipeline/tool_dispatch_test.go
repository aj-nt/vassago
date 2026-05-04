// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"fmt"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

func TestToolBudget(t *testing.T) {
	// Create a tool registry with test tools
	registry := tools.NewToolRegistry()
	registry.Register("FileRead", func(args map[string]interface{}) (string, bool) {
		return "file read result", false
	}, nil)
	registry.Register("FileSearch", func(args map[string]interface{}) (string, bool) {
		return "file search result", false
	}, nil)
	registry.Register("Execute", func(args map[string]interface{}) (string, bool) {
		return "execute result", false
	}, nil)
	registry.Register("UnlimitedTool", func(args map[string]interface{}) (string, bool) {
		return "unlimited result", false
	}, nil)

	// Create tool dispatch stage
	stage := NewToolDispatchStage(registry, &eventbus.EventBus{})

	// Test FileRead budget (max 3)
	t.Run("FileRead_budget", func(t *testing.T) {
		stage.callCounts = make(map[string]int)
		
		// First 3 calls should succeed
		for i := 0; i < 3; i++ {
			result := stage.Process(nil, agentctx.ParsedResponse{
				ToolCalls: []agentctx.ToolCall{{
					ID:       fmt.Sprintf("call%d", i),
					Name:     "FileRead",
					Arguments: []byte("{}"),
				}},
			})
			if len(result.ToolResults) != 1 || result.ToolResults[0].IsError {
				t.Errorf("FileRead call %d should succeed", i+1)
			}
		}

		// 4th call should be blocked
		result := stage.Process(nil, agentctx.ParsedResponse{
			ToolCalls: []agentctx.ToolCall{{
				ID:       "call4",
				Name:     "FileRead",
				Arguments: []byte("{}"),
			}},
		})
		if len(result.ToolResults) != 1 || !result.ToolResults[0].IsError {
			t.Error("FileRead 4th call should be blocked")
		} else if !contains(result.ToolResults[0].Result, "already called 3 times") {
			t.Errorf("Expected budget exceeded message, got: %s", result.ToolResults[0].Result)
		}
	})

	// Test Execute budget (max 2)
	t.Run("Execute_budget", func(t *testing.T) {
		stage.callCounts = make(map[string]int)
		
		// First 2 calls should succeed
		for i := 0; i < 2; i++ {
			result := stage.Process(nil, agentctx.ParsedResponse{
				ToolCalls: []agentctx.ToolCall{{
					ID:       fmt.Sprintf("call%d", i),
					Name:     "Execute",
					Arguments: []byte("{}"),
				}},
			})
			if len(result.ToolResults) != 1 || result.ToolResults[0].IsError {
				t.Errorf("Execute call %d should succeed", i+1)
			}
		}

		// 3rd call should be blocked
		result := stage.Process(nil, agentctx.ParsedResponse{
			ToolCalls: []agentctx.ToolCall{{
				ID:       "call3",
				Name:     "Execute",
				Arguments: []byte("{}"),
			}},
		})
		if len(result.ToolResults) != 1 || !result.ToolResults[0].IsError {
			t.Error("Execute 3rd call should be blocked")
		} else if !contains(result.ToolResults[0].Result, "already called 2 times") {
			t.Errorf("Expected budget exceeded message, got: %s", result.ToolResults[0].Result)
		}
	})

	// Test unlimited tool
	t.Run("Unlimited_tool", func(t *testing.T) {
		stage.callCounts = make(map[string]int)
		
		// Should be able to call unlimited times
		for i := 0; i < 10; i++ {
			result := stage.Process(nil, agentctx.ParsedResponse{
				ToolCalls: []agentctx.ToolCall{{
					ID:       fmt.Sprintf("call%d", i),
					Name:     "UnlimitedTool",
					Arguments: []byte("{}"),
				}},
			})
			if len(result.ToolResults) != 1 || result.ToolResults[0].IsError {
				t.Errorf("UnlimitedTool call %d should succeed", i+1)
			}
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}