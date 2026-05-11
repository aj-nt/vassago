// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/client"
	"github.com/aj-nt/mnemosyne/agent/provider"
	agentlog "github.com/aj-nt/mnemosyne/agent/log"
)

// ExtractMemories calls an LLM to extract key information from a conversation
// and saves extracted memories via the MnemoClient.
func ExtractMemories(
	ctx context.Context,
	messages []agentctx.Message,
	prov provider.Provider,
	mnemo client.MnemoClient,
	sessionID string,
	agentName string,
) error {
	if len(messages) == 0 {
		return nil
	}

	// Build a conversation summary for the extraction prompt
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
		}
	}
	convText := sb.String()
	if strings.TrimSpace(convText) == "" {
		return nil
	}

	extractionPrompt := `You are a memory extraction system. Review the conversation below and extract key facts, preferences, decisions, and important information that should be remembered for future conversations.

Output a JSON array of objects with these fields:
- "key": a short unique identifier (e.g., "user_preference_theme")
- "content": the fact or preference to remember
- "category": one of "user", "project", "observation", "quirk", "environment"
- "priority": importance from 1-5 (5 = critical, 1 = minor)

If nothing is worth remembering, output an empty array [].

Conversation:
` + convText + `

JSON output:`

	// Build a minimal conversation context for the provider
	userMsg := agentctx.Message{Role: "user", Content: extractionPrompt}
	convCtx := &agentctx.ConversationContext{
		Messages: []agentctx.Message{userMsg},
	}

	prepared, err := prov.Prepare(ctx, convCtx, agentctx.PreparedRequest{})
	if err != nil {
		return fmt.Errorf("memory extraction prepare: %w", err)
	}

	result, err := prov.Execute(ctx, prepared)
	if err != nil {
		return fmt.Errorf("memory extraction execute: %w", err)
	}

	parsed, err := prov.Parse(ctx, result)
	if err != nil {
		return fmt.Errorf("memory extraction parse: %w", err)
	}

	extractedText := parsed.Text
	// Strip markdown code fences if present
	extractedText = strings.TrimPrefix(extractedText, "```json")
	extractedText = strings.TrimPrefix(extractedText, "```")
	extractedText = strings.TrimSuffix(extractedText, "```")
	extractedText = strings.TrimSpace(extractedText)

	var entries []struct {
		Key      string `json:"key"`
		Content  string `json:"content"`
		Category string `json:"category"`
		Priority int32  `json:"priority"`
	}
	if err := json.Unmarshal([]byte(extractedText), &entries); err != nil {
		return fmt.Errorf("memory extraction json parse: %w (text: %q)", err, extractedText)
	}

	for _, e := range entries {
		if e.Key == "" || e.Content == "" {
			continue
		}
		_, err := mnemo.AddMemory(ctx, "memory", e.Category, e.Key, e.Content, e.Priority, agentName)
		if err != nil {
			// Log but don't fail — one bad memory shouldn't block the rest
			agentlog.Printf("memory extraction: failed to save key %q: %v\n", e.Key, err)
		}
	}

	return nil
}