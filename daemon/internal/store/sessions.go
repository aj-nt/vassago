package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
	_, err := s.EnsureAgent(agentID, "", "")
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
	defer tx.Rollback()

	for _, m := range messages {
		_, err := tx.Exec(`
			INSERT INTO messages (session_id, role, content, tool_call_id, tool_name, timestamp)
			VALUES (?, ?, ?, ?, ?, ?)`,
			sessionID, m.Role, m.Content, m.ToolCallID, m.ToolName, m.Timestamp,
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
func (s *Store) EnsureAgent(agentID, name, role string) (*Agent, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent_id must not be empty")
	}
	if len(agentID) > MaxIDLength {
		return nil, fmt.Errorf("agent_id too long: %d chars, max %d", len(agentID), MaxIDLength)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	_, err := s.db.Exec(`
		INSERT INTO agents (id, name, role, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET last_seen_at = excluded.last_seen_at`,
		agentID, name, role, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("ensure agent: %w", err)
	}

	return &Agent{
		ID:         agentID,
		Name:       name,
		Role:       role,
		CreatedAt:  now,
		LastSeenAt: now,
	}, nil
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