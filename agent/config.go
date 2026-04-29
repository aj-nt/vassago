package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AgentConfig is the full agent configuration from YAML.
type AgentConfig struct {
	Agent     AgentSection     `yaml:"agent"`
	Provider  ProviderSection  `yaml:"provider"`
	Mnemosyne MnemosyneSection `yaml:"mnemosyne"`
	HotBlock  HotBlockSection  `yaml:"hot_block"`
	Tools     ToolsSection     `yaml:"tools"`
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
	Address string `yaml:"address"`
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

	return cfg, nil
}

// Validate checks the config for invalid values.
func (c *AgentConfig) Validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.Agent.MaxTurns < 1 {
		return fmt.Errorf("agent.max_turns must be >= 1, got %d", c.Agent.MaxTurns)
	}
	if c.Provider.Name == "" {
		return fmt.Errorf("provider.name is required")
	}
	if c.Provider.Model == "" {
		return fmt.Errorf("provider.model is required")
	}
	if c.Mnemosyne.Address == "" {
		return fmt.Errorf("mnemosyne.address is required")
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
