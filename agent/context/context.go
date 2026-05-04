// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package context

// Message is a conversation message in the agent's context.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ConversationContext holds the full state for a conversation loop.
type ConversationContext struct {
	SessionID       string
	Messages        []Message
	SystemPrompt    string
	Tools           []ToolDefinition
	MaxIterations   int
	Iteration       int
	Interrupt       *InterruptSignal
	HotBlockContent string
	AgentID         string
	Temperature     float64
}

// AddMessage appends a message to the conversation.
func (c *ConversationContext) AddMessage(msg Message) {
	c.Messages = append(c.Messages, msg)
}

// LastAssistantMessage returns the last assistant message, or nil.
func (c *ConversationContext) LastAssistantMessage() *Message {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == "assistant" {
			return &c.Messages[i]
		}
	}
	return nil
}

// TokenCount estimates the total token count (rough: 4 chars per token).
func (c *ConversationContext) TokenCount() int {
	total := len(c.SystemPrompt)
	for _, m := range c.Messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Name) + len(tc.Arguments)
		}
	}
	return total / 4
}
