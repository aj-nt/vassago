// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// TokenCounter estimates token counts for text.
// Different models use different tokenizers, so counting
// should be model-specific when possible.
type TokenCounter interface {
	// CountTokens returns the estimated number of tokens in the text.
	CountTokens(text string) int
}

// --- Ratio-based counter (fallback) ---

// RatioCounter estimates tokens as len(text) / Ratio.
// This is the fallback for models without a known tokenizer.
type RatioCounter struct {
	Ratio float64 // characters per token (default: 4.0)
}

// NewRatioCounter creates a counter with the given ratio.
func NewRatioCounter(ratio float64) *RatioCounter {
	if ratio <= 0 {
		ratio = 4.0
	}
	return &RatioCounter{Ratio: ratio}
}

// CountTokens estimates token count from character length.
func (r *RatioCounter) CountTokens(text string) int {
	return int(float64(len(text)) / r.Ratio)
}

// --- Default counter (chars/4) ---

// DefaultTokenCounter is the legacy chars/4 estimator, always available.
var DefaultTokenCounter TokenCounter = NewRatioCounter(4.0)

// --- Tiktoken-based counter ---

// TiktokenCounter uses tiktoken-go for accurate OpenAI model counting.
// Thread-safe; lazily loads encodings on first use.
type TiktokenCounter struct {
	mu       sync.Mutex
	encoding string            // e.g., "cl100k_base", "o200k_base"
	loaded   *tiktoken.Tiktoken // cached encoding, nil until first use
}

// NewTiktokenCounter creates a tiktoken-backed counter for the given encoding.
// Common encodings:
//   - "cl100k_base" — GPT-4, GPT-4o, GPT-3.5-turbo
//   - "o200k_base"  — o1, o3, o4-mini
//   - "r50k_base"   — GPT-3 models (davinci, etc.)
func NewTiktokenCounter(encoding string) *TiktokenCounter {
	if encoding == "" {
		encoding = "cl100k_base"
	}
	return &TiktokenCounter{encoding: encoding}
}

// CountTokens accurately counts tokens using the model's tokenizer.
func (t *TiktokenCounter) CountTokens(text string) int {
	tke := t.ensureLoaded()
	if tke == nil {
		// Fall back to chars/4 on loading failure
		return DefaultTokenCounter.CountTokens(text)
	}
	tokens := tke.Encode(text, nil, nil)
	return len(tokens)
}

func (t *TiktokenCounter) ensureLoaded() *tiktoken.Tiktoken {
	if t.loaded != nil {
		return t.loaded
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.loaded != nil {
		return t.loaded
	}
	tke, err := tiktoken.GetEncoding(t.encoding)
	if err != nil {
		// Encoding not found — leave loaded as nil (falls back to DefaultTokenCounter)
		return nil
	}
	t.loaded = tke
	return t.loaded
}

// --- CounterForModel selects the best counter for a given model name ---

// CounterForModel returns the appropriate TokenCounter for a model.
// Uses tiktoken for OpenAI/OpenRouter models, ratio-based for others.
//
// Model detection is prefix-based (case-insensitive):
//   - "gpt-4", "gpt-3.5", "o1", "o3", "o4" → cl100k_base or o200k_base
//   - "claude" → Ratio 3.5 (Anthropic uses a similar subword tokenizer)
//   - "deepseek" → Ratio 3.5 (DeepSeek uses a BPE tokenizer similar to GPT)
//   - "glm" → Ratio 4.0 (conservative for unknown tokenizers)
//   - Everything else → chars/4 ratio
func CounterForModel(modelName string) TokenCounter {
	lower := strings.ToLower(modelName)

	// OpenAI o-series models use o200k_base
	if strings.HasPrefix(lower, "o1") || strings.HasPrefix(lower, "o3") || strings.HasPrefix(lower, "o4") {
		return NewTiktokenCounter("o200k_base")
	}

	// All other OpenAI models (GPT-4, GPT-3.5, etc.) use cl100k_base
	if strings.HasPrefix(lower, "gpt-") {
		return NewTiktokenCounter("cl100k_base")
	}

	// Claude and DeepSeek use BPE tokenizers with similar ratios to GPT
	// These are not tiktoken-compatible, but the ratio is well-characterized
	if strings.HasPrefix(lower, "claude") {
		return NewRatioCounter(3.5)
	}
	if strings.Contains(lower, "deepseek") {
		return NewRatioCounter(3.5)
	}

	// Zhipu GLM series — unknown tokenizer, default to chars/4
	if strings.HasPrefix(lower, "glm") {
		return NewRatioCounter(4.0)
	}

	// Fallback: default chars/4
	return DefaultTokenCounter
}

// --- Wire into ConversationContext ---

// SetTokenCounter sets the token counter for this conversation context.
// Called by the agent before the pipeline starts, based on the selected model.
// If nil, DefaultTokenCounter (chars/4) is used.
func (c *ConversationContext) SetTokenCounter(counter TokenCounter) {
	if counter == nil {
		counter = DefaultTokenCounter
	}
	c.tokenCounter = counter
}

// getTokenCounter returns the active counter, defaulting if not set.
func (c *ConversationContext) getTokenCounter() TokenCounter {
	if c.tokenCounter != nil {
		return c.tokenCounter
	}
	return DefaultTokenCounter
}

// --- Message-aware counting ---

// CountMessageTokens counts tokens for a conversation message,
// including role overhead and tool call metadata.
// Role overhead: approximately 4 tokens per message (OpenAI convention).
func CountMessageTokens(counter TokenCounter, msg Message) int {
	tokens := counter.CountTokens(msg.Content) + 4 // role overhead
	for _, tc := range msg.ToolCalls {
		tokens += counter.CountTokens(tc.Name + string(tc.Arguments)) + 5 // tool call overhead
	}
	return tokens
}

// FormatTokenUsage returns a human-readable token usage string.
func FormatTokenUsage(used, maximum int) string {
	if maximum <= 0 {
		return fmt.Sprintf("%d tokens", used)
	}
	pct := float64(used) / float64(maximum) * 100
	return fmt.Sprintf("%d/%d tokens (%.0f%%)", used, maximum, pct)
}
