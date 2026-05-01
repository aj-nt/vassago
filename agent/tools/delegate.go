package tools

import (
	"fmt"
	"time"
)

// DelegateConfig controls sub-agent spawning behavior.
type DelegateConfig struct {
	Enabled  bool // Whether delegation is allowed
	MaxDepth int  // Maximum nesting depth (1 = sub-agents can't delegate)
	MaxTurns int  // Max turns for the sub-agent (default: 10)
}

// DefaultDelegateConfig returns safe defaults.
func DefaultDelegateConfig() DelegateConfig {
	return DelegateConfig{
		Enabled:  true,
		MaxDepth: 1,
		MaxTurns: 10,
	}
}

// SubAgentResult is the result of running a sub-agent.
type SubAgentResult struct {
	Text       string
	IsError    bool
	Iterations int
}

// SubAgentRunner is implemented by the agent package to run sub-agents.
// This interface breaks the import cycle between tools and agent.
type SubAgentRunner interface {
	RunSubAgent(prompt string, agentID string, maxTurns int) (*SubAgentResult, error)
}

// RegisterDelegationTool adds the Delegate tool to the registry.
func RegisterDelegationTool(
	registry *ToolRegistry,
	parentAgentID string,
	runner SubAgentRunner,
	delegateCfg DelegateConfig,
) {
	registry.Register("Delegate",
		makeDelegate(parentAgentID, runner, delegateCfg),
		DelegateSchema,
	)
}

// makeDelegate creates the Delegate tool handler.
func makeDelegate(
	parentAgentID string,
	runner SubAgentRunner,
	cfg DelegateConfig,
) ToolHandler {
	return func(args map[string]interface{}) (string, bool) {
		prompt := strArg(args, "prompt", "")
		if prompt == "" {
			return "prompt is required", true
		}

		if !cfg.Enabled {
			return "delegation is disabled in agent config", true
		}

		// Generate sub-agent ID
		subAgentID := fmt.Sprintf("%s-sub-%d", parentAgentID, time.Now().UnixNano())

		// Run sub-agent (synchronous — blocks until done)
		result, err := runner.RunSubAgent(prompt, subAgentID, cfg.MaxTurns)
		if err != nil {
			return fmt.Sprintf("Delegate sub-agent failed: %v", err), true
		}

		if result.Text == "" {
			return "Delegate sub-agent returned no response", false
		}

		return result.Text, result.IsError
	}
}

// DelegateSchema is the JSON schema for the Delegate tool.
var DelegateSchema = map[string]interface{}{
	"name":        "Delegate",
	"description": "Spawn a sub-agent to handle a specific task. The sub-agent has access to memory tools (AddMemory, SearchMemories, etc.) but cannot delegate further. Use this for complex research, analysis, or multi-step tasks that benefit from a focused agent.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{
				"type":        "string",
				"description":   "The task description for the sub-agent. Be specific about what to do and what to return.",
			},
		},
		"required": []string{"prompt"},
	},
}