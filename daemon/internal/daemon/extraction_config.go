// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package daemon

// ExtractionConfig configures the autonomous memory extraction system.
// Extraction fires when the daemon detects new messages in a session.
// The debounced scheduler triggers the LLM-based extraction pipeline,
// which persists new memories directly without agent involvement.
type ExtractionConfig struct {
	// Enabled turns on autonomous memory extraction.
	// When false, the daemon only stores what agents explicitly tell it.
	Enabled bool `yaml:"enabled"`

	// Model is the LLM model used for extraction (e.g., "glm-5.1:cloud").
	Model string `yaml:"model"`

	// Provider is the inference provider (e.g., "ollama", "openai").
	Provider string `yaml:"provider"`

	// BaseURL is the provider's API endpoint.
	BaseURL string `yaml:"base_url"`

	// APIKey authenticates with the provider.
	APIKey string `yaml:"api_key"`

	// DebounceInterval is the quiet period (seconds) after the last message
	// before extraction fires. Default: 30.
	DebounceInterval int `yaml:"debounce_interval"`

	// HardCapInterval is the maximum interval (seconds) between extractions
	// for a long-running session. Default: 900 (15 minutes).
	HardCapInterval int `yaml:"hard_cap_interval"`
}
