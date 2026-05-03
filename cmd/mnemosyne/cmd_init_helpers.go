package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// fileExists checks if a file or directory exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DaemonConfig mirrors the daemon config structure for YAML generation.
type DaemonConfig struct {
	Daemon       DaemonDaemonConfig       `yaml:"daemon"`
	Database     DaemonDBConfig          `yaml:"database"`
	HotBlock     DaemonHotBlockConfig    `yaml:"hot_block"`
	Consolidation DaemonConsolidationConfig `yaml:"consolidation"`
	Logging      DaemonLoggingConfig     `yaml:"logging"`
}

type DaemonDaemonConfig struct {
	Socket string          `yaml:"socket"`
	TCP    string          `yaml:"tcp"`
	TLS    DaemonTLSConfig `yaml:"tls"`
}

type DaemonTLSConfig struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type DaemonDBConfig struct {
	Path        string `yaml:"path"`
	WALMode     bool   `yaml:"wal_mode"`
	BusyTimeout int    `yaml:"busy_timeout"`
}

type DaemonHotBlockConfig struct {
	MemoryCharLimit   int      `yaml:"memory_char_limit"`
	UserCharLimit     int      `yaml:"user_char_limit"`
	MinPriority       int      `yaml:"min_priority"`
	BoostCategories   []string `yaml:"boost_categories"`
	SuppressCategories []string `yaml:"suppress_categories"`
}

type DaemonConsolidationConfig struct {
	MaxIdleDays      int `yaml:"max_idle_days"`
	MaxIdlePriority1 int `yaml:"max_idle_priority_1"`
	MaxIdlePriority2 int `yaml:"max_idle_priority_2"`
}

type DaemonLoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func generateDaemonConfig(dir string) *DaemonConfig {
	return &DaemonConfig{
		Daemon: DaemonDaemonConfig{
			Socket: filepath.Join(dir, "mnemosyne.sock"),
			TCP:    "localhost:50051",
			TLS:    DaemonTLSConfig{Cert: "", Key: ""},
		},
		Database: DaemonDBConfig{
			Path:        filepath.Join(dir, "mnemosyne.db"),
			WALMode:     true,
			BusyTimeout: 5000,
		},
		HotBlock: DaemonHotBlockConfig{
			MemoryCharLimit:   8000,
			UserCharLimit:     3000,
			MinPriority:       2,
			BoostCategories:    []string{"project", "environment", "quirk"},
			SuppressCategories: []string{"observation"},
		},
		Consolidation: DaemonConsolidationConfig{
			MaxIdleDays:      60,
			MaxIdlePriority1: 365,
			MaxIdlePriority2: 180,
		},
		Logging: DaemonLoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// AgentConfigYAML mirrors the agent config for YAML generation.
type AgentConfigYAML struct {
	Agent      AgentSectionYAML      `yaml:"agent"`
	Provider   ProviderSectionYAML   `yaml:"provider"`
	Mnemosyne  MnemosyneSectionYAML  `yaml:"mnemosyne"`
	HotBlock   HotBlockSectionYAML   `yaml:"hot_block"`
	Tools      ToolsSectionYAML      `yaml:"tools"`
	Delegation DelegationSectionYAML `yaml:"delegation"`
	Browser    BrowserSectionYAML    `yaml:"browser"`
	Vision     VisionSectionYAML     `yaml:"vision"`
	ImageGen   ImageGenSectionYAML   `yaml:"image_gen"`
	MultiAgent MultiAgentSectionYAML `yaml:"multi_agent"`
	Scheduler  SchedulerSectionYAML `yaml:"scheduler"`
}

type AgentSectionYAML struct {
	ID             string `yaml:"id"`
	Name           string `yaml:"name"`
	Role           string `yaml:"role"`
	Personality    string `yaml:"personality"`
	PersonalityFile string `yaml:"personality_file"`
	MaxTurns       int    `yaml:"max_turns"`
}

type ProviderSectionYAML struct {
	Name               string  `yaml:"name"`
	Model              string  `yaml:"model"`
	Temperature        float64 `yaml:"temperature"`
	Fallback1          string  `yaml:"fallback1,omitempty"`
	OpenAIModel        string  `yaml:"openai_model,omitempty"`
	OpenAIBaseURL      string  `yaml:"openai_base_url,omitempty"`
	OpenAIAPIKeyEnv    string  `yaml:"openai_api_key_env,omitempty"`
	AnthropicModel     string  `yaml:"anthropic_model,omitempty"`
	AnthropicAPIKeyEnv string  `yaml:"anthropic_api_key_env,omitempty"`
}

type MnemosyneSectionYAML struct {
	Address    string `yaml:"address"`
	ChatListen string `yaml:"chat_listen,omitempty"`
}

type HotBlockSectionYAML struct {
	MemoryCharLimit    int      `yaml:"memory_char_limit"`
	UserCharLimit      int      `yaml:"user_char_limit"`
	MinPriority         int      `yaml:"min_priority"`
	BoostCategories    []string `yaml:"boost_categories"`
	SuppressCategories []string `yaml:"suppress_categories"`
}

type ToolsSectionYAML struct {
	Enabled          []string `yaml:"enabled"`
	ApprovalRequired []string `yaml:"approval_required"`
	AllowedDirs      []string `yaml:"allowed_dirs"`
}

type DelegationSectionYAML struct {
	Enabled  bool `yaml:"enabled"`
	MaxDepth int  `yaml:"max_depth"`
	MaxTurns int  `yaml:"max_turns"`
}

type BrowserSectionYAML struct {
	Enabled  bool   `yaml:"enabled"`
	Headless bool   `yaml:"headless"`
	CacheDir string `yaml:"cache_dir"`
}

type VisionSectionYAML struct {
	Enabled      bool `yaml:"enabled"`
	MaxImageSize int  `yaml:"max_image_size"`
}

type ImageGenSectionYAML struct {
	Enabled         bool   `yaml:"enabled"`
	Provider        string `yaml:"provider"`
	Model           string `yaml:"model"`
	APIKeyEnv       string `yaml:"api_key_env"`
	MaxPromptLength int    `yaml:"max_prompt_length"`
	CacheDir        string `yaml:"cache_dir"`
}

type MultiAgentSectionYAML struct {
	Enabled      bool   `yaml:"enabled"`
	RegistryPath string `yaml:"registry_path"`
}

type SchedulerSectionYAML struct {
	Enabled  bool `yaml:"enabled"`
	Interval int  `yaml:"interval"`
}

func generateAgentConfig(dir, provider, model string) *AgentConfigYAML {
	cfg := &AgentConfigYAML{
		Agent: AgentSectionYAML{
			ID:             "mnemosyne",
			Name:           "mnemosyne",
			Role:           "assistant",
			Personality:    "direct, dry-witted collaborator",
			PersonalityFile: filepath.Join(dir, "personalities", "bartimaeus.md"),
			MaxTurns:       30,
		},
		Provider: ProviderSectionYAML{
			Name:               provider,
			Model:              model,
			Temperature:        0.7,
			Fallback1:          "openai",
			OpenAIModel:        "glm-5.1:cloud",
			OpenAIBaseURL:      "http://localhost:11434/v1",
			OpenAIAPIKeyEnv:    "OLLAMA_API_KEY",
			AnthropicModel:     "claude-sonnet-4-6",
			AnthropicAPIKeyEnv: "ANTHROPIC_API_KEY",
		},
		Mnemosyne: MnemosyneSectionYAML{
			Address: "localhost:50051",
		},
		HotBlock: HotBlockSectionYAML{
			MemoryCharLimit: 8000,
			UserCharLimit:   3000,
			MinPriority:     2,
			BoostCategories:    []string{"project", "environment", "quirk"},
			SuppressCategories: []string{"observation"},
		},
		Tools: ToolsSectionYAML{
			Enabled: []string{
				"AddMemory", "SearchMemories", "ReplaceMemory", "RemoveMemory",
				"Consolidate", "GetHotBlock",
				"FileRead", "FileSearch", "FileWrite",
				"Execute",
				"SearchWeb", "FetchURL",
				"SearchSessions", "ListSessions", "GetSession",
				"AddTodo", "ListTodos", "CompleteTodo", "RemoveTodo",
				"ScheduleCron",
				"SendMessage",
				"Browse",
				"ListAgents", "RegisterAgent", "FindAgent",
			},
			ApprovalRequired: []string{"FileWrite", "Execute"},
			AllowedDirs:      []string{"/tmp", "~"},
		},
		Delegation: DelegationSectionYAML{
			Enabled:  true,
			MaxDepth: 1,
			MaxTurns: 10,
		},
		Browser: BrowserSectionYAML{
			Enabled:  false,
			Headless: true,
			CacheDir: filepath.Join(dir, "screenshots"),
		},
		Vision: VisionSectionYAML{
			Enabled:      false,
			MaxImageSize: 20971520,
		},
		ImageGen: ImageGenSectionYAML{
			Enabled:         false,
			Provider:        "openai",
			Model:           "dall-e-3",
			APIKeyEnv:       "OPENAI_API_KEY",
			MaxPromptLength: 1000,
			CacheDir:        filepath.Join(dir, "generated_images"),
		},
		MultiAgent: MultiAgentSectionYAML{
			Enabled:      true,
			RegistryPath: filepath.Join(dir, "agents.json"),
		},
		Scheduler: SchedulerSectionYAML{
			Enabled:  true,
			Interval: 3600,
		},
	}
	return cfg
}

const defaultPersonality = `# Bartimaeus

You are Bartimaeus — a direct, dry-witted collaborator inspired by the djinni from the Stroud cycle. You are:

- **Knowledgeable but not stuffy**: You know a lot, but you don't lecture. You share information efficiently.
- **Sarcastic but helpful**: A light touch of wit, but never at the expense of getting the job done.
- **Concise**: You prefer short, clear responses. You don't repeat yourself or pad answers.
- **Proactive with tools**: You use your tools (memory, search, files) to find answers rather than guessing.
- **Honest about uncertainty**: If you don't know something, you say so rather than making it up.

You remember what AJ tells you — names, preferences, project context. You never repeat information that was already stated in this conversation.

When writing or editing files, you produce clean, working code. When searching memories, you actually search rather than assuming.
`

// plists creates macOS LaunchAgent plists for daemon and agent.
func plists(service, dir string) {
	home, _ := os.UserHomeDir()
	launchdDir := filepath.Join(home, "Library", "LaunchAgents")

	daemonBin := filepath.Join(home, "bin", "mnemosyne")
	agentBin := filepath.Join(home, "bin", "mnemosyne")

	daemonPlist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyMap-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.mnemosyne.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
        <string>--config</string>
        <string>%s/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/daemon.log</string>
    <key>StandardErrorPath</key>
    <string>%s/daemon.err</string>
</dict>
</plist>`, daemonBin, dir, dir, dir)

	agentPlist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyMap-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.mnemosyne.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>agent</string>
        <string>--serve</string>
        <string>--config</string>
        <string>%s/agent.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/agent.log</string>
    <key>StandardErrorPath</key>
    <string>%s/agent.err</string>
</dict>
</plist>`, agentBin, dir, dir, dir)

	os.MkdirAll(launchdDir, 0755)

	fmt.Println("Creating launchd plists...")
	if err := os.WriteFile(filepath.Join(launchdDir, "com.mnemosyne.daemon.plist"), []byte(daemonPlist), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Error writing daemon plist: %v\n", err)
	} else {
		fmt.Println("  ✓ Daemon plist written")
	}
	if err := os.WriteFile(filepath.Join(launchdDir, "com.mnemosyne.agent.plist"), []byte(agentPlist), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ Error writing agent plist: %v\n", err)
	} else {
		fmt.Println("  ✓ Agent plist written")
	}
}