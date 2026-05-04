// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

// StreamEvent represents a single event during streaming output.
// Providers emit these as they receive SSE chunks from the LLM.
type StreamEvent struct {
	// Type identifies the event kind.
	Type StreamEventType

	// Text is the text delta for text events.
	Text string

	// ToolCallID is the tool use ID for tool_start/tool_delta events.
	ToolCallID string

	// ToolName is the tool name for tool_start events.
	ToolName string

	// ToolInputDelta is the partial JSON input for tool_delta events.
	ToolInputDelta string

	// Error is set if an error occurred during streaming.
	Error error

	// Done is true for the final event in the stream.
	Done bool

	// Usage is set on the final event with token counts.
	Usage *UsageInfo
}

// StreamEventType identifies the kind of streaming event.
type StreamEventType int

const (
	// StreamText is a text delta — print it immediately.
	StreamText StreamEventType = iota

	// StreamToolStart signals a tool_use block is starting.
	// The agent should stop streaming text and wait for the full tool call.
	StreamToolStart

	// StreamToolDelta is a partial JSON input for a tool call in progress.
	StreamToolDelta

	// StreamToolEnd signals the tool call input is complete.
	StreamToolEnd

	// StreamToolExec signals tool execution is starting (after the tool_use block
	// is parsed and before dispatch). Emitted by the agent, not the provider.
	StreamToolExec

	// StreamError means something went wrong.
	StreamError

	// StreamDone means the response is complete.
	StreamDone
)