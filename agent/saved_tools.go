// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/tools"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// sortedTool is a saved tool entry with a computed trigger-match score.
type sortedTool struct {
	entry *pb.SavedToolEntry
	score int
}

// rankedSavedTools fetches saved tools from the daemon, scores them by
// trigger-match × priority, sorts descending, and returns up to maxTools.
// Shared by GetSavedToolDefinitions and RegisterSavedToolHandlers so both
// the LLM tool list and the runtime handlers stay in sync.
func (a *Agent) rankedSavedTools(ctx context.Context, userMessage string, maxTools int) []sortedTool {
	if a.Mnemo == nil {
		return nil
	}

	entries, err := a.Mnemo.ListSavedTools(ctx, "", "", 100)
	if err != nil {
		log.Printf("Warning: failed to load saved tools: %v", err)
		return nil
	}

	if len(entries) == 0 {
		return nil
	}

	if maxTools <= 0 {
		maxTools = 10
	}

	// Score: priority × (3 if trigger matches, 1 otherwise)
	lowerMsg := strings.ToLower(userMessage)
	var ranked []sortedTool
	for _, e := range entries {
		score := int(e.Priority)
		for _, trigger := range e.Triggers {
			if strings.Contains(lowerMsg, strings.ToLower(trigger)) {
				score *= 3
				break
			}
		}
		ranked = append(ranked, sortedTool{entry: e, score: score})
	}

	// Sort by score descending
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].score > ranked[i].score {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}

	if len(ranked) > maxTools {
		ranked = ranked[:maxTools]
	}
	return ranked
}

// GetSavedToolDefinitions fetches saved tools from the daemon and converts
// them to ToolDefinitions for the LLM. Uses trigger-ranked loading:
// tools whose triggers match the user's first message get a ×3 priority boost.
// Always loads up to maxTools (default 10), ranked by (trigger_match × priority).
func (a *Agent) GetSavedToolDefinitions(ctx context.Context, userMessage string, maxTools int) []agentctx.ToolDefinition {
	ranked := a.rankedSavedTools(ctx, userMessage, maxTools)
	if len(ranked) == 0 {
		return nil
	}

	defs := make([]agentctx.ToolDefinition, 0, len(ranked))
	for _, st := range ranked {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(st.entry.Parameters), &params); err != nil {
			log.Printf("Warning: invalid parameters JSON for saved tool %q: %v", st.entry.Name, err)
			continue
		}
		defs = append(defs, agentctx.ToolDefinition{
			Name:        st.entry.Name,
			Description: st.entry.Description,
			Parameters:  params,
		})
	}
	return defs
}

// RegisterSavedToolHandlers creates dynamic ToolHandlers for the top-ranked
// saved tools (via trigger-ranked loading) and registers them in the tool
// registry. Script-type tools run via the existing Execute handler with all
// security checks + approval.
func (a *Agent) RegisterSavedToolHandlers(ctx context.Context, userMessage string) {
	if a.Mnemo == nil || a.ToolRegistry == nil {
		return
	}

	ranked := a.rankedSavedTools(ctx, userMessage, 10)
	if len(ranked) == 0 {
		return
	}

	for _, st := range ranked {
		name := st.entry.Name
		script := st.entry.Script
		requiresApproval := st.entry.RequiresApprovalEveryCall

		handler := makeSavedToolScriptHandler(a, name, script, requiresApproval)

		var paramsSchema map[string]interface{}
		json.Unmarshal([]byte(st.entry.Parameters), &paramsSchema)

		schema := map[string]interface{}{
			"name":        name,
			"description": st.entry.Description,
			"parameters":  paramsSchema,
		}

		a.ToolRegistry.Register(name, handler, schema)
	}

	log.Printf("Registered %d saved tools as dynamic handlers (trigger-ranked, max 10)", len(ranked))
}

// makeSavedToolScriptHandler creates a handler that substitutes parameters
// into a script template and runs it through the Execute tool.
func makeSavedToolScriptHandler(a *Agent, toolName, script string, requiresApproval bool) tools.ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		// Parameter substitution: replace {{param}} with value
		executed := script
		for key, val := range args {
			placeholder := fmt.Sprintf("{{%s}}", key)
			executed = strings.ReplaceAll(executed, placeholder, fmt.Sprint(val))
		}

		// Check for unresolved placeholders
		if strings.Contains(executed, "{{") {
			var unresolved []string
			for _, part := range strings.Split(executed, "{{") {
				if idx := strings.Index(part, "}}"); idx > 0 {
					unresolved = append(unresolved, part[:idx])
				}
			}
			return fmt.Sprintf("Unresolved parameters in saved tool %q: %v", toolName, unresolved), true
		}

		// Get the Execute handler from the registry
		execHandler, err := a.ToolRegistry.Get("Execute")
		if err != nil {
			return fmt.Sprintf("Saved tool %q: Execute handler not found: %v", toolName, err), true
		}

		// Call Execute with the substituted command
		execArgs := map[string]interface{}{
			"command":         executed,
			"requires_approval": requiresApproval,
		}

		result, isErr := execHandler(execArgs)
		return fmt.Sprintf("[%s] %s", toolName, result), isErr
	}
}
