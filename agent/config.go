// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aj-nt/mnemosyne/agent/provider"
	"gopkg.in/yaml.v3"
)

// AgentConfig is the full agent configuration from YAML.
type AgentConfig struct {
	Agent            AgentSection      `yaml:"agent"`
	Provider         ProviderSection   `yaml:"provider"`
	Routing          RoutingSection    `yaml:"routing"`
	Mnemosyne        MnemosyneSection  `yaml:"mnemosyne"`
	HotBlock         HotBlockSection   `yaml:"hot_block"`
	Tools            ToolsSection      `yaml:"tools"`
	Delegation       DelegationSection `yaml:"delegation"`
	MemorySafety     MemorySafetySection `yaml:"memory_safety"`
	Browser          BrowserSection    `yaml:"browser"`
	Scheduler        SchedulerSection  `yaml:"scheduler"`
	Messaging        MessagingSection  `yaml:"messaging"`
	Vision           VisionSection         `yaml:"vision"`
	ImageGen         ImageGenSection       `yaml:"image_gen"`
	MultiAgent       MultiAgentSection     `yaml:"multi_agent"`
	BackgroundProcess BackgroundProcessSection `yaml:"background_process"`
	CodeExec         CodeExecSection       `yaml:"code_exec"`
}

// RoutingSection configures the model routing strategy and model definitions.
type RoutingSection struct {
	Strategy string                `yaml:"strategy"` // "always-primary", "cost-optimized", "quality-optimized", "smart"
	Models   []provider.ModelConfig `yaml:"models"`
}

type AgentSection struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Role          string `yaml:"role"`
	Personality   string `yaml:"personality"`
	MaxTurns      int    `yaml:"max_turns"`
}

type ProviderSection struct {
	Name              string  `yaml:"name"`
	Model             string  `yaml:"model"`
	APIKeyEnv         string  `yaml:"api_key_env"`
	BaseURL           string  `yaml:"base_url"`
	Temperature       float64 `yaml:"temperature"`
	Fallback1         string  `yaml:"fallback1"` // Fallback provider name
	Fallback2         string  `yaml:"fallback2"` // Second fallback provider name
	AnthropicModel    string  `yaml:"anthropic_model"`    // Model override for Anthropic provider
	AnthropicAPIKeyEnv string `yaml:"anthropic_api_key_env"` // Env var for Anthropic API key (default: ANTHROPIC_API_KEY)
	OpenAIModel        string  `yaml:"openai_model"`       // Model override for OpenAI/Ollama provider
	OpenAIAPIKeyEnv    string  `yaml:"openai_api_key_env"` // Env var for OpenAI API key (default: OPENAI_API_KEY)
	OpenAIBaseURL      string  `yaml:"openai_base_url"`    // Base URL for OpenAI-compatible provider
}

type MnemosyneSection struct {
	Address      string `yaml:"address"`
	ChatListen   string `yaml:"chat_listen"`   // gRPC address for ChatStream server (e.g., ":50052")
	HealthListen string `yaml:"health_listen"`  // HTTP address for /health endpoint (e.g., ":8080")
}

type MessagingSection struct {
	Discord  DiscordMessagingConfig  `yaml:"discord"`
	Telegram TelegramMessagingConfig `yaml:"telegram"`
	IMessage IMessageMessagingConfig `yaml:"imessage"`
	Email    EmailMessagingConfig    `yaml:"email"`
}

type DiscordMessagingConfig struct {
	Enabled bool `yaml:"enabled"`
}

type TelegramMessagingConfig struct {
	Enabled bool `yaml:"enabled"`
}

type IMessageMessagingConfig struct {
	Enabled bool `yaml:"enabled"`
}

type EmailMessagingConfig struct {
	Enabled bool `yaml:"enabled"`
}

type HotBlockSection struct {
	MemoryCharLimit   int      `yaml:"memory_char_limit"`
	UserCharLimit     int      `yaml:"user_char_limit"`
	MinPriority       int      `yaml:"min_priority"`
	BoostCategories   []string `yaml:"boost_categories"`
	SuppressCategories []string `yaml:"suppress_categories"`
}

type ToolsSection struct {
	Enabled         []string `yaml:"enabled"`
	ApprovalRequired []string `yaml:"approval_required"`
	AllowedDirs     []string `yaml:"allowed_dirs"`
}

// MemorySafetySection configures memory mutation safety limits.
type MemorySafetySection struct {
	MaxMutationsPerSession int `yaml:"max_mutations_per_session"` // 0 = use default (10), -1 = unlimited
}

