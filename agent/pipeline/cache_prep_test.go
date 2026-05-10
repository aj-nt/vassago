// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// ============================================================================
// Phase 32: Prompt Caching — TDD tests
// Verify that RequestPrepStage sets CacheBreakpoint correctly.
// ============================================================================

func TestCacheBreakpoint_SetForSystemPrompt(t *testing.T) {
	stage := NewRequestPrepStage("helpful")

	ctx := &agentctx.ConversationContext{
		HotBlockContent: "[env] key: value",
		Messages: []agentctx.Message{
			{Role: "user", Content: "hello"},
		},
	}

	req := stage.Process(ctx)

	if req.CacheBreakpoint == 0 {
		t.Error("CacheBreakpoint should be > 0 when system prompt exists")
	}

	// System prompt should contain personality
	if !strings.Contains(req.SystemPrompt, "helpful") {
		t.Error("SystemPrompt should contain personality")
	}
}

func TestCacheBreakpoint_GrowsWithLongerPrompt(t *testing.T) {
	stage := NewRequestPrepStage("a very detailed and thorough assistant that provides comprehensive answers")

	ctx := &agentctx.ConversationContext{
		HotBlockContent: "[env] os: darwin\n[project] name: mnemosyne\n[project] status: active",
	}

	req := stage.Process(ctx)

	// Longer prompt → larger breakpoint
	if req.CacheBreakpoint < 10 {
		t.Errorf("CacheBreakpoint = %d, should be larger for a long prompt", req.CacheBreakpoint)
	}

	// Verify it roughly matches system prompt length/4
	expected := len(req.SystemPrompt) / 4
	if req.CacheBreakpoint != expected {
		t.Errorf("CacheBreakpoint = %d, expected ~%d (len(SystemPrompt)/4)", req.CacheBreakpoint, expected)
	}
}

func TestCacheBreakpoint_ZeroForEmptyPrompt(t *testing.T) {
	// Empty personality → minimal system prompt (just the fixed sections)
	stage := NewRequestPrepStage("")

	ctx := &agentctx.ConversationContext{}

	req := stage.Process(ctx)

	// Should still have a system prompt (tool guide, anti-repetition, etc.)
	// but CacheBreakpoint should reflect its length
	if len(req.SystemPrompt) > 0 && req.CacheBreakpoint == 0 {
		t.Error("CacheBreakpoint should be > 0 if SystemPrompt is non-empty")
	}
}

func TestCacheBreakpoint_PassesThroughTools(t *testing.T) {
	stage := NewRequestPrepStage("helpful")

	ctx := &agentctx.ConversationContext{
		Tools: []agentctx.ToolDefinition{
			{Name: "echo", Description: "echo tool"},
			{Name: "search", Description: "search tool"},
		},
	}

	req := stage.Process(ctx)

	// CacheBreakpoint should not affect tool passing
	if len(req.Tools) != 2 {
		t.Errorf("want 2 tools, got %d", len(req.Tools))
	}
}
