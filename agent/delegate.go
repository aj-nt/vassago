// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	memoclient "github.com/aj-nt/mnemosyne/client"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
)

// SubAgentRunnerImpl implements tools.SubAgentRunner using the Agent package.
type SubAgentRunnerImpl struct {
	Mnemo    *memoclient.Client
	Registry *provider.ProviderRegistry
	Router   *provider.ModelRouter // may be nil
}

// RunSubAgent creates and runs a sub-agent with the given prompt.
// The sub-agent gets a restricted toolset (memory only, no delegation).
func (r *SubAgentRunnerImpl) RunSubAgent(prompt string, agentID string, maxTurns int) (*tools.SubAgentResult, error) {
	// Create a restricted tool registry for the sub-agent — memory tools only
	subToolReg := tools.NewToolRegistry()
	tools.RegisterMemoryTools(subToolReg, r.Mnemo, agentID, tools.NewMutationRateLimit(5))
	// Deliberately NOT registering: Delegate, file, terminal, web tools

	// Create sub-agent config
	subCfg := Config{
		AgentID:     agentID,
		AgentName:   fmt.Sprintf("delegate-%s", agentID),
		AgentRole:   "delegate",
		Personality: "You are a focused sub-agent. Complete the task concisely and return results. You have access to memory tools but cannot delegate further.",
		MaxTurns:    maxTurns,
		HotBlockConfig: HotBlockConfig{
			CharLimit:     4000,
			BoostPriority: 2,
		},
	}

	// Create sub-agent
	var subAgent *Agent
	if r.Router != nil {
		subAgent = NewAgentWithRouter(subCfg, r.Registry, r.Router, subToolReg, r.Mnemo)
	} else {
		subAgent = NewAgent(subCfg, r.Registry, subToolReg, r.Mnemo)
	}

	// Register sub-agent with daemon
	if r.Mnemo != nil {
		regCtx, regCancel := context.WithTimeout(context.Background(), 5*time.Second)
		registeredID, _, err := r.Mnemo.RegisterAgent(regCtx, agentID, subCfg.AgentName, "delegate")
		regCancel()
		if err != nil {
			log.Printf("Delegate: sub-agent registration failed: %v", err)
		} else {
			subCfg.AgentID = registeredID
		}
	}

	// Run sub-agent with timeout
	runCtx, runCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer runCancel()

	result, err := subAgent.Run(runCtx, prompt)
	if err != nil {
		return &tools.SubAgentResult{
			Text:    fmt.Sprintf("sub-agent error: %v", err),
			IsError: true,
		}, err
	}

	text := ""
	if result.Response != nil && result.Response.Text != "" {
		text = result.Response.Text
	} else {
		// Sub-agent completed but produced no text output — return meaningful feedback
		text = "sub-agent completed but returned no output"
	}

	return &tools.SubAgentResult{
		Text:       text,
		IsError:    false,
		Iterations: result.Iterations,
	}, nil
}