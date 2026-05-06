// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

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

	// Tool guide — concise, action-oriented
	parts = append(parts, "")
	parts = append(parts, "## Tools")
	parts = append(parts, "FileSearch → find files by name/pattern | FileRead → read file contents (need path first) | FileWrite → write/update files | Execute → run shell commands | SearchMemories → recall past info | AddMemory → save important facts")
	parts = append(parts, "")
	parts = append(parts, "Rules: Do NOT restate tool capabilities in responses. Do NOT call a tool if you already have the answer from a prior call. If a tool fails, try a different approach — never retry the same failing call.")

	// Anti-repetition directive
	parts = append(parts, "")
	parts = append(parts, "## Anti-repetition")
	parts = append(parts, "- NEVER list or describe your tools/capabilities in responses. The user already knows what you can do.")
	parts = append(parts, "- NEVER say \"I can use X to Y\" or \"Let me use my Z tool\" — just USE the tool, or give the answer directly.")
	parts = append(parts, "- ALWAYS include text in your response, even when making tool calls. Explain briefly what you're doing.")
	parts = append(parts, "- Do NOT make more than 2 consecutive tool calls without generating text in between.")
	parts = append(parts, "- Do NOT repeat information you already stated. Each response adds new info only.")
	parts = append(parts, "- Do NOT prefix responses with self-identification (\"Claude\", \"I'm running on X\") unless asked.")
	parts = append(parts, "- If you cannot complete a task after 2 attempts, describe what you tried and ask for clarification.")
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
