// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// HotBlockConfig controls how a hot block is computed for a given agent.
type HotBlockConfig struct {
	BoostCategories    []string // Categories to boost in priority
	SuppressCategories []string // Categories to suppress
	BoostPriority      int32    // Priority >= this gets boosted
	CharLimit          int32    // Max characters in the rendered block
}

// HotBlockResult is the computed hot block output.
type HotBlockResult struct {
	Content    string // Formatted text for prompt injection
	CharCount  int32  // Total characters used
	EntryCount int32  // Number of entries included
}

// ComputeHotBlock builds a formatted text block of the highest-priority memories
// for prompt injection. It respects category boost/suppress, char limits,
// and excludes expired observations.
func (s *Store) ComputeHotBlock(owner, target string, config HotBlockConfig) (*HotBlockResult, error) {
	// Fetch all non-expired entries for this target
	entries, err := s.ListMemories(owner, target, "", config.BoostPriority)
	if err != nil {
		return nil, fmt.Errorf("list memories for hot block: %w", err)
	}

	// Filter out expired observations
	now := unixNow()
	var filtered []*MemoryEntry
	for _, e := range entries {
		if e.ExpiresAt > 0 && e.ExpiresAt < now {
			continue
		}
		filtered = append(filtered, e)
	}

	// Sort: boosted categories first, then suppressed last, then by priority desc
	sort.Slice(filtered, func(i, j int) bool {
		iBoost := isBoosted(filtered[i].Category, config.BoostCategories)
		jBoost := isBoosted(filtered[j].Category, config.BoostCategories)
		iSuppress := isSuppressed(filtered[i].Category, config.SuppressCategories)
		jSuppress := isSuppressed(filtered[j].Category, config.SuppressCategories)

		// Boosted entries come first
		if iBoost && !jBoost {
			return true
		}
		if !iBoost && jBoost {
			return false
		}
		// Suppressed entries come last
		if iSuppress && !jSuppress {
			return false
		}
		if !iSuppress && jSuppress {
			return true
		}
		// Within same boost/suppress status, higher priority first
		if filtered[i].Priority != filtered[j].Priority {
			return filtered[i].Priority > filtered[j].Priority
		}
		// Same priority: more recently updated first
		return filtered[i].UpdatedAt > filtered[j].UpdatedAt
	})

	// Build the text block, respecting char limit
	charLimit := config.CharLimit
	if charLimit <= 0 {
		charLimit = DefaultHotBlockCharLimit
	}

	var lines []string
	var totalChars int32
	var entryCount int32

	for _, e := range filtered {
		line := formatEntry(e)
		lineLen := int32(len(line))

		// Always include at least one entry even if it exceeds the limit
		if entryCount > 0 && totalChars+lineLen > charLimit {
			break
		}

		lines = append(lines, line)
		totalChars += lineLen
		entryCount++
	}

	content := strings.Join(lines, "\n")
	return &HotBlockResult{
		Content:    content,
		CharCount:  totalChars,
		EntryCount: entryCount,
	}, nil
}

// ComputeHotBlockWithSession builds a hot block with optional last-session context
// prepended above the memory entries.
func (s *Store) ComputeHotBlockWithSession(owner, target string, config HotBlockConfig, lastSessionID, lastSessionSummary string) (*HotBlockResult, error) {
	result, err := s.ComputeHotBlock(owner, target, config)
	if err != nil {
		return nil, err
	}

	if lastSessionID != "" {
		prefix := fmt.Sprintf("[session] Last session: %s — %s", lastSessionID, lastSessionSummary)
		if result.Content != "" {
			result.Content = prefix + "\n" + result.Content
		} else {
			result.Content = prefix
		}
		result.CharCount += int32(len(prefix)) + 1 // +1 for newline
	}

	return result, nil
}
func formatEntry(e *MemoryEntry) string {
	if e.Key != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Category, e.Key, e.Content)
	}
	return fmt.Sprintf("[%s] %s", e.Category, e.Content)
}

func isBoosted(category string, boostCategories []string) bool {
	for _, c := range boostCategories {
		if strings.EqualFold(category, c) {
			return true
		}
	}
	return false
}

func isSuppressed(category string, suppressCategories []string) bool {
	for _, c := range suppressCategories {
		if strings.EqualFold(category, c) {
			return true
		}
	}
	return false
}

func unixNow() int64 {
	return time.Now().Unix()
}
