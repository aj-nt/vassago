// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package migration

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/aj-nt/mnemosyne/daemon/internal/store"
)

func TestImportHermesDB_Empty(t *testing.T) {
	// Create a minimal Hermes state.db
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	// Create Mnemosyne store
	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath)
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}
	if result.MemoriesRead != 0 {
		t.Errorf("expected 0 memories read, got %d", result.MemoriesRead)
	}
	if result.SessionsRead != 0 {
		t.Errorf("expected 0 sessions read, got %d", result.SessionsRead)
	}
}

func TestImportHermesDB_Memories(t *testing.T) {
	// Create a single temp directory that both test helper and import use
	tmpDir := t.TempDir()
	hermesPath := filepath.Join(tmpDir, "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesMemory(t, hermesPath, "memory", "environment", "test-key-1", "test content 1", 3, 0)
	insertHermesMemory(t, hermesPath, "memory", "project", "test-key-2", "test content 2", 2, 0)
	insertHermesMemory(t, hermesPath, "user", "user", "user-name", "AJ", 4, 0)

	// Verify the Hermes DB has the data before ImportHermesDB
	hDB, err := sql.Open("sqlite", hermesPath)
	if err != nil {
		t.Fatalf("open hermes db for verification: %v", err)
	}
	var memCount int
	if err := hDB.QueryRow("SELECT COUNT(*) FROM memories").Scan(&memCount); err != nil {
		t.Fatalf("scan memory count: %v", err)
	}
	if memCount != 3 {
		t.Fatalf("expected 3 memories in Hermes, got %d", memCount)
	}
	hDB.Close() // Close before ImportHermesDB opens its own connection

	// Create Mnemosyne store in the SAME temp directory
	mnemoPath := filepath.Join(tmpDir, "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath)
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}
	if result.MemoriesImported != 3 {
		t.Errorf("expected 3 memories imported, got %d", result.MemoriesImported)
	}

	// Verify the memories are in Mnemosyne
	entries, err := mnemoStore.ListMemories("memory", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 memory entries, got %d", len(entries))
	}

	userEntries, err := mnemoStore.ListMemories("user", "", 0)
	if err != nil {
		t.Fatalf("list user entries: %v", err)
	}
	if len(userEntries) < 1 {
		t.Errorf("expected at least 1 user entry, got %d", len(userEntries))
	}
}

func TestImportHermesDB_Idempotent(t *testing.T) {
	// Create a Hermes state.db with test data
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesMemory(t, hermesPath, "memory", "environment", "dup-key", "content", 3, 0)

	// Create Mnemosyne store
	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	// Import once
	result1, err := ImportHermesDB(mnemoStore, hermesPath)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if result1.MemoriesImported != 1 {
		t.Errorf("expected 1 memory imported, got %d", result1.MemoriesImported)
	}

	// Import again — should succeed via upsert (no duplicates created)
	result2, err := ImportHermesDB(mnemoStore, hermesPath)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	// Second import upserts the same entry, so MemoriesImported should be 1
	// (AddMemory upserts on target+key match)
	if result2.MemoriesImported != 1 {
		t.Errorf("expected 1 memory on re-import (upsert), got %d", result2.MemoriesImported)
	}

	// Verify only 1 memory exists (deduplicated by key)
	entries, err := mnemoStore.ListMemories("memory", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 memory after idempotent import, got %d", len(entries))
	}
}

func TestImportHermesDB_ExpiredMemories(t *testing.T) {
	// Create a Hermes state.db with expired and non-expired memories
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesMemory(t, hermesPath, "memory", "observation", "expired-key", "old content", 2, 1000)       // Expired
	insertHermesMemory(t, hermesPath, "memory", "environment", "current-key", "current content", 3, 9999999999) // Not expired

	// Create Mnemosyne store and import
	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath)
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}
	if result.MemoriesImported != 2 {
		t.Errorf("expected 2 memories imported, got %d", result.MemoriesImported)
	}

	// Run expiry — should remove the expired one
	removed, err := mnemoStore.ExpireStaleMemories()
	if err != nil {
		t.Fatalf("expire stale memories: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 expired memory removed, got %d", removed)
	}
}

// Helper: create a minimal Hermes state.db
func createHermesTestDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	// Enable WAL mode and checkpoint for visibility
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}

	// Create Hermes schema (simplified — just the tables we need)
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version VALUES (9)`,
		`CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target TEXT NOT NULL CHECK(target IN ('memory', 'user')),
			category TEXT NOT NULL CHECK(category IN ('user', 'environment', 'quirk', 'project', 'observation')),
			key TEXT NOT NULL,
			content TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
			created_at REAL NOT NULL DEFAULT (strftime('%s','now')),
			updated_at REAL NOT NULL DEFAULT (strftime('%s','now')),
			last_accessed REAL NOT NULL DEFAULT (strftime('%s','now')),
			source_session TEXT,
			access_count INTEGER NOT NULL DEFAULT 0,
			expires_at REAL,
			UNIQUE(target, key)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			title TEXT,
			started_at REAL NOT NULL,
			ended_at REAL,
			message_count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			role TEXT NOT NULL,
			content TEXT,
			tool_call_id TEXT,
			tool_name TEXT,
			timestamp REAL NOT NULL
		)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// Helper: insert a test memory into Hermes DB
func insertHermesMemory(t *testing.T, dbPath, target, category, key, content string, priority int32, expiresAt float64) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	defer db.Close()

	// Enable WAL mode for write visibility
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL mode: %v", err)
	}

	var expiresVal interface{}
	if expiresAt > 0 {
		expiresVal = expiresAt
	}

	res, err := db.Exec(`
		INSERT INTO memories (target, category, key, content, priority, created_at, updated_at, last_accessed, expires_at)
		VALUES (?, ?, ?, ?, ?, strftime('%s','now'), strftime('%s','now'), strftime('%s','now'), ?)`,
		target, category, key, content, priority, expiresVal)
	if err != nil {
		t.Fatalf("insert hermes memory: %v", err)
	}
	rows, _ := res.RowsAffected()
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}

	// Verify the insert worked
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count); err != nil {
		t.Fatalf("scan count after insert: %v", err)
	}
	if count == 0 {
		t.Errorf("no memories found after insert into %s", dbPath)
	}

	// Checkpoint WAL so subsequent readers can see the data
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}