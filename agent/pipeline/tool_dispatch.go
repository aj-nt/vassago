package pipeline

import (
	"encoding/json"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// ToolDispatchStage routes tool calls to their handlers and collects results.
type ToolDispatchStage struct {
	Registry *tools.ToolRegistry
}

// NewToolDispatchStage creates a new Stage 4.
func NewToolDispatchStage(registry *tools.ToolRegistry) *ToolDispatchStage {
	return &ToolDispatchStage{Registry: registry}
}

// Process executes tool calls and returns their results.
func (s *ToolDispatchStage) Process(_ *agentctx.ConversationContext, parsed agentctx.ParsedResponse) agentctx.ToolDispatchResult {
	if len(parsed.ToolCalls) == 0 {
		return agentctx.ToolDispatchResult{}
	}

	var toolResults []agentctx.ToolResult
	for _, tc := range parsed.ToolCalls {
		handler, err := s.Registry.Get(tc.Name)
		if err != nil {
			toolResults = append(toolResults, agentctx.ToolResult{
				CallID:  tc.ID,
				Name:    tc.Name,
				Result:  fmt.Sprintf("unknown tool: %s", tc.Name),
				IsError: true,
			})
			continue
		}

		// Unmarshal arguments
		var args map[string]interface{}
		if err := json.Unmarshal(tc.Arguments, &args); err != nil {
			toolResults = append(toolResults, agentctx.ToolResult{
				CallID:  tc.ID,
				Name:    tc.Name,
				Result:  fmt.Sprintf("invalid arguments: %v", err),
				IsError: true,
			})
			continue
		}

		result, isErr := handler(args)
		toolResults = append(toolResults, agentctx.ToolResult{
			CallID:  tc.ID,
			Name:    tc.Name,
			Result:  result,
			IsError: isErr,
		})
	}

	return agentctx.ToolDispatchResult{
		ToolResults: toolResults,
	}
}
