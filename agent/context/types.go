// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

// Package context defines the core types that flow through the agent pipeline.
package context

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// StageAction determines what happens after a pipeline iteration.
type StageAction int

const (
	StageActionCONTINUE  StageAction = iota // Loop again (tool call result needs processing)
	StageActionYIELD                        // LLM yielded final text, done
	StageActionTERMINATE                     // Max iterations or fatal error
)

func (a StageAction) String() string {
	switch a {
	case StageActionCONTINUE:
		return "CONTINUE"
	case StageActionYIELD:
		return "YIELD"
	case StageActionTERMINATE:
		return "TERMINATE"
	default:
		return "UNKNOWN"
	}
}

// PreparedRequest is the output of Stage 1 (Request Prep).
type PreparedRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
	APIKwargs    map[string]interface{}
	ImageContent *ImageContent
}

// ImageContent represents image data for vision-capable providers
type ImageContent struct {
	Data     string
	MimeType string
	Detail   string
}

// ToolDefinition describes a tool the LLM can call.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ProviderResult is the output of Stage 2 (Provider Call).
type ProviderResult struct {
	RawResponse    interface{}
	ShouldRetry    bool
	ShouldFallback bool
	Error          error
	StreamState    *StreamState
}

// ParsedResponse is the output of Stage 3 (Response Processing).
type ParsedResponse struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
	Reasoning    string
	Usage        *UsageInfo
}

// UsageInfo tracks token consumption.
type UsageInfo struct {
	PromptTokens    int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

// ToolCall represents a single tool call from the LLM.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult is the outcome of executing a tool call.
type ToolResult struct {
	CallID  string
	Name    string
	Result  string
	IsError bool
}

// ToolDispatchResult is the output of Stage 4 (Tool Dispatch).
type ToolDispatchResult struct {
	ToolResults  []ToolResult
	PreDispatch  []string
	PostDispatch []string
}

// PipelineResult is the final output of the agent loop.
type PipelineResult struct {
	Response         *ParsedResponse
	Iterations       int
	Interrupted      bool
	MaxIterationsHit bool
	SessionID        string
}

// StreamState tracks streaming response accumulation.
type StreamState struct {
	AccumulatedText string
	Done            bool
	Error           error
	Config          StreamConfig
}

// StreamConfig controls streaming behavior and timeouts.
type StreamConfig struct {
	StaleTimeout           float64
	StaleCharRate          float64
	MaxInactivity          float64
	EmptyContentMaxRetries int
}

// DefaultStreamConfig returns sensible defaults.
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		StaleTimeout:           30.0,
		StaleCharRate:          0.5,
		MaxInactivity:         120.0,
		EmptyContentMaxRetries: 3,
	}
}

// InterruptSignal is a thread-safe way to signal pipeline interruption.
type InterruptSignal struct {
	set atomic.Bool
	mu  sync.Mutex
	ch  chan struct{}
}

// NewInterruptSignal creates a new interrupt signal.
func NewInterruptSignal() *InterruptSignal {
	return &InterruptSignal{
		ch: make(chan struct{}),
	}
}

// Set marks the signal as interrupted.
func (s *InterruptSignal) Set() {
	s.set.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.ch:
	default:
		close(s.ch)
	}
}

// IsSet returns whether the signal has been set.
func (s *InterruptSignal) IsSet() bool {
	return s.set.Load()
}

// Channel returns a channel that closes when the signal is set.
func (s *InterruptSignal) Channel() <-chan struct{} {
	return s.ch
}
