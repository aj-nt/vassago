package pipeline

import (
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// ContextManagementStage appends results, checks limits, and decides whether to loop.
type ContextManagementStage struct {
	MaxContextTokens int
}

// NewContextManagementStage creates a new Stage 5.
func NewContextManagementStage(maxContextTokens int) *ContextManagementStage {
	return &ContextManagementStage{MaxContextTokens: maxContextTokens}
}

// Process updates the conversation context and returns the next action.
func (s *ContextManagementStage) Process(convCtx *agentctx.ConversationContext, toolResults []agentctx.ToolResult, parsed agentctx.ParsedResponse) agentctx.StageAction {
	// If LLM just returned text (no tool calls), we're done
	if len(parsed.ToolCalls) == 0 {
		return agentctx.StageActionYIELD
	}

	// Append assistant message with tool calls
	assistantMsg := agentctx.Message{
		Role:      "assistant",
		Content:   parsed.Text,
		ToolCalls: parsed.ToolCalls,
	}
	convCtx.AddMessage(assistantMsg)

	// Append tool results
	for _, tr := range toolResults {
		content := tr.Result
		if tr.IsError {
			content = fmt.Sprintf("Error: %s", tr.Result)
		}
		convCtx.AddMessage(agentctx.Message{
			Role:       "tool",
			Content:    content,
			ToolCallID: tr.CallID,
			Name:       tr.Name,
		})
	}

	// Check iteration limit
	if convCtx.Iteration >= convCtx.MaxIterations {
		return agentctx.StageActionTERMINATE
	}

	// Check context token limit
	if s.MaxContextTokens > 0 && convCtx.TokenCount() > s.MaxContextTokens {
		return agentctx.StageActionTERMINATE
	}

	// Continue the loop to process tool results
	return agentctx.StageActionCONTINUE
}
