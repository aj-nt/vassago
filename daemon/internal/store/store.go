// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	// Pure-Go SQLite driver — no CGo required
	_ "modernc.org/sqlite"
)

// MemoryEntry represents a single memory record.
type MemoryEntry struct {
	ID          string
	Owner       string // namespace
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

// Validation and limit constants (Fowler: Replace Magic Number with Named Constant)
const (
	MaxContentLength = 100000 // 100KB max per memory content
	MaxKeyLength     = 500
	MaxCategoryLen   = 50
	MaxIDLength      = 256

	defaultResultLimit = 1000  // Default number of results for list/search queries
	maxResultLimit     = 10000 // Upper bound for paginated queries

	DefaultHotBlockCharLimit = 4000 // Default char limit for hot block computation
)

// ClampLimit clamps a limit value to a reasonable range.
// If limit <= 0, returns fallback.
// If max > 0 and limit > max, returns max.
// Otherwise returns limit unchanged.
func ClampLimit(limit, fallback, max int) int {
	if limit <= 0 {
		return fallback
	}
	if max > 0 && limit > max {
		return max
	}
	return limit
}

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

// Ping checks the database connection health.
func (s *Store) Ping() error {
	// Simple query to verify database connectivity
	var result int
	return s.db.QueryRow("SELECT 1").Scan(&result)
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
	if len(strings.TrimSpace(content)) == 0 {
		return fmt.Errorf("content must not be empty")
	}
	if priority < 1 || priority > 5 {
		return fmt.Errorf("invalid priority %d: must be 1-5", priority)
	}
	return nil
}

// --- Memory CRUD ---

// AddMemory is in add_memory.go (uses AddMemoryParams struct for readability).

// GetMemoryByID retrieves a memory by its ID.
func (s *Store) GetMemoryByID(id string) (*MemoryEntry, error) {
	entry := &MemoryEntry{}
	err := s.db.QueryRow(`
		SELECT id, owner, target, category, key, content, priority, source_agent,
		       created_at, updated_at, accessed_at, expires_at
		FROM memories WHERE id = ?`, id,
	).Scan(&entry.ID, &entry.Owner, &entry.Target, &entry.Category, &entry.Key, &entry.Content,
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
	s.db.Exec("UPDATE memories SET accessed_at = ? WHERE id = ?", time.Now().Unix(), id) //nolint:errcheck
	return entry, nil
}

// RemoveMemory deletes a memory by ID, scoped to owner (Phase 28).
func (s *Store) RemoveMemory(id, owner string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	result, err := s.db.Exec("DELETE FROM memories WHERE id = ? AND owner = ?", id, owner)
	if err != nil {
		return false, fmt.Errorf("remove memory: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ListMemories returns memories matching the given filters.
// Results are ordered by priority DESC, updated_at DESC.
// Uses a default limit of 1000.
func (s *Store) ListMemories(owner, target string, category string, minPriority int32) ([]*MemoryEntry, error) {
	return s.listMemories(owner, target, category, minPriority, 1000, 0, false)
}

// ListMemoriesPaginated returns memories with explicit pagination.
func (s *Store) ListMemoriesPaginated(owner, target string, category string, minPriority int32, limit int, offset int) ([]*MemoryEntry, error) {
	limit = ClampLimit(limit, defaultResultLimit, maxResultLimit)
	if offset < 0 {
		offset = 0
	}
	return s.listMemories(owner, target, category, minPriority, limit, offset, true)
}

// listMemories is the shared implementation for ListMemories and ListMemoriesPaginated.
func (s *Store) listMemories(owner, target string, category string, minPriority int32, limit int, offset int, paginate bool) ([]*MemoryEntry, error) {
	query := `
		SELECT id, owner, target, category, key, content, priority, source_agent,
		       created_at, updated_at, accessed_at, expires_at
		FROM memories WHERE owner = ?`
	args := []any{owner}

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
	if paginate {
		query += " OFFSET ?"
		args = append(args, offset)
	}

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
		if err := rows.Scan(&e.ID, &e.Owner, &e.Target, &e.Category, &e.Key, &e.Content,
			&e.Priority, &e.SourceAgent, &e.CreatedAt, &e.UpdatedAt,
			&e.AccessedAt, &e.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// --- FTS5 Search ---
// Search and sanitizeFTS5 are in search.go.