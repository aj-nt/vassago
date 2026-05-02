package store

import (
	"fmt"
	"strings"
)

// Search performs a full-text search using FTS5.
// Multi-word queries use OR semantics by default.
// Special tokens (colons, dots) are quoted to prevent FTS5 column filter errors.
func (s *Store) Search(query string, target string, limit int) ([]*MemoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > defaultResultLimit {
		limit = defaultResultLimit
	}

	// Sanitize the FTS5 query
	sanitized := sanitizeFTS5(query)

	// Build the query with optional target filter
	sqlQuery := `
		SELECT m.id, m.target, m.category, m.key, m.content, m.priority,
		       m.source_agent, m.created_at, m.updated_at, m.accessed_at, m.expires_at
		FROM memories m
		JOIN memories_fts fts ON m.rowid = fts.rowid
		WHERE memories_fts MATCH ?`
	args := []any{sanitized}

	if target != "" {
		sqlQuery += " AND m.target = ?"
		args = append(args, target)
	}
	sqlQuery += " ORDER BY m.priority DESC, m.updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

// sanitizeFTS5 cleans a user query for safe use in FTS5 MATCH.
// Preserves OR, AND, NOT, NEAR operators.
// Quotes tokens containing colons or dots to prevent FTS5 column filter errors.
// Multi-word queries with no explicit operators use OR (not AND) semantics.
func sanitizeFTS5(query string) string {
	words := strings.Fields(query)
	if len(words) == 0 {
		return `""` // Empty query — return empty match
	}

	var result []string
	for _, word := range words {
		// Preserve FTS5 operators
		switch strings.ToUpper(word) {
		case "AND", "OR", "NOT":
			result = append(result, strings.ToUpper(word))
			continue
		case "NEAR":
			result = append(result, "NEAR")
			continue
		}

		// Quote tokens with special FTS5 chars (colon = column filter, dot = tokenization)
		if strings.Contains(word, ":") || strings.Contains(word, ".") {
			result = append(result, `"`+word+`"`)
			continue
		}

		result = append(result, word)
	}

	// If no explicit operators, use OR (broad recall for agent searches)
	hasOperator := false
	for _, w := range result {
		upper := strings.ToUpper(w)
		if upper == "AND" || upper == "OR" || upper == "NOT" || upper == "NEAR" {
			hasOperator = true
			break
		}
	}
	if !hasOperator && len(result) > 1 {
		// Insert OR between all terms
		joined := make([]string, 0, len(result)*2-1)
		for i, w := range result {
			if i > 0 {
				joined = append(joined, "OR")
			}
			joined = append(joined, w)
		}
		return strings.Join(joined, " ")
	}

	return strings.Join(result, " ")
}