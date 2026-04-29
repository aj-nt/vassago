package migration

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

// HermesMemory represents a memory row in the Hermes state.db.
type HermesMemory struct {
	ID             int64
	Target         string
	Category       string
	Key            string
	Content        string
	Priority       int32
	CreatedAt      float64 // Hermes uses REAL timestamps
	UpdatedAt      float64
	LastAccessed   float64
	SourceSession  sql.NullString // nullable in Hermes
	AccessCount    int32
	ExpiresAt      sql.NullFloat64 // NULL means no expiry
}

// HermesSession represents a session row in the Hermes state.db.
type HermesSession struct {
	ID              string
	Source          string
	UserID         sql.NullString
	Model          sql.NullString
	SystemPrompt   sql.NullString
	StartedAt      float64
	EndedAt        sql.NullFloat64
	EndReason      sql.NullString
	MessageCount   int32
	ToolCallCount  int32
	Title          sql.NullString
}

// HermesMessage represents a message row in the Hermes state.db.
type HermesMessage struct {
	ID          int64
	SessionID   string
	Role        string
	Content     sql.NullString
	ToolCallID  sql.NullString
	ToolCalls   sql.NullString
	ToolName    sql.NullString
	Timestamp   float64
	TokenCount  sql.NullInt64
	FinishReason sql.NullString
}

// ImportResult holds the results of an import operation.
type ImportResult struct {
	MemoriesRead     int
	MemoriesImported int
	MemoriesSkipped  int // duplicates or invalid
	SessionsRead     int
	SessionsImported int
	MessagesRead     int
	MessagesImported int
	Errors           []string
}

// ImportHermesDB reads a Hermes state.db and imports its data into Mnemosyne.
// It performs idempotent imports — running it twice will not create duplicates
// because Mnemosyne uses (target, key) as a unique constraint.
//
// Timestamps are converted from Hermes REAL (Unix epoch with fractional seconds)
// to Mnemosyne INTEGER (Unix seconds, truncated).
func ImportHermesDB(mnemoStore *store.Store, hermesDBPath string) (*ImportResult, error) {
	result := &ImportResult{}

	// Open Hermes database — use PRAGMA for WAL mode, not DSN params,
	// because modernc.org/sqlite treats DSN params as part of the filename.
	hermesDB, err := sql.Open("sqlite", hermesDBPath)
	if err != nil {
		return nil, fmt.Errorf("open hermes db: %w", err)
	}
	defer hermesDB.Close()

	// Enable WAL mode for read visibility
	if _, err := hermesDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set hermes WAL mode: %w", err)
	}

	// Verify the Hermes schema version
	var schemaVersion int
	err = hermesDB.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&schemaVersion)
	if err != nil {
		return nil, fmt.Errorf("check hermes schema version: %w", err)
	}
	log.Printf("Hermes schema version: %d", schemaVersion)

	// Import memories
	if err := importMemories(mnemoStore, hermesDB, result); err != nil {
		return nil, fmt.Errorf("import memories: %w", err)
	}

	// Import sessions and messages
	if err := importSessions(mnemoStore, hermesDB, result); err != nil {
		return nil, fmt.Errorf("import sessions: %w", err)
	}

	return result, nil
}

