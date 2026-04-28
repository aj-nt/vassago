package store

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	// Pure-Go SQLite driver — no CGo required
	_ "modernc.org/sqlite"

	"github.com/google/uuid"
)

// MemoryEntry represents a single memory record.
type MemoryEntry struct {
	ID          string
	Target      string // "memory" or "user"
	Category    string
	Key         string
	Content     string
	Priority    int32
	SourceAgent string
	CreatedAt   int64 // Unix timestamp
	UpdatedAt   int64
	AccessedAt  int64
	ExpiresAt   int64 // 0 = no expiry
}

// Validation constants
const (
	MaxContentLength = 100000 // 100KB max per memory content
	MaxKeyLength     = 500
	MaxCategoryLen   = 50
	MaxIDLength      = 256
)

// Store is the SQLite-backed data store for Mnemosyne.
type Store struct {
	db    *sql.DB
	writeMu sync.Mutex // Serializes write operations for SQLite BUSY avoidance
}

// New creates a new Store and initializes the database.
// If the database doesn't exist, it creates it with the current schema.
// If it exists, it applies any pending migrations.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	// Set busy timeout
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	s := &Store{db: db}
	if err := s.initDB(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection (for testing and CLI access).
func (s *Store) DB() *sql.DB {
	return s.db
}

// initDB creates the schema if it doesn't exist, or applies migrations.
func (s *Store) initDB() error {
	// Check if schema_version table exists
	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		// Table doesn't exist yet — create fresh schema
		for _, stmt := range SchemaStmts {
			if _, err := s.db.Exec(stmt); err != nil {
				truncated := stmt
				if len(truncated) > 80 {
					truncated = truncated[:80] + "..."
				}
				return fmt.Errorf("create schema: %w (statement: %q)", err, truncated)
			}
		}
		// Record schema version
		if _, err := s.db.Exec(
			"INSERT OR REPLACE INTO schema_version (version, applied_at) VALUES (?, ?)",
			SchemaVersion, time.Now().Unix(),
		); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
		return nil
	}

	// Apply pending migrations
	if version < SchemaVersion {
		for i := version; i < SchemaVersion; i++ {
			idx := i - 1 // 0-indexed
			if idx < len(Migrations) {
				if _, err := s.db.Exec(Migrations[idx]); err != nil {
					return fmt.Errorf("apply migration %d: %w", i, err)
				}
			}
			if _, err := s.db.Exec(
				"INSERT OR REPLACE INTO schema_version (version, applied_at) VALUES (?, ?)",
				i+1, time.Now().Unix(),
			); err != nil {
				return fmt.Errorf("record migration %d: %w", i+1, err)
			}
		}
	}

	return nil
}

// validateMemoryInput validates common fields for AddMemory and UpdateMemory.
func validateMemoryInput(target, category, key, content string, priority int32) error {
	if target != "memory" && target != "user" {
		return fmt.Errorf("invalid target %q: must be 'memory' or 'user'", target)
	}
	if category == "" {
		return fmt.Errorf("category must not be empty")
	}
	if len(category) > MaxCategoryLen {
		return fmt.Errorf("category too long: %d chars, max %d", len(category), MaxCategoryLen)
	}
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if len(key) > MaxKeyLength {
		return fmt.Errorf("key too long: %d chars, max %d", len(key), MaxKeyLength)
	}
	if len(content) > MaxContentLength {
		return fmt.Errorf("content too long: %d chars, max %d", len(content), MaxContentLength)
	}
	if priority < 1 || priority > 5 {
		return fmt.Errorf("invalid priority %d: must be 1-5", priority)
	}
	return nil
}

// --- Memory CRUD ---

