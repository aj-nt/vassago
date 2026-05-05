// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

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
	ID            int64
	Target        string
	Category      string
	Key           string
	Content       string
	Priority       int32
	CreatedAt     float64 // Hermes uses REAL timestamps (Unix epoch with fractional seconds)
	UpdatedAt     float64
	LastAccessed  float64
	SourceSession sql.NullString
	AccessCount   int32
	ExpiresAt     sql.NullFloat64 // NULL means no expiry
}

// HermesSession represents a session row in the Hermes state.db.
// Hermes v10+ schema includes many more columns than v9.
type HermesSession struct {
	ID              string
	Source          string
	UserID         sql.NullString
	Model          sql.NullString
	ModelConfig    sql.NullString
	SystemPrompt   sql.NullString
	ParentSessionID sql.NullString
	StartedAt      float64
	EndedAt        sql.NullFloat64
	EndReason      sql.NullString
	MessageCount   int32
	ToolCallCount  int32
	InputTokens    sql.NullInt64
	OutputTokens   sql.NullInt64
	CacheReadTokens sql.NullInt64
	CacheWriteTokens sql.NullInt64
	ReasoningTokens sql.NullInt64
	Title          sql.NullString
	APICallCount   sql.NullInt64
	BillingProvider sql.NullString
	BillingBaseURL sql.NullString
	BillingMode    sql.NullString
	EstimatedCost  sql.NullFloat64
	ActualCost     sql.NullFloat64
	CostStatus     sql.NullString
	CostSource     sql.NullString
	PricingVersion sql.NullString
}

// HermesMessage represents a message row in the Hermes state.db.
type HermesMessage struct {
	ID               int64
	SessionID       string
	Role             string
	Content          sql.NullString
	ToolCallID      sql.NullString
	ToolCalls       sql.NullString // JSON (Hermes v10+)
	ToolName        sql.NullString
	Timestamp       float64
	TokenCount      sql.NullInt64
	FinishReason    sql.NullString
	Reasoning       sql.NullString // JSON (Hermes v10+)
	ReasoningDetails sql.NullString // JSON (Hermes v10+)
	ReasoningContent sql.NullString // Hermes v10+
	CodexReasoningItems sql.NullString // JSON (Hermes v10+)
	CodexMessageItems   sql.NullString // JSON (Hermes v10+)
}

// ImportResult holds the results of an import operation.
type ImportResult struct {
	MemoriesRead     int
	MemoriesImported int
	MemoriesSkipped int // duplicates or invalid
	MemoriesUpdated  int // existing entries updated with new content
	SessionsRead     int
	SessionsImported int
	SessionsSkipped  int // already exist with same ID
	MessagesRead     int
	MessagesImported int
	MessagesSkipped  int // session not found or duplicate
	AgentsCreated    int
	Errors           []string
}

// ImportHermesDB reads a Hermes state.db and imports its data into Mnemosyne.
// It performs idempotent imports — running it twice will not create duplicates.
//
// Session IDs are preserved from Hermes so that message references remain intact.
// Timestamps are converted from Hermes REAL (Unix epoch with fractional seconds)
// to Mnemosyne INTEGER (Unix seconds, truncated).
//
// The agentID parameter is used as the agent_id for imported sessions and memories.
// If empty, defaults to "hermes-import".
func ImportHermesDB(mnemoStore *store.Store, hermesDBPath string, agentID string) (*ImportResult, error) {
	if agentID == "" {
		agentID = "hermes-import"
	}
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

	// Ensure the import agent exists
	agentCreated, err := ensureImportAgent(mnemoStore, agentID)
	if err != nil {
		return nil, fmt.Errorf("ensure import agent: %w", err)
	}
	if agentCreated {
		result.AgentsCreated++
	}

	// Import memories
	if err := importMemories(mnemoStore, hermesDB, agentID, result); err != nil {
		return nil, fmt.Errorf("import memories: %w", err)
	}

	// Import sessions and messages
	if err := importSessions(mnemoStore, hermesDB, agentID, result); err != nil {
		return nil, fmt.Errorf("import sessions: %w", err)
	}

	return result, nil
}

