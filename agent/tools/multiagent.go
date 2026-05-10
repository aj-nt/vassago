// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// --- Agent Registry ---

// AgentInfo describes a registered agent's identity and capabilities.
type AgentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities"`
	Status       string   `json:"status"` // "online", "offline"
	LastSeen     int64    `json:"last_seen"` // Unix timestamp
}

// AgentRegistry maintains a local registry of known agents.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]AgentInfo
	path   string // persistence path
}

// NewAgentRegistry creates a new agent registry, loading from disk if available.
func NewAgentRegistry(path string) *AgentRegistry {
	reg := &AgentRegistry{
		agents: make(map[string]AgentInfo),
		path:   path,
	}
	reg.load()
	return reg
}

// Register adds or updates an agent in the registry.
func (r *AgentRegistry) Register(agent AgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	agent.LastSeen = time.Now().Unix()
	agent.Status = "online"
	r.agents[agent.ID] = agent
	r.save()
}

// Unregister removes an agent from the registry.
func (r *AgentRegistry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info, ok := r.agents[agentID]; ok {
		info.Status = "offline"
		r.agents[agentID] = info
		r.save()
	}
}

// Get returns info about a specific agent.
func (r *AgentRegistry) Get(agentID string) (AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.agents[agentID]
	return info, ok
}

// List returns all registered agents.
func (r *AgentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		result = append(result, info)
	}
	return result
}

// ListByCapability returns agents that have a specific capability.
func (r *AgentRegistry) ListByCapability(capability string) []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []AgentInfo
	for _, info := range r.agents {
		if info.Status == "online" {
			for _, cap := range info.Capabilities {
				if strings.EqualFold(cap, capability) {
					result = append(result, info)
					break
				}
			}
		}
	}
	return result
}

// ListByRole returns agents with a specific role.
func (r *AgentRegistry) ListByRole(role string) []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []AgentInfo
	for _, info := range r.agents {
		if info.Status == "online" && strings.EqualFold(info.Role, role) {
			result = append(result, info)
		}
	}
	return result
}

// FindBestAgent returns the best agent for a given set of required capabilities.
// Returns the agent that matches the most capabilities.
func (r *AgentRegistry) FindBestAgent(requiredCapabilities []string) (AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best AgentInfo
	bestScore := -1

	for _, info := range r.agents {
		if info.Status != "online" {
			continue
		}
		score := 0
		for _, reqCap := range requiredCapabilities {
			for _, agentCap := range info.Capabilities {
				if strings.EqualFold(agentCap, reqCap) {
					score++
					break
				}
			}
		}
		if score > bestScore {
			bestScore = score
			best = info
		}
	}

	return best, bestScore > 0
}

// Persistence

func (r *AgentRegistry) load() {
	if r.path == "" {
		return
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return // File doesn't exist yet, that's OK
	}
	var agents []AgentInfo
	if err := json.Unmarshal(data, &agents); err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, info := range agents {
		r.agents[info.ID] = info
	}
}

func (r *AgentRegistry) save() {
	if r.path == "" {
		return
	}
	agents := make([]AgentInfo, 0, len(r.agents))
	for _, info := range r.agents {
		agents = append(agents, info)
	}
	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(r.path), 0755)
	os.WriteFile(r.path, data, 0644)
}

// --- MultiAgent Config ---

type MultiAgentConfig struct {
	Enabled       bool   `yaml:"enabled"`
	RegistryPath  string `yaml:"registry_path"` // Path to agent registry JSON (default: ~/.mnemosyne/agents.json)
}

// DefaultMultiAgentConfig returns sensible defaults.
func DefaultMultiAgentConfig() MultiAgentConfig {
	homeDir, _ := os.UserHomeDir()
	return MultiAgentConfig{
		Enabled:      false,
		RegistryPath: filepath.Join(homeDir, ".mnemosyne", "agents.json"),
	}
}

// --- Tool Schemas ---

var ListAgentsSchema = map[string]interface{}{
	"name":        "ListAgents",
	"description": "List registered agents. Optionally filter by role or capability.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"role": map[string]interface{}{
				"type":        "string",
				"description": "Filter by agent role (e.g., 'assistant', 'delegate')",
			},
			"capability": map[string]interface{}{
				"type":        "string",
				"description": "Filter by required capability (e.g., 'memory', 'browser', 'vision')",
			},
		},
	},
}

var RegisterAgentSchema = map[string]interface{}{
	"name":        "RegisterAgent",
	"description": "Register this agent's capabilities in the multi-agent registry so other agents can discover it.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"capabilities": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of capabilities this agent provides (e.g., 'memory', 'browser', 'vision', 'code')",
			},
		},
	},
}

var FindAgentSchema = map[string]interface{}{
	"name":        "FindAgent",
	"description": "Find the best agent for a task by searching for required capabilities.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"capabilities": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Required capabilities for the task",
			},
		},
		"required": []string{"capabilities"},
	},
}

// --- Registration ---

// RegisterMultiAgentTools adds multi-agent orchestration tools to the registry.
// All mutable state (registry, agent identity) is captured in closures —
// no package-level globals. Each call creates its own AgentRegistry.
// Returns the AgentRegistry for test inspection.
func RegisterMultiAgentTools(toolReg *ToolRegistry, cfg MultiAgentConfig, agentID, agentName, agentRole string) *AgentRegistry {
	if !cfg.Enabled {
		return nil
	}

	agentRegistry := NewAgentRegistry(cfg.RegistryPath)

	// Register self
	agentRegistry.Register(AgentInfo{
		ID:           agentID,
		Name:         agentName,
		Role:         agentRole,
		Capabilities: []string{}, // Updated by RegisterAgent tool below
		Status:       "online",
		LastSeen:     time.Now().Unix(),
	})

	// ListAgents — captures agentRegistry
	toolReg.Register("ListAgents", func(args map[string]interface{}) (string, bool) {
		role := strArg(args, "role", "")
		capability := strArg(args, "capability", "")

		var agents []AgentInfo
		if capability != "" {
			agents = agentRegistry.ListByCapability(capability)
		} else if role != "" {
			agents = agentRegistry.ListByRole(role)
		} else {
			agents = agentRegistry.List()
		}

		if len(agents) == 0 {
			return "No agents found", false
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Found %d agent(s):\n", len(agents)))
		for _, a := range agents {
			caps := strings.Join(a.Capabilities, ", ")
			if caps == "" {
				caps = "none"
			}
			b.WriteString(fmt.Sprintf("  %s (%s) — role: %s, caps: %s, status: %s\n",
				a.Name, a.ID, a.Role, caps, a.Status))
		}
		const maxResultChars = 4000
		return truncateOutput(b.String(), maxResultChars), false
	}, ListAgentsSchema)

	// RegisterAgent — captures agentRegistry, agentID, agentName, agentRole
	toolReg.Register("RegisterAgent", func(args map[string]interface{}) (string, bool) {
		var capabilities []string
		if caps, ok := args["capabilities"]; ok {
			switch v := caps.(type) {
			case []string:
				capabilities = v
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						capabilities = append(capabilities, s)
					}
				}
			}
		}

		info := AgentInfo{
			ID:           agentID,
			Name:         agentName,
			Role:         agentRole,
			Capabilities: capabilities,
			Status:       "online",
			LastSeen:     time.Now().Unix(),
		}
		agentRegistry.Register(info)

		caps := strings.Join(capabilities, ", ")
		if caps == "" {
			caps = "none"
		}
		return fmt.Sprintf("Registered agent %s (%s) with capabilities: %s", info.Name, info.ID, caps), false
	}, RegisterAgentSchema)

	// FindAgent — captures agentRegistry
	toolReg.Register("FindAgent", func(args map[string]interface{}) (string, bool) {
		var capabilities []string
		if caps, ok := args["capabilities"]; ok {
			switch v := caps.(type) {
			case []string:
				capabilities = v
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						capabilities = append(capabilities, s)
					}
				}
			}
		}

		if len(capabilities) == 0 {
			return "At least one capability is required", true
		}

		agent, found := agentRegistry.FindBestAgent(capabilities)
		if !found {
			capsStr := strings.Join(capabilities, ", ")
			return fmt.Sprintf("No agent found with capabilities: %s", capsStr), false
		}

		return fmt.Sprintf("Best agent: %s (%s) — role: %s, caps: %s, status: %s",
			agent.Name, agent.ID, agent.Role,
			strings.Join(agent.Capabilities, ", "),
			agent.Status), false
	}, FindAgentSchema)

	return agentRegistry
}