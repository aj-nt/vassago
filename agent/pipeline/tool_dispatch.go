// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"encoding/json"
	"fmt"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// ToolDispatchStage routes tool calls to their handlers and collects results.
type ToolDispatchStage struct {
	Registry *tools.ToolRegistry
	EventBus *eventbus.EventBus
	// Approver is called before executing a tool. Return false to skip.
	// This is used by the CLI to prompt the user for approval of dangerous tools.
	Approver func(toolName string, args map[string]interface{}) bool
	// MaxParallelTools caps the number of tool calls executed per turn.
	// When the LLM returns more than this, excess calls are skipped with an
	// error message. This prevents models like GLM-5.1 from firing 5+
	// tool calls without generating any text.
	MaxParallelTools int
	// callCounts tracks how many times each tool has been called in the current turn
	callCounts map[string]int
}

// NewToolDispatchStage creates a new Stage 4.
func NewToolDispatchStage(registry *tools.ToolRegistry, eventBus *eventbus.EventBus) *ToolDispatchStage {
	return &ToolDispatchStage{
		Registry:        registry,
		EventBus:        eventBus,
		MaxParallelTools: 2,
	}
}

// Process executes tool calls and returns their results.
func (s *ToolDispatchStage) Process(_ *agentctx.ConversationContext, parsed agentctx.ParsedResponse) agentctx.ToolDispatchResult {
	if len(parsed.ToolCalls) == 0 {
		return agentctx.ToolDispatchResult{}
	}

	// Initialize call counts map if not already done
	if s.callCounts == nil {
		s.callCounts = make(map[string]int)
	}

	var toolResults []agentctx.ToolResult

	// Cap parallel tool calls to prevent models from firing 5+ calls
	// without generating text. Excess calls get an error result.
	maxTools := s.MaxParallelTools
	if maxTools <= 0 {
		maxTools = 2 // default cap
	}

	for i, tc := range parsed.ToolCalls {
		if i >= maxTools {
			toolResults = append(toolResults, agentctx.ToolResult{
				CallID:  tc.ID,
				Name:    tc.Name,
				Result:  fmt.Sprintf("Too many parallel tool calls (limit: %d). Respond with text first, then make your next tool call.", maxTools),
				IsError: true,
			})
			continue
		}
		if s.EventBus != nil {
			s.EventBus.Emit(eventbus.Event{Kind: "tool_call", Data: tc.Name})
		}
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

		// Check if the tool has exceeded its budget
		if !tools.CheckBudget(tc.Name, s.callCounts[tc.Name]) {
			toolResults = append(toolResults, agentctx.ToolResult{
				CallID:  tc.ID,
				Name:    tc.Name,
				Result:  tools.BudgetExceededMessage(tc.Name, s.callCounts[tc.Name]),
				IsError: true,
			})
			continue
		}

		// Unmarshal arguments — treat empty/null as empty map
		var args map[string]interface{}
		rawArgs := tc.Arguments
		if len(rawArgs) == 0 || string(rawArgs) == "null" {
			rawArgs = json.RawMessage("{}")
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			toolResults = append(toolResults, agentctx.ToolResult{
				CallID:  tc.ID,
				Name:    tc.Name,
				Result:  fmt.Sprintf("invalid arguments: %v", err),
				IsError: true,
			})
			continue
		}

		// Validate arguments against the tool's JSON Schema
		if schema, ok := s.Registry.Schemas()[tc.Name]; ok {
			// Apply defaults first
			tools.ApplyDefaults(args, schema)
			// Validate
			if err := tools.ValidateArgs(tc.Name, args, schema); err != nil {
				verr, ok := err.(*tools.ValidationError)
				if !ok {
					verr = &tools.ValidationError{Tool: tc.Name, Field: "", Issue: err.Error()}
				}
				toolResults = append(toolResults, agentctx.ToolResult{
					CallID:  tc.ID,
					Name:    tc.Name,
					Result:  fmt.Sprintf("Invalid argument: %s. Please check the tool schema and try again.", verr.Error()),
					IsError: true,
				})
				if s.EventBus != nil {
					s.EventBus.Emit(eventbus.Event{Kind: "tool_validation_error", Data: tc.Name})
				}
				continue
			}
		}

		// Check approval if an approver is set
		if s.Approver != nil && !s.Approver(tc.Name, args) {
			toolResults = append(toolResults, agentctx.ToolResult{
				CallID:  tc.ID,
				Name:    tc.Name,
				Result:  "tool execution skipped: user denied approval",
				IsError: true,
			})
			if s.EventBus != nil {
				s.EventBus.Emit(eventbus.Event{Kind: "tool_denied", Data: tc.Name})
			}
			continue
		}

		// Call handler with panic recovery
		result, isErr := safeCallHandler(handler, args)
		if s.EventBus != nil {
			s.EventBus.Emit(eventbus.Event{Kind: "tool_result", Data: result})
		}
		toolResults = append(toolResults, agentctx.ToolResult{
			CallID:  tc.ID,
			Name:    tc.Name,
			Result:  result,
			IsError: isErr,
		})

		// Increment the call count after successful execution
		s.callCounts[tc.Name]++
	}

	return agentctx.ToolDispatchResult{
		ToolResults: toolResults,
	}
}


// safeCallHandler calls a tool handler with panic recovery.
// If the handler panics, the panic is caught and returned as an error.
func safeCallHandler(handler tools.ToolHandler, args map[string]interface{}) (result string, isErr bool) {
	defer func() {
		if r := recover(); r != nil {
			result = fmt.Sprintf("tool handler panicked: %v", r)
			isErr = true
		}
	}()
	return handler(args)
}