// ensureImportAgent creates the agent record for the import if it doesn't exist.
func ensureImportAgent(mnemoStore *store.Store, agentID string) (bool, error) {
	_, isNew, err := mnemoStore.EnsureAgent(agentID, "Hermes Import", "import")
	if err != nil {
		return false, fmt.Errorf("ensure agent: %w", err)
	}
	return isNew, nil
}

// importMemories reads all Hermes memories and imports them into Mnemosyne.
// Uses AddMemory for idempotent upsert (same target+key = update).
func importMemories(mnemoStore *store.Store, hermesDB *sql.DB, agentID string, result *ImportResult) error {
	rows, err := hermesDB.Query(`
		SELECT id, target, category, key, content, priority,
		       created_at, updated_at, last_accessed, source_session,
		       access_count, expires_at
		FROM memories ORDER BY id`)
	if err != nil {
		return fmt.Errorf("query hermes memories: %w", err)
	}
	defer rows.Close()

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

		// Use source_session to build source_agent, with fallback
		sourceAgent := agentID
		if m.SourceSession.Valid && m.SourceSession.String != "" {
			sourceAgent = "hermes:" + m.SourceSession.String
		}

		// Import via AddMemory's upsert (target, key) — idempotent
		entry, err := mnemoStore.AddMemory(store.AddMemoryParams{
			Target:      m.Target,
			Category:    m.Category,
			Key:         m.Key,
			Content:     m.Content,
			Priority:    m.Priority,
			SourceAgent:  sourceAgent,
			ExpiresAt:   expiresAt,
		})
		if err != nil {
			// Check if it's a duplicate key error (which shouldn't happen with upsert, but be safe)
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				result.MemoriesSkipped++
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("import memory %d (%s): %v", m.ID, m.Key, err))
			continue
		}

		// Determine if this was an insert or update
		if entry != nil {
			// Check timestamps — if created_at matches now, it was just inserted;
			// if it's a Unix timestamp from the past, it was updated.
			// Either way, fix the timestamps to match the original Hermes values.
			createdAt := int64(m.CreatedAt)
			updatedAt := int64(m.UpdatedAt)
			accessedAt := int64(m.LastAccessed)

			_, err = mnemoStore.DB().Exec(
				`UPDATE memories SET created_at = ?, updated_at = ?, accessed_at = ? WHERE id = ?`,
				createdAt, updatedAt, accessedAt, entry.ID,
			)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update timestamps for memory %s: %v", entry.ID, err))
			}
		}
		result.MemoriesImported++
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
// Session IDs are preserved from Hermes (prefixed with "hermes:") to avoid collisions.
// Messages are imported directly via SQL to bypass the store API's auto-generated IDs.
func importSessions(mnemoStore *store.Store, hermesDB *sql.DB, agentID string, result *ImportResult) error {
	// First, determine which Hermes session columns exist (schema v9 vs v10+)
	hermesCols, err := getHermesSessionColumns(hermesDB)
	if err != nil {
		return fmt.Errorf("detect hermes session columns: %w", err)
	}

	// Build the query based on available columns
	query := buildSessionQuery(hermesCols)

	rows, err := hermesDB.Query(query)
	if err != nil {
		return fmt.Errorf("query hermes sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s HermesSession
		var endedAt sql.NullFloat64
		var messageCount int32

		// Scan based on available columns
		if err := scanSessionRow(rows, hermesCols, &s, &endedAt, &messageCount); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan session: %v", err))
			continue
		}
		result.SessionsRead++

		title := "Imported from Hermes"
		if s.Title.Valid && s.Title.String != "" {
			title = s.Title.String
		}

		// Convert Hermes session ID to Mnemosyne format
		// Prefix with "hermes:" to avoid ID collisions with Mnemosyne-generated UUIDs
		mnemoSessionID := "hermes:" + s.ID

		// Convert timestamps
		createdAt := int64(s.StartedAt)
		lastActiveAt := createdAt
		if endedAt.Valid && endedAt.Float64 > 0 {
			lastActiveAt = int64(endedAt.Float64)
		}
		endedAtInt := int64(0)
		if endedAt.Valid && endedAt.Float64 > 0 {
			endedAtInt = int64(endedAt.Float64)
		}

		source := s.Source
		if source == "" {
			source = "cli"
		}

		// Insert session directly (bypass CreateSession which generates a new UUID)
		res, err := mnemoStore.DB().Exec(`
			INSERT OR IGNORE INTO sessions (id, agent_id, source, title, created_at, last_active_at, message_count, ended_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			mnemoSessionID, agentID, source, title, createdAt, lastActiveAt, messageCount, endedAtInt,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("create session %s: %v", s.ID, err))
			continue
		}
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			// Session already exists (INSERT OR IGNORE)
			result.SessionsSkipped++
		} else {
			result.SessionsImported++
		}

		// Import messages for this session
		if err := importSessionMessages(mnemoStore, hermesDB, mnemoSessionID, s.ID, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("import messages for session %s: %v", s.ID, err))
			continue
		}
	}

	return nil
}

// importSessionMessages imports all messages for a single session from Hermes.
func importSessionMessages(mnemoStore *store.Store, hermesDB *sql.DB, mnemoSessionID, hermesSessionID string, result *ImportResult) error {
	// Determine which message columns exist
	msgCols, err := getHermesMessageColumns(hermesDB)
	if err != nil {
		return fmt.Errorf("detect hermes message columns: %w", err)
	}

	query := buildMessageQuery(msgCols)
	msgRows, err := hermesDB.Query(query, hermesSessionID)
	if err != nil {
		return fmt.Errorf("query messages: %w", err)
	}
	defer msgRows.Close()

	batchSize := 0
	tx, err := mnemoStore.DB().Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for msgRows.Next() {
		var m HermesMessage
		if err := scanMessageRow(msgRows, msgCols, &m); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan message: %v", err))
			continue
		}
		result.MessagesRead++

		role := m.Role
		content := ""
		if m.Content.Valid {
			content = m.Content.String
		}
		toolCallID := ""
		if m.ToolCallID.Valid {
			toolCallID = m.ToolCallID.String
		}
		toolName := ""
		if m.ToolName.Valid {
			toolName = m.ToolName.String
		}
		timestamp := int64(m.Timestamp)

		// Sanitize content for valid UTF-8 (Mnemosyne requires valid UTF-8 in protobuf)
		content = sanitizeContent(content)
		toolCallID = sanitizeContent(toolCallID)
		toolName = sanitizeContent(toolName)

		_, err := tx.Exec(`
			INSERT INTO messages (session_id, role, content, tool_call_id, tool_name, timestamp)
			VALUES (?, ?, ?, ?, ?, ?)`,
			mnemoSessionID, role, content, toolCallID, toolName, timestamp,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("insert message: %v", err))
			continue
		}
		result.MessagesImported++
		batchSize++

		// Commit in batches of 500 to avoid huge transactions
		if batchSize >= 500 {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit message batch: %w", err)
			}
			tx, err = mnemoStore.DB().Begin()
			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}
			batchSize = 0
		}
	}

	if batchSize > 0 {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit final message batch: %w", err)
		}
	}

	return nil
}

// sanitizeContent replaces invalid UTF-8 bytes with the Unicode replacement character.
// This is needed because some Hermes message data may contain invalid UTF-8 sequences
// that would cause errors in gRPC/protobuf serialization.
func sanitizeContent(s string) string {
	if len(s) == 0 {
		return s
	}
	// Fast path: check if valid UTF-8
	valid := true
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			// ASCII — always valid
			i++
		} else if s[i]&0xE0 == 0xC0 {
			if i+1 >= len(s) || s[i+1]&0xC0 != 0x80 {
				valid = false
				break
			}
			i += 2
		} else if s[i]&0xF0 == 0xE0 {
			if i+2 >= len(s) || s[i+1]&0xC0 != 0x80 || s[i+2]&0xC0 != 0x80 {
				valid = false
				break
			}
			i += 3
		} else if s[i]&0xF8 == 0xF0 {
			if i+3 >= len(s) || s[i+1]&0xC0 != 0x80 || s[i+2]&0xC0 != 0x80 || s[i+3]&0xC0 != 0x80 {
				valid = false
				break
			}
			i += 4
		} else {
			valid = false
			break
		}
	}
	if valid {
		return s
	}

	// Slow path: replace invalid bytes
	result := make([]byte, 0, len(s)+len(s)/10)
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			result = append(result, s[i])
			i++
		} else if s[i]&0xE0 == 0xC0 && i+1 < len(s) && s[i+1]&0xC0 == 0x80 {
			result = append(result, s[i], s[i+1])
			i += 2
		} else if s[i]&0xF0 == 0xE0 && i+2 < len(s) && s[i+1]&0xC0 == 0x80 && s[i+2]&0xC0 == 0x80 {
			result = append(result, s[i], s[i+1], s[i+2])
			i += 3
		} else if s[i]&0xF8 == 0xF0 && i+3 < len(s) && s[i+1]&0xC0 == 0x80 && s[i+2]&0xC0 == 0x80 && s[i+3]&0xC0 == 0x80 {
			result = append(result, s[i], s[i+1], s[i+2], s[i+3])
			i += 4
		} else {
			result = append(result, "\xef\xbf\xbd"...) // U+FFFD
			i++
		}
	}
	return string(result)
}

// --- Schema detection helpers ---

// getHermesSessionColumns returns the set of column names in the Hermes sessions table.
func getHermesSessionColumns(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		return nil, fmt.Errorf("get session columns: %w", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		cols[name] = true
	}
	return cols, nil
}

// buildSessionQuery constructs the SELECT query for sessions based on available columns.
func buildSessionQuery(cols map[string]bool) string {
	// Required columns (always present)
	selectCols := []string{"id", "source", "started_at"}

	// Optional columns with defaults
	if cols["title"] {
		selectCols = append(selectCols, "title")
	} else {
		selectCols = append(selectCols, "'' as title")
	}
	if cols["ended_at"] {
		selectCols = append(selectCols, "ended_at")
	} else {
		selectCols = append(selectCols, "0 as ended_at")
	}
	if cols["message_count"] {
		selectCols = append(selectCols, "message_count")
	} else {
		selectCols = append(selectCols, "0 as message_count")
	}

	return "SELECT " + strings.Join(selectCols, ", ") + " FROM sessions ORDER BY started_at"
}

// scanSessionRow scans a session row based on available columns.
func scanSessionRow(rows *sql.Rows, cols map[string]bool, s *HermesSession, endedAt *sql.NullFloat64, messageCount *int32) error {
	// Always: id, source, started_at
	// Then: title (nullable), ended_at (nullable), message_count
	var title sql.NullString

	err := rows.Scan(&s.ID, &s.Source, &s.StartedAt, &title, endedAt, messageCount)
	s.Title = title
	return err
}

// getHermesMessageColumns returns the set of column names in the Hermes messages table.
func getHermesMessageColumns(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(messages)")
	if err != nil {
		return nil, fmt.Errorf("get message columns: %w", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		cols[name] = true
	}
	return cols, nil
}

// buildMessageQuery constructs the SELECT query for messages based on available columns.
func buildMessageQuery(cols map[string]bool) string {
	// Core columns (always present in v9+)
	selectCols := []string{"role"}

	// content — always present but nullable in Hermes
	if cols["content"] {
		selectCols = append(selectCols, "content")
	} else {
		selectCols = append(selectCols, "'' as content")
	}

	// tool_call_id — nullable
	if cols["tool_call_id"] {
		selectCols = append(selectCols, "tool_call_id")
	} else {
		selectCols = append(selectCols, "'' as tool_call_id")
	}

	// tool_name — nullable
	if cols["tool_name"] {
		selectCols = append(selectCols, "tool_name")
	} else {
		selectCols = append(selectCols, "'' as tool_name")
	}

	// timestamp — always present
	selectCols = append(selectCols, "timestamp")

	return "SELECT " + strings.Join(selectCols, ", ") + " FROM messages WHERE session_id = ? ORDER BY timestamp"
}

// scanMessageRow scans a message row based on available columns.
func scanMessageRow(rows *sql.Rows, cols map[string]bool, m *HermesMessage) error {
	var content sql.NullString
	var toolCallID sql.NullString
	var toolName sql.NullString
	var timestamp float64

	err := rows.Scan(&m.Role, &content, &toolCallID, &toolName, &timestamp)
	m.Content = content
	m.ToolCallID = toolCallID
	m.ToolName = toolName
	m.Timestamp = timestamp
	return err
}