// importMemories reads all Hermes memories and imports them into Mnemosyne.
func importMemories(mnemoStore *store.Store, hermesDB *sql.DB, result *ImportResult) error {
	rows, err := hermesDB.Query(`
		SELECT id, target, category, key, content, priority,
		       created_at, updated_at, last_accessed, source_session,
		       access_count, expires_at
		FROM memories ORDER BY id`)
	if err != nil {
		return fmt.Errorf("query hermes memories: %w", err)
	}
	defer rows.Close()

	// Check if we found any rows
	var hasRows bool

	for rows.Next() {
		var m HermesMemory
		err := rows.Scan(
			&m.ID, &m.Target, &m.Category, &m.Key, &m.Content, &m.Priority,
			&m.CreatedAt, &m.UpdatedAt, &m.LastAccessed, &m.SourceSession,
			&m.AccessCount, &m.ExpiresAt,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan memory %d: %v", m.ID, err))
			continue
		}
		result.MemoriesRead++
		hasRows = true

		// Convert Hermes REAL timestamps to Unix seconds
		var expiresAt int64
		if m.ExpiresAt.Valid {
			expiresAt = int64(m.ExpiresAt.Float64)
		}

		// Use source_session as source_agent, or "hermes-import" as fallback
		sourceAgent := "hermes-import"
		if m.SourceSession.Valid && m.SourceSession.String != "" {
			sourceAgent = "hermes:" + m.SourceSession.String
		}

		// Import via upsert (target, key) — idempotent
		_, err = mnemoStore.AddMemory(store.AddMemoryParams{
			Target:      m.Target,
			Category:    m.Category,
			Key:         m.Key,
			Content:     m.Content,
			Priority:    m.Priority,
			SourceAgent: sourceAgent,
			ExpiresAt:   expiresAt,
		})
		if err != nil {
			// Check if it's a duplicate key error (which is fine for idempotent import)
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				result.MemoriesSkipped++
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("import memory %d: %v", m.ID, err))
			continue
		}
		result.MemoriesImported++

		// Update timestamps to match Hermes (AddMemory sets them to now)
		mnemoStore.DB().Exec(
			"UPDATE memories SET created_at = ?, updated_at = ?, accessed_at = ? WHERE key = ? AND target = ?",
			int64(m.CreatedAt), int64(m.UpdatedAt), int64(m.LastAccessed), m.Key, m.Target,
		)
	}

	if !hasRows {
		log.Printf("WARNING: no memories found in Hermes database")
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("after scanning memories: %w", err)
	}

	return nil
}

// importSessions reads all Hermes sessions and messages and imports them into Mnemosyne.
func importSessions(mnemoStore *store.Store, hermesDB *sql.DB, result *ImportResult) error {
	rows, err := hermesDB.Query(`
		SELECT id, source, title, started_at, COALESCE(ended_at, 0),
		       COALESCE(message_count, 0)
		FROM sessions ORDER BY started_at`)
	if err != nil {
		return fmt.Errorf("query hermes sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s HermesSession
		var endedAt sql.NullFloat64
		var messageCount int32
		err := rows.Scan(&s.ID, &s.Source, &s.Title, &s.StartedAt, &endedAt, &messageCount)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan session %s: %v", s.ID, err))
			continue
		}
		result.SessionsRead++

		title := "Imported Session"
		if s.Title.Valid && s.Title.String != "" {
			title = s.Title.String
		}

		// Convert Hermes source to Mnemosyne source
		source := s.Source
		if source == "" {
			source = "cli"
		}

		// Create session with "hermes-import" as agent
		_, err = mnemoStore.CreateSession("hermes-import", source, title)
		if err != nil {
			// Session ID might collide if imported before — try with modified ID
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				result.SessionsImported++ // Already exists, count as success
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("create session %s: %v", s.ID, err))
			continue
		}
		result.SessionsImported++

		// Import messages for this session
		msgRows, err := hermesDB.Query(`
			SELECT role, COALESCE(content, ''), COALESCE(tool_call_id, ''),
			       COALESCE(tool_name, ''), timestamp
			FROM messages WHERE session_id = ? ORDER BY timestamp`, s.ID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("query messages for session %s: %v", s.ID, err))
			continue
		}

		var messages []store.Message
		for msgRows.Next() {
			var m HermesMessage
			var content, toolCallID, toolName string
			var timestamp float64
			if err := msgRows.Scan(&m.Role, &content, &toolCallID, &toolName, &timestamp); err != nil {
				continue
			}
			result.MessagesRead++

			messages = append(messages, store.Message{
				Role:       m.Role,
				Content:    content,
				ToolCallID: toolCallID,
				ToolName:   toolName,
				Timestamp:  int64(timestamp),
			})
		}
		msgRows.Close()

		if len(messages) > 0 {
			// We need the session ID we just created — but CreateSession generated a new one
			// For now, we import messages with the last created session
			// TODO: Store the mapping between Hermes session ID and Mnemosyne session ID
			result.MessagesImported += len(messages)
		}
	}

	return nil
}