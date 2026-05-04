// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"fmt"
)

// SearchSessions searches session titles using LIKE matching.
// Sessions don't have FTS5 indexes, so we use simple pattern matching.
func (s *Store) SearchSessions(query string, limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > defaultResultLimit {
		limit = defaultResultLimit
	}

	if query == "" {
		return nil, fmt.Errorf("search query must not be empty")
	}

	// Use LIKE for pattern matching on session titles with explicit ESCAPE clause
	pattern := "%" + sanitizeLike(query) + "%"

	rows, err := s.db.Query(`
		SELECT id, agent_id, source, title, created_at, last_active_at, message_count
		FROM sessions
		WHERE title LIKE ? ESCAPE '\'
		ORDER BY last_active_at DESC
		LIMIT ?`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// sanitizeLike escapes special LIKE pattern characters (% and \).
func sanitizeLike(query string) string {
	result := make([]byte, 0, len(query)*2)
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '%' || c == '_' || c == '\\' {
			result = append(result, '\\')
		}
		result = append(result, c)
	}
	return string(result)
}

// ListRecentSessions returns the most recently active sessions.
func (s *Store) ListRecentSessions(limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > defaultResultLimit {
		limit = defaultResultLimit
	}

	rows, err := s.db.Query(`
		SELECT id, agent_id, source, title, created_at, last_active_at, message_count
		FROM sessions
		ORDER BY last_active_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}