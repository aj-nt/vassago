// This file is part of Vassago.
// See LICENSE for license information.

package extraction

import (
	"fmt"
)

// Config configures memory extraction behavior.
// Debounce = extraction fires N seconds after the last message.
// HardCap = extraction fires at least every N seconds even if messages keep coming.
// Set DebounceInterval=0 to disable automatic extraction entirely.
type Config struct {
	// DebounceInterval is the quiet period (seconds) required before extraction triggers.
	// After the last message in a session, the scheduler waits this long.
	// If another message arrives, the timer resets.
	DebounceInterval int // seconds; 0 disables automatic extraction

	// HardCapInterval is the maximum interval (seconds) between extractions for a
	// long-running session. If messages keep flowing without enough quiet time,
	// extraction fires at least this often.
	HardCapInterval int // seconds

	// MaxMessagesPerExtraction limits how many recent messages are sent to
	// the LLM for analysis. Older messages are truncated.
	MaxMessagesPerExtraction int

	// MinMessagesForExtraction prevents extraction calls on sessions that
	// have too few messages to be meaningful.
	MinMessagesForExtraction int

	// Priority is the default priority for auto-extracted memories (1-5).
	Priority int32

	// Model is the LLM model name to use for extraction.
	Model string
}

// DefaultConfig returns sensible defaults for memory extraction.
func DefaultConfig() Config {
	return Config{
		DebounceInterval:         30,
		HardCapInterval:          900, // 15 minutes
		MaxMessagesPerExtraction: 50,
		MinMessagesForExtraction: 4,
		Priority:                 2,
	}
}

// Validate checks the config for invalid values.
func (c Config) Validate() error {
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	if c.DebounceInterval < 0 {
		return fmt.Errorf("debounce_interval must be >= 0, got %d", c.DebounceInterval)
	}
	if c.DebounceInterval > 0 && c.DebounceInterval < 5 {
		return fmt.Errorf("debounce_interval must be >= 5 when enabled, got %d", c.DebounceInterval)
	}
	if c.DebounceInterval > 0 && c.DebounceInterval > 600 {
		return fmt.Errorf("debounce_interval must be <= 600, got %d", c.DebounceInterval)
	}
	return nil
}

// Candidate is a potential memory extracted from session messages by the LLM.
type Candidate struct {
	Key      string
	Content  string
	Category string
	Target   string // "memory" or "user"
	Priority int32
}

// Validate checks the candidate for required fields and valid values.
func (c Candidate) Validate() error {
	if c.Key == "" {
		return fmt.Errorf("key is required")
	}
	if c.Content == "" {
		return fmt.Errorf("content is required")
	}
	if c.Category == "" {
		return fmt.Errorf("category is required")
	}
	if c.Target != "memory" && c.Target != "user" {
		return fmt.Errorf("target must be 'memory' or 'user', got %q", c.Target)
	}
	if c.Priority < 1 || c.Priority > 5 {
		return fmt.Errorf("priority must be 1-5, got %d", c.Priority)
	}
	return nil
}

// SessionMessage is a lightweight read-only view of a session message
// used for extraction prompts. Tool outputs are truncated to avoid
// blowing out the LLM context.
type SessionMessage struct {
	Role      string     // "user", "assistant", "system", "tool"
	Content   string     // truncated if a tool output
	ToolName  string     // populated for tool-call messages
	ToolCalls []ToolCall // populated for assistant messages with tool calls
}

// ToolCall is a lightweight summary of a tool invocation.
type ToolCall struct {
	Name string
	Args string
}