// AddMemory creates a new memory entry. If target+key already exists, it updates instead.
// The operation is atomic — the check-then-upsert is wrapped in a transaction.
func (s *Store) AddMemory(target, category, key, content string, priority int32, sourceAgent string, expiresAt int64) (*MemoryEntry, error) {
	if err := validateMemoryInput(target, category, key, content, priority); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	if sourceAgent == "" {
		return nil, fmt.Errorf("source_agent must not be empty")
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()

	// Transaction for atomic upsert
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if entry exists with this target+key
	var existingID string
	err = tx.QueryRow("SELECT id FROM memories WHERE target = ? AND key = ?", target, key).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("check existing: %w", err)
	}

	if existingID != "" {
		// Update existing entry
		_, err = tx.Exec(`
			UPDATE memories SET
				content = ?,
				priority = ?,
				category = ?,
				source_agent = ?,
				updated_at = ?,
				expires_at = ?
			WHERE id = ?`,
			content, priority, category, sourceAgent, now, expiresAt, existingID,
		)
		if err != nil {
			return nil, fmt.Errorf("update memory: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit update: %w", err)
		}
		return s.GetMemoryByID(existingID)
	}

	// Create new entry
	id := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO memories (id, target, category, key, content, priority, source_agent, created_at, updated_at, accessed_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, target, category, key, content, priority, sourceAgent, now, now, now, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert memory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit insert: %w", err)
	}
	return s.GetMemoryByID(id)
}

// GetMemoryByID retrieves a memory by its ID.
func (s *Store) GetMemoryByID(id string) (*MemoryEntry, error) {
	entry := &MemoryEntry{}
	err := s.db.QueryRow(`
		SELECT id, target, category, key, content, priority, source_agent,
		       created_at, updated_at, accessed_at, expires_at
		FROM memories WHERE id = ?`, id,
	).Scan(&entry.ID, &entry.Target, &entry.Category, &entry.Key, &entry.Content,
		&entry.Priority, &entry.SourceAgent, &entry.CreatedAt, &entry.UpdatedAt,
		&entry.AccessedAt, &entry.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("memory not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get memory: %w", err)
	}

	// Update accessed_at (side effect of read — consider making this explicit)
	s.db.Exec("UPDATE memories SET accessed_at = ? WHERE id = ?", time.Now().Unix(), id)
	return entry, nil
}

// RemoveMemory deletes a memory by ID.
func (s *Store) RemoveMemory(id string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("remove memory: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ListMemories returns memories matching the given filters.
// Results are ordered by priority DESC, updated_at DESC.
// Use limit to paginate; 0 means use default (1000).
func (s *Store) ListMemories(target string, category string, minPriority int32) ([]*MemoryEntry, error) {
	limit := 1000
	return s.listMemories(target, category, minPriority, limit)
}

// ListMemoriesPaginated returns memories with explicit pagination.
func (s *Store) ListMemoriesPaginated(target string, category string, minPriority int32, limit int, offset int) ([]*MemoryEntry, error) {
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, target, category, key, content, priority, source_agent,
		       created_at, updated_at, accessed_at, expires_at
		FROM memories WHERE 1=1`
	args := []any{}

	if target != "" {
		query += " AND target = ?"
		args = append(args, target)
	}
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if minPriority > 0 {
		query += " AND priority >= ?"
		args = append(args, minPriority)
	}
	query += " ORDER BY priority DESC, updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

func (s *Store) listMemories(target string, category string, minPriority int32, limit int) ([]*MemoryEntry, error) {
	query := `
		SELECT id, target, category, key, content, priority, source_agent,
		       created_at, updated_at, accessed_at, expires_at
		FROM memories WHERE 1=1`
	args := []any{}

	if target != "" {
		query += " AND target = ?"
		args = append(args, target)
	}
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if minPriority > 0 {
		query += " AND priority >= ?"
		args = append(args, minPriority)
	}
	query += " ORDER BY priority DESC, updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	return scanMemories(rows)
}

func scanMemories(rows *sql.Rows) ([]*MemoryEntry, error) {
	var entries []*MemoryEntry
	for rows.Next() {
		e := &MemoryEntry{}
		if err := rows.Scan(&e.ID, &e.Target, &e.Category, &e.Key, &e.Content,
			&e.Priority, &e.SourceAgent, &e.CreatedAt, &e.UpdatedAt,
			&e.AccessedAt, &e.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// --- FTS5 Search ---

// Search performs a full-text search using FTS5.
// Multi-word queries use OR semantics by default.
// Special tokens (colons, dots) are quoted to prevent FTS5 column filter errors.
func (s *Store) Search(query string, target string, limit int) ([]*MemoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	// Sanitize the FTS5 query
	sanitized := s.sanitizeFTS5(query)

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
func (s *Store) sanitizeFTS5(query string) string {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}