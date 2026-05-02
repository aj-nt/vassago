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
	Scheduler        SchedulerSection  `yaml:"scheduler"`
	Messaging        MessagingSection  `yaml:"messaging"`
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
	PersonalityFile string `yaml:"personality_file"`
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
	Address    string `yaml:"address"`
	ChatListen string `yaml:"chat_listen"` // gRPC address for ChatStream server (e.g., ":50052")
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
	if cfg.Agent.MaxTurns == 0 {
		cfg.Agent.MaxTurns = 50
	}
	if cfg.Provider.Temperature == 0 {
		cfg.Provider.Temperature = 0.7
	}
	if cfg.HotBlock.MemoryCharLimit == 0 {
		cfg.HotBlock.MemoryCharLimit = 4000
	}
	if cfg.HotBlock.MinPriority == 0 {
		cfg.HotBlock.MinPriority = 2
	}
	if cfg.Delegation.MaxDepth == 0 {
		cfg.Delegation.MaxDepth = 1
	}
	if cfg.Delegation.MaxTurns == 0 {
		cfg.Delegation.MaxTurns = 10
	}
	if cfg.Scheduler.Interval == 0 {
		cfg.Scheduler.Interval = 3600 // 1 hour
	}

	// Validate tool names in tools.enabled (#16)
	if len(cfg.Tools.Enabled) > 0 {
		knownTools := map[string]bool{
			"AddMemory": true, "SearchMemories": true, "ReplaceMemory": true,
			"RemoveMemory": true, "Consolidate": true, "GetHotBlock": true,
			"FileRead": true, "FileWrite": true, "FileSearch": true,
			"Execute": true,
			"SearchWeb": true, "FetchURL": true,
			"SearchSessions": true, "ListSessions": true, "GetSession": true,
			"AddTodo": true, "ListTodos": true, "CompleteTodo": true, "RemoveTodo": true,
			"SendMessage": true,
			"Delegate": true,
		}
		for _, name := range cfg.Tools.Enabled {
			if !knownTools[name] {
				fmt.Fprintf(os.Stderr, "Warning: unknown tool name %q in tools.enabled — will be ignored\n", name)
			}
		}
	}

	// Load personality from file if specified
	if cfg.Agent.PersonalityFile != "" {
		personalityPath := cfg.Agent.PersonalityFile
		// If the path is relative, resolve it relative to the config file's directory
		if !filepath.IsAbs(personalityPath) {
			configDir := filepath.Dir(path)
			personalityPath = filepath.Join(configDir, personalityPath)
		}

		personalityData, err := os.ReadFile(personalityPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read personality file: %w", err)
		}
		cfg.Agent.Personality = string(personalityData)
	}

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

	return nil
}

// ToAgentConfig converts the YAML config to the Agent's Config struct.
func (c *AgentConfig) ToAgentConfig() Config {
	return Config{
		AgentID:      c.Agent.ID,
		AgentName:    c.Agent.Name,
		AgentRole:    c.Agent.Role,
		Personality:  c.Agent.Personality,
		Temperature:  c.Provider.Temperature,
		MaxTurns:      c.Agent.MaxTurns,
		MaxContextTokens: 128000,
		HotBlockConfig: HotBlockConfig{
			CharLimit:          c.HotBlock.MemoryCharLimit,
			MinPriority:        c.HotBlock.MinPriority,
			BoostCategories:    c.HotBlock.BoostCategories,
			SuppressCategories: c.HotBlock.SuppressCategories,
			BoostPriority:      2,
		},
	}
}
