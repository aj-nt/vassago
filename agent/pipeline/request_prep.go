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

	// Role + Personality
	if s.Personality != "" {
		// If personality is a full prompt (multi-line, from a file), use it directly.
		// Otherwise, wrap it: "direct" → "You are direct."
		if strings.Contains(s.Personality, "\n") {
			parts = append(parts, s.Personality)
		} else {
			parts = append(parts, fmt.Sprintf("You are %s.", s.Personality))
		}
	}

	// Tool Usage Guide
	parts = append(parts, "")
	parts = append(parts, "## Tool Usage")
	parts = append(parts, "")
	parts = append(parts, "You have tools available. Use them efficiently:")
	parts = append(parts, "- FileSearch: Use to LOCATE files by name or pattern. Returns file paths, not contents.")
	parts = append(parts, "- FileRead: Use to READ a specific file's contents. Only call after you know the path.")
	parts = append(parts, "- FileWrite: Use to WRITE or UPDATE a file. Always read the file first to understand its current state.")
	parts = append(parts, "- Execute: Use to RUN commands (git, go test, etc.). Prefer purpose-built tools over shell commands.")
	parts = append(parts, "- SearchMemories: Use to RECALL information from past sessions.")
	parts = append(parts, "- AddMemory: Use to SAVE important facts for future sessions.")
	parts = append(parts, "")
	parts = append(parts, "IMPORTANT: Do not call tools you don't need. If you already have the information from a previous tool call in this conversation, use it — do not call the same tool again. When writing a file, call FileWrite directly — do not FileSearch first if you already know the path.")

	// Anti-loop directive
	parts = append(parts, "")
	parts = append(parts, "## Rules")
	parts = append(parts, "")
	parts = append(parts, "- Do NOT repeat information you already stated. Each response should add new information only.")
	parts = append(parts, "- Do NOT prefix responses with self-identification (\"Claude\", \"I'm running on X\") unless asked.")
	parts = append(parts, "- If a tool call fails or returns \"not found\", try a different approach — do not retry the same call.")
	parts = append(parts, "- If you cannot complete a task after 2 attempts, tell the user what you tried and ask for clarification.")
	parts = append(parts, "- Use SearchMemories proactively when the user mentions a specific topic, bug, or project.")

	// Hot Block
	if ctx.HotBlockContent != "" {
		parts = append(parts, "[Memory — what you know about the user and environment]")
		parts = append(parts, ctx.HotBlockContent)
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