// BrowserSection configures browser automation.
type BrowserSection struct {
	Enabled  bool   `yaml:"enabled"`
	Headless bool   `yaml:"headless"`
	CacheDir string `yaml:"cache_dir"` // Directory for screenshots (default: ~/.mnemosyne/screenshots)
}

// VisionSection configures image analysis.
type VisionSection struct {
	Enabled      bool `yaml:"enabled"`
	MaxImageSize int  `yaml:"max_image_size"` // in bytes, default 20MB
}

// ImageGenSection configures image generation.
type ImageGenSection struct {
	Enabled         bool   `yaml:"enabled"`
	Provider        string `yaml:"provider"`          // openai or stability
	Model           string `yaml:"model"`             // default: dall-e-3
	APIKeyEnv       string `yaml:"api_key_env"`        // env var for API key
	MaxPromptLength int    `yaml:"max_prompt_length"`  // default: 1000
	CacheDir        string `yaml:"cache_dir"`           // directory for generated images
}

// BackgroundProcessSection configures background process management.
type BackgroundProcessSection struct {
	Enabled      bool `yaml:"enabled"`
	MaxProcesses int  `yaml:"max_processes"`
	MaxOutputKB  int  `yaml:"max_output_kb"`
	MaxAgeMin    int  `yaml:"max_age_min"`
	ApprovalRequired bool `yaml:"approval_required"`
}

// CodeExecSection configures Python code execution sandbox.
type CodeExecSection struct {
	Enabled      bool `yaml:"enabled"`
	Timeout      int  `yaml:"timeout"`        // max seconds (default 300)
	MaxOutputKB  int  `yaml:"max_output_kb"`  // max output in KB (default 50)
	MaxCodeChars int  `yaml:"max_code_chars"` // max code length in chars (default 50000)
}

// MultiAgentSection configures multi-agent orchestration.
type MultiAgentSection struct {
	Enabled      bool   `yaml:"enabled"`
	RegistryPath string `yaml:"registry_path"` // Path to agent registry JSON
}

// DelegationSection configures sub-agent spawning.
type DelegationSection struct {
	Enabled  bool `yaml:"enabled"`
	MaxDepth int  `yaml:"max_depth"` // Max nesting depth (1 = sub-agents can't delegate)
	MaxTurns int  `yaml:"max_turns"` // Max turns for sub-agent (default: 10)
}

// SchedulerSection configures periodic scheduled tasks.
type SchedulerSection struct {
	Enabled  bool `yaml:"enabled"`   // Whether to run scheduled tasks
	Interval int  `yaml:"interval"` // Base interval in seconds for consolidation (default: 3600 = 1h)
}

// applyDefaults sets default values for configuration fields
func (c *AgentConfig) applyDefaults() {
	// Apply defaults
	if c.Agent.MaxTurns == 0 {
		c.Agent.MaxTurns = 50
	}
	if c.Provider.Temperature == 0 {
		c.Provider.Temperature = 0.7
	}
	if c.HotBlock.MemoryCharLimit == 0 {
		c.HotBlock.MemoryCharLimit = 4000
	}
	if c.HotBlock.UserCharLimit == 0 {
		c.HotBlock.UserCharLimit = 4000
	}
	if c.HotBlock.MinPriority == 0 {
		c.HotBlock.MinPriority = 2
	}
	if c.Delegation.MaxDepth == 0 {
		c.Delegation.MaxDepth = 1
	}
	if c.Delegation.MaxTurns == 0 {
		c.Delegation.MaxTurns = 10
	}
	if c.Scheduler.Interval == 0 {
		c.Scheduler.Interval = 3600 // 1 hour
	}
	// Browser defaults to headless when enabled
	if c.Browser.Enabled && !c.Browser.Headless {
		c.Browser.Headless = true // default to headless
	}

	// Vision defaults
	if c.Vision.MaxImageSize == 0 {
		c.Vision.MaxImageSize = 20 * 1024 * 1024 // 20MB
	}

	// Image generation defaults
	if c.ImageGen.MaxPromptLength == 0 {
		c.ImageGen.MaxPromptLength = 1000
	}
	if c.ImageGen.Provider == "" {
		c.ImageGen.Provider = "openai"
	}
	if c.ImageGen.Model == "" {
		c.ImageGen.Model = "dall-e-3"
	}
	if c.ImageGen.APIKeyEnv == "" {
		c.ImageGen.APIKeyEnv = "OPENAI_API_KEY"
	}

	// MultiAgent defaults
	if c.MultiAgent.Enabled && c.MultiAgent.RegistryPath == "" {
		homeDir, _ := os.UserHomeDir()
		c.MultiAgent.RegistryPath = filepath.Join(homeDir, ".mnemosyne", "agents.json")
	}

	// BackgroundProcess defaults
	if c.BackgroundProcess.MaxProcesses == 0 {
		c.BackgroundProcess.MaxProcesses = 5
	}
	if c.BackgroundProcess.MaxOutputKB == 0 {
		c.BackgroundProcess.MaxOutputKB = 64
	}
	if c.BackgroundProcess.MaxAgeMin == 0 {
		c.BackgroundProcess.MaxAgeMin = 30
	}

	// CodeExec defaults
	if c.CodeExec.Timeout == 0 {
		c.CodeExec.Timeout = 300
	}
	if c.CodeExec.MaxOutputKB == 0 {
		c.CodeExec.MaxOutputKB = 50
	}
	if c.CodeExec.MaxCodeChars == 0 {
		c.CodeExec.MaxCodeChars = 50000
	}
}

// validateTools checks that all enabled tools are known tool names
func (c *AgentConfig) validateTools() {
	// Validate tool names in tools.enabled (#16)
	if len(c.Tools.Enabled) > 0 {
		knownTools := map[string]bool{
			"AddMemory": true, "SearchMemories": true, "ReplaceMemory": true,
			"RemoveMemory": true, "Consolidate": true, "GetHotBlock": true,
			"FileRead": true, "FileWrite": true, "FileSearch": true,
			"FileEdit": true,
			"Execute": true,
			"ExecuteCode": true,
			"BackgroundProcess": true,
			"SearchWeb": true, "FetchURL": true,
			"SearchSessions": true, "ListSessions": true, "GetSession": true,
			"AddTodo": true, "ListTodos": true, "CompleteTodo": true, "RemoveTodo": true,
			"SendMessage": true,
			"Delegate": true,
			"ScheduleCron": true,
			"Browse": true,
			"AnalyzeImage": true,
			"GenerateImage": true,
			"ListAgents": true,
			"RegisterAgent": true,
			"FindAgent": true,
		}
		for _, name := range c.Tools.Enabled {
			if !knownTools[name] {
				fmt.Fprintf(os.Stderr, "Warning: unknown tool name %q in tools.enabled — will be ignored\n", name)
			}
		}
	}
}



// LoadConfig reads agent config from a YAML file.
func LoadConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &AgentConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate tools
	cfg.validateTools()



	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the config for invalid values.
func (c *AgentConfig) Validate() error {
	// Agent section validation
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.Agent.MaxTurns < 1 {
		return fmt.Errorf("agent.max_turns must be >= 1, got %d", c.Agent.MaxTurns)
	}
	if c.Agent.MaxTurns > 200 {
		return fmt.Errorf("agent.max_turns must be <= 200, got %d", c.Agent.MaxTurns)
	}

	// Provider section validation
	if c.Provider.Name == "" {
		return fmt.Errorf("provider.name is required")
	}
	if c.Provider.Model == "" {
		return fmt.Errorf("provider.model is required")
	}
	if c.Provider.Temperature < 0 || c.Provider.Temperature > 2 {
		return fmt.Errorf("provider.temperature must be between 0 and 2, got %f", c.Provider.Temperature)
	}

	// Routing section validation (optional)
	if c.Routing.Strategy != "" {
		validStrategies := map[string]bool{
			"always-primary":  true,
			"cost-optimized":  true,
			"quality-optimized": true,
			"smart": true,
		}
		if !validStrategies[c.Routing.Strategy] {
			return fmt.Errorf("routing.strategy must be one of: always-primary, cost-optimized, quality-optimized, smart, got %s", c.Routing.Strategy)
		}
	}

	// Mnemosyne section validation
	if c.Mnemosyne.Address == "" {
		return fmt.Errorf("mnemosyne.address is required")
	}

	// HotBlock section validation
	if c.HotBlock.MemoryCharLimit < 1000 || c.HotBlock.MemoryCharLimit > 20000 {
		return fmt.Errorf("hot_block.memory_char_limit must be between 1000 and 20000, got %d", c.HotBlock.MemoryCharLimit)
	}
	if c.HotBlock.UserCharLimit < 500 || c.HotBlock.UserCharLimit > 10000 {
		return fmt.Errorf("hot_block.user_char_limit must be between 500 and 10000, got %d", c.HotBlock.UserCharLimit)
	}
	if c.HotBlock.MinPriority < 1 || c.HotBlock.MinPriority > 5 {
		return fmt.Errorf("hot_block.min_priority must be between 1 and 5, got %d", c.HotBlock.MinPriority)
	}

	// Delegation section validation
	if c.Delegation.MaxDepth < 1 || c.Delegation.MaxDepth > 5 {
		return fmt.Errorf("delegation.max_depth must be between 1 and 5, got %d", c.Delegation.MaxDepth)
	}
	if c.Delegation.MaxTurns < 1 || c.Delegation.MaxTurns > 50 {
		return fmt.Errorf("delegation.max_turns must be between 1 and 50, got %d", c.Delegation.MaxTurns)
	}

	// Scheduler section validation
	if c.Scheduler.Interval < 60 || c.Scheduler.Interval > 86400 {
		return fmt.Errorf("scheduler.interval must be between 60 and 86400 seconds, got %d", c.Scheduler.Interval)
	}

	// Vision section validation
	if c.Vision.Enabled && c.Vision.MaxImageSize < 1024*1024 {
		return fmt.Errorf("vision.max_image_size must be at least 1MB, got %d", c.Vision.MaxImageSize)
	}
	if c.Vision.MaxImageSize > 100*1024*1024 {
		return fmt.Errorf("vision.max_image_size must be at most 100MB, got %d", c.Vision.MaxImageSize)
	}

	// ImageGen section validation
	if c.ImageGen.Enabled {
		validProviders := map[string]bool{"openai": true, "stability": true}
		if !validProviders[c.ImageGen.Provider] {
			return fmt.Errorf("image_gen.provider must be one of: openai, stability, got %s", c.ImageGen.Provider)
		}
		if c.ImageGen.MaxPromptLength < 100 || c.ImageGen.MaxPromptLength > 4000 {
			return fmt.Errorf("image_gen.max_prompt_length must be between 100 and 4000, got %d", c.ImageGen.MaxPromptLength)
		}
	}

	// CodeExec section validation
	if c.CodeExec.Enabled {
		if c.CodeExec.Timeout < 10 || c.CodeExec.Timeout > 600 {
			return fmt.Errorf("code_exec.timeout must be between 10 and 600 seconds, got %d", c.CodeExec.Timeout)
		}
		if c.CodeExec.MaxOutputKB < 1 || c.CodeExec.MaxOutputKB > 1024 {
			return fmt.Errorf("code_exec.max_output_kb must be between 1 and 1024, got %d", c.CodeExec.MaxOutputKB)
		}
		if c.CodeExec.MaxCodeChars < 1000 || c.CodeExec.MaxCodeChars > 500000 {
			return fmt.Errorf("code_exec.max_code_chars must be between 1000 and 500000, got %d", c.CodeExec.MaxCodeChars)
		}
	}

	return nil
}

// ToAgentConfig converts the YAML config to the Agent's Config struct.
func (c *AgentConfig) ToAgentConfig() Config {
	// Compute MaxContextTokens from the smallest context_window in the routing models.
	// This ensures we never overflow a smaller model's context window.
	// If no models are defined, fall back to 128000.
	maxContextTokens := 0
	for _, m := range c.Routing.Models {
		if m.ContextWindow > 0 {
			if maxContextTokens == 0 || m.ContextWindow < maxContextTokens {
				maxContextTokens = m.ContextWindow
			}
		}
	}
	if maxContextTokens == 0 {
		maxContextTokens = 128000
	}

	return Config{
		AgentID:          c.Agent.ID,
		AgentName:        c.Agent.Name,
		AgentRole:        c.Agent.Role,
		Personality:      c.Agent.Personality,
		Temperature:      c.Provider.Temperature,
		MaxTurns:         c.Agent.MaxTurns,
		MaxContextTokens:  maxContextTokens,
		HotBlockConfig: HotBlockConfig{
			CharLimit:          c.HotBlock.MemoryCharLimit,
			MinPriority:        c.HotBlock.MinPriority,
			BoostCategories:    c.HotBlock.BoostCategories,
			SuppressCategories: c.HotBlock.SuppressCategories,
			BoostPriority:      2,
		},
		MaxMutations: c.MemorySafety.MaxMutationsPerSession,
	}
}
