// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// sanitizeUTF8 replaces invalid UTF-8 bytes with the Unicode replacement character (U+FFFD).
// This is necessary because protobuf/gRPC requires valid UTF-8 strings, and some
// imported Hermes data or LLM outputs may contain invalid byte sequences.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// Replace invalid bytes with U+FFFD
	var b strings.Builder
	b.Grow(len(s) + len(s)/10) // extra room for replacements
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString("\uFFFD")
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// Session represents a conversation session.
type Session struct {
	ID           string
	AgentID      string
	Source       string // "cli", "telegram", "cron", etc.
	Title        string
	CreatedAt    int64
	LastActiveAt int64
	MessageCount int32
	EndedAt      int64 // 0 = active
}

// Message represents a single message in a session.
type Message struct {
	ID         int64
	SessionID  string
	Role       string // "user", "assistant", "system", "tool"
	Content    string
	ToolCallID string
	ToolName   string
	Timestamp  int64
}

// CreateSession starts a new session.
func (s *Store) CreateSession(agentID, source, title string) (*Session, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent_id must not be empty")
	}
	if source == "" {
		source = "cli"
	}

	// Ensure agent exists (has its own lock)
	_, _, err := s.EnsureAgent(agentID, "", "")
	if err != nil {
		return nil, fmt.Errorf("ensure agent: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	id := uuid.New().String()
	now := time.Now().Unix()

	_, err = s.db.Exec(`
		INSERT INTO sessions (id, agent_id, source, title, created_at, last_active_at, message_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, agentID, source, title, now, now, 0,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Session{
		ID:           id,
		AgentID:      agentID,
		Source:       source,
		Title:        title,
		CreatedAt:    now,
		LastActiveAt: now,
		MessageCount: 0,
	}, nil
}

// AddMessages appends messages to a session within a transaction.
// Also updates the session's last_active_at and message_count atomically.
func (s *Store) AddMessages(sessionID string, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range messages {
		// Sanitize the content to ensure it is valid UTF-8
		sanitizedContent := sanitizeUTF8(m.Content)

		_, err := tx.Exec(`
			INSERT INTO messages (session_id, role, content, tool_call_id, tool_name, timestamp)
			VALUES (?, ?, ?, ?, ?, ?)`,
			sessionID, m.Role, sanitizedContent, m.ToolCallID, m.ToolName, m.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("insert message: %w", err)
		}
	}

	// Update session's last_active_at and message_count within the same transaction
	_, err = tx.Exec(`
		UPDATE sessions SET
			last_active_at = ?,
			message_count = message_count + ?
		WHERE id = ?`,
		time.Now().Unix(), len(messages), sessionID,
	)
	if err != nil {
		return fmt.Errorf("update session: %w", err)
	}

	return tx.Commit()
}

// EndSession marks a session as ended.
func (s *Store) EndSession(sessionID string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec(`
		UPDATE sessions SET ended_at = ? WHERE id = ? AND ended_at = 0`,
		time.Now().Unix(), sessionID,
	)
	if err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found or already ended: %s", sessionID)
	}
	return nil
}

// GetSession retrieves a session by ID.
func (s *Store) GetSession(sessionID string) (*Session, error) {
	sess := &Session{}
	err := s.db.QueryRow(`
		SELECT id, agent_id, source, title, created_at, last_active_at, message_count, ended_at
		FROM sessions WHERE id = ?`, sessionID,
	).Scan(&sess.ID, &sess.AgentID, &sess.Source, &sess.Title,
		&sess.CreatedAt, &sess.LastActiveAt, &sess.MessageCount, &sess.EndedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return sess, nil
}

// EnsureAgent creates an agent record if it doesn't exist.
// Returns the agent and whether it was newly created.
func (s *Store) EnsureAgent(agentID, name, role string) (*Agent, bool, error) {
	if agentID == "" {
		return nil, false, fmt.Errorf("agent_id must not be empty")
	}
	if len(agentID) > MaxIDLength {
		return nil, false, fmt.Errorf("agent_id too long: %d chars, max %d", len(agentID), MaxIDLength)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	// Check if agent already exists
	var existingCount int
	row := s.db.QueryRow("SELECT COUNT(*) FROM agents WHERE id = ?", agentID)
	if err := row.Scan(&existingCount); err != nil {
		return nil, false, fmt.Errorf("check agent existence: %w", err)
	}
	isNew := existingCount == 0

	_, err := s.db.Exec(`
		INSERT INTO agents (id, name, role, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET last_seen_at = excluded.last_seen_at`,
		agentID, name, role, now, now,
	)
	if err != nil {
		return nil, false, fmt.Errorf("ensure agent: %w", err)
	}

	// Read the actual created_at (preserved on update)
	var createdAt int64
	row = s.db.QueryRow("SELECT created_at FROM agents WHERE id = ?", agentID)
	if err := row.Scan(&createdAt); err != nil {
		return nil, false, fmt.Errorf("read agent: %w", err)
	}

	return &Agent{
		ID:         agentID,
		Name:       name,
		Role:       role,
		CreatedAt:  createdAt,
		LastSeenAt: now,
	}, isNew, nil
}

// scanSession scans a single row into a Session.
func scanSession(rows *sql.Rows) (*Session, error) {
	sess := &Session{}
	err := rows.Scan(&sess.ID, &sess.AgentID, &sess.Source, &sess.Title,
		&sess.CreatedAt, &sess.LastActiveAt, &sess.MessageCount)
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return sess, nil
}

// GetSessionMessages retrieves all messages for a session.
func (s *Store) GetSessionMessages(sessionID string) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, tool_call_id, tool_name, timestamp
		FROM messages WHERE session_id = ?
		ORDER BY timestamp ASC, id ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolCallID, &m.ToolName, &m.Timestamp); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// AddMessage adds a single message to a session.
func (s *Store) AddMessage(m Message) error {
	return s.AddMessages(m.SessionID, []Message{m})
}

// Agent represents a connected agent.
type Agent struct {
	ID         string
	Name       string
	Role       string
	CreatedAt  int64
	LastSeenAt int64
	Config     string // JSON: HotBlockConfig per role
}