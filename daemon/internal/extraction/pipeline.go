// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package extraction

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LLMClient is the interface for calling an LLM to extract memories.
// This is the only external dependency of the extraction pipeline —
// everything else is pure data transformation.
type LLMClient interface {
	Complete(prompt string) (string, error)
}

// Pipeline orchestrates memory extraction from session messages.
// Format → LLM call → Parse → Validate → Deduplicate → Persist.
type Pipeline struct {
	config Config
	client LLMClient
	store  MemoryStore // nil during testing
}

// MemoryStore is the interface for persisting extracted memories.
// Implemented by the store package.
type MemoryStore interface {
	AddMemory(owner, target, category, key, content string, priority int32, sourceAgent string) (string, error)
	ListMemoryKeys(owner string) ([]string, error)
}

// NewPipeline creates a new extraction pipeline with the given config.
func NewPipeline(config Config, store MemoryStore) *Pipeline {
	return &Pipeline{config: config, store: store}
}

// SetClient sets the LLM client (useful for testing with mocks).
func (p *Pipeline) SetClient(client LLMClient) {
	p.client = client
}

// ExtractFromMessages analyzes session messages and returns extracted memory candidates.
// This is the core extraction step: format → prompt → LLM → parse → validate.
// Persistence is handled separately via PersistCandidates.
func (p *Pipeline) ExtractFromMessages(msgs []SessionMessage) ([]Candidate, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages to extract from")
	}
	if p.client == nil {
		return nil, fmt.Errorf("no LLM client configured")
	}

	prompt := buildExtractionPrompt(msgs)
	response, err := p.client.Complete(prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM extraction failed: %w", err)
	}

	return parseExtractionResponse(response)
}

// ExtractAndPersist runs the full pipeline: extract, deduplicate against
// existing memories, and persist new candidates.
func (p *Pipeline) ExtractAndPersist(owner, sourceAgent string, msgs []SessionMessage) (int, error) {
	if p.store == nil {
		return 0, fmt.Errorf("no store configured for persistence")
	}

	candidates, err := p.ExtractFromMessages(msgs)
	if err != nil {
		return 0, err
	}

	// Build set of existing keys
	existingKeys, err := p.store.ListMemoryKeys(owner)
	if err != nil {
		return 0, fmt.Errorf("list existing keys: %w", err)
	}
	existingSet := make(map[string]bool, len(existingKeys))
	for _, k := range existingKeys {
		existingSet[k] = true
	}

	newCandidates := deduplicateCandidates(candidates, existingSet)

	var persisted int
	for _, c := range newCandidates {
		_, err := p.store.AddMemory(owner, c.Target, c.Category, c.Key, c.Content, c.Priority, sourceAgent)
		if err != nil {
			// Log but continue — don't let one bad candidate block the rest
			continue
		}
		persisted++
	}

	return persisted, nil
}

// buildExtractionPrompt constructs the LLM prompt for memory extraction.
func buildExtractionPrompt(msgs []SessionMessage) string {
	formatted := formatMessagesForExtraction(msgs)

	prompt := fmt.Sprintf(`You are a memory extraction system. Analyze the following conversation and extract durable facts that should be remembered permanently.

For each fact, classify it as:
- "memory": technical facts, project conventions, environment details, lessons learned
- "user": personal information about the user (preferences, identity, habits, relationships)

Guidelines:
- Extract facts that will still matter weeks from now
- Skip task progress, session outcomes, completed work logs
- Skip trivial observations
- Write concise, declarative facts (one sentence each)
- Key should be a brief snake_case identifier
- Priority: 2=low, 3=normal, 4=high, 5=critical
- If nothing durable is found, return an empty memories array

Conversation:
%s

Respond with ONLY a JSON object in this format:
{
  "memories": [
    {"key": "snake_case_id", "content": "Declarative fact.", "category": "preference|quirk|personal|project|lesson", "target": "memory|user", "priority": 2}
  ]
}`, formatted)

	return prompt
}

// parseExtractionResponse parses the LLM's JSON response into validated candidates.
// Invalid candidates (missing fields, bad values) are silently skipped.
func parseExtractionResponse(response string) ([]Candidate, error) {
	// Strip markdown code fences if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	if response == "" {
		return nil, fmt.Errorf("empty extraction response")
	}

	var result struct {
		Memories []Candidate `json:"memories"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("parse extraction response: %w", err)
	}

	if len(result.Memories) == 0 {
		return nil, fmt.Errorf("no memories found in extraction response")
	}

	var valid []Candidate
	for _, c := range result.Memories {
		if err := c.Validate(); err != nil {
			continue
		}
		valid = append(valid, c)
	}

	if len(valid) == 0 {
		return nil, fmt.Errorf("no valid candidates after filtering")
	}

	return valid, nil
}

// deduplicateCandidates filters out candidates whose keys already exist
// in the store. Among candidates with the same key, the highest priority wins.
func deduplicateCandidates(candidates []Candidate, existing map[string]bool) []Candidate {
	seen := make(map[string]*Candidate)
	var order []string

	for i := range candidates {
		c := &candidates[i]
		if existing[c.Key] {
			continue
		}
		if prev, ok := seen[c.Key]; ok {
			// Keep the higher priority version
			if c.Priority > prev.Priority {
				seen[c.Key] = c
			}
		} else {
			seen[c.Key] = c
			order = append(order, c.Key)
		}
	}

	result := make([]Candidate, 0, len(order))
	for _, key := range order {
		result = append(result, *seen[key])
	}
	return result
}
