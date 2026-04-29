package pipeline

import (
	"fmt"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// RequestPrepStage builds the system prompt, injects the hot block,
// and prepares API kwargs for the LLM call.
type RequestPrepStage struct {
	Personality string // Personality directive for system prompt
}

// NewRequestPrepStage creates a new Stage 1.
func NewRequestPrepStage(personality string) *RequestPrepStage {
	return &RequestPrepStage{Personality: personality}
}

// Process builds a PreparedRequest from the conversation context.
func (s *RequestPrepStage) Process(ctx *agentctx.ConversationContext) agentctx.PreparedRequest {
	systemPrompt := s.buildSystemPrompt(ctx)

	// Set provider-specific parameters
	apiKwargs := map[string]interface{}{
		"temperature": ctx.Temperature,
	}

	return agentctx.PreparedRequest{
		SystemPrompt: systemPrompt,
		Messages:     ctx.Messages,
		Tools:        ctx.Tools,
		APIKwargs:    apiKwargs,
	}
}

// buildSystemPrompt constructs the full system prompt with hot block injection.
func (s *RequestPrepStage) buildSystemPrompt(ctx *agentctx.ConversationContext) string {
	var parts []string

	// Personality directive
	if s.Personality != "" {
		// If personality is a full prompt (multi-line, from a file), use it directly.
		// Otherwise, wrap it: "direct" → "You are direct."
		if strings.Contains(s.Personality, "\n") {
			parts = append(parts, s.Personality)
		} else {
			parts = append(parts, fmt.Sprintf("You are %s.", s.Personality))
		}
	}

	// Anti-repetition directive — this is critical for multi-turn conversations
	// where tool results from prior turns get compacted into summaries.
	// Without this, models tend to re-assert information from old tool outputs.
	parts = append(parts, "")
	parts = append(parts, "Do NOT repeat information you've already stated in earlier messages. If you already told the user their name, preferences, or any other fact, do not re-state it unless they ask again. Each response should add new information or address only the current question.")
	parts = append(parts, "")
	parts = append(parts, "Proactive memory usage: When the user mentions a specific topic, bug, project, or past event, SEARCH your memories with SearchMemories before asking the user to explain it. Your memory may already contain the full context. Do NOT ask the user for ticket numbers, repo names, or background that you can look up yourself — look it up first, then ask only if you still need clarification.")

	// Memory section
	if ctx.HotBlockContent != "" {
		parts = append(parts, "[Memory — what you know about the user and environment]")
		parts = append(parts, ctx.HotBlockContent)
		parts = append(parts, "")
		parts = append(parts, "[Instructions — how to use memory]")
		parts = append(parts, "- You can search memories with SearchMemories when you need something not in the hot block.")
		parts = append(parts, "- Use AddMemory to persist important facts, preferences, and lessons learned.")
		parts = append(parts, "- Use ReplaceMemory to update existing memories (match by key).")
		parts = append(parts, "- Use RemoveMemory to delete a memory by key.")
		parts = append(parts, "- Observations auto-expire after 30 days unless promoted to a higher category.")
		parts = append(parts, "- Other agents may update memories while you work. If you receive a hot block update, trust it.")
	}

	return joinParts(parts)
}

func joinParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}
