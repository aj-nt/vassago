// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package migration

import (
	"database/sql"
	"fmt"
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

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}
	if result.MemoriesRead != 0 {
		t.Errorf("expected 0 memories read, got %d", result.MemoriesRead)
	}
	if result.SessionsRead != 0 {
		t.Errorf("expected 0 sessions read, got %d", result.SessionsRead)
	}
	if result.AgentsCreated != 1 {
		t.Errorf("expected 1 agent created, got %d", result.AgentsCreated)
	}
}

func TestImportHermesDB_Memories(t *testing.T) {
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
	hDB.Close()

	mnemoPath := filepath.Join(tmpDir, "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}
	if result.MemoriesImported != 3 {
		t.Errorf("expected 3 memories imported, got %d", result.MemoriesImported)
	}

	// Verify the memories are in Mnemosyne
	entries, err := mnemoStore.ListMemories("default", "memory", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 memory entries, got %d", len(entries))
	}

	userEntries, err := mnemoStore.ListMemories("default", "user", "", 0)
	if err != nil {
		t.Fatalf("list user entries: %v", err)
	}
	if len(userEntries) < 1 {
		t.Errorf("expected at least 1 user entry, got %d", len(userEntries))
	}
}

func TestImportHermesDB_Idempotent(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesMemory(t, hermesPath, "memory", "environment", "dup-key", "content", 3, 0)

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	// Import once
	result1, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if result1.MemoriesImported != 1 {
		t.Errorf("expected 1 memory imported on first run, got %d", result1.MemoriesImported)
	}

	// Import again — should upsert the same entry (no duplicates created)
	result2, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	// AddMemory upserts on target+key, so Imported count is still 1
	if result2.MemoriesImported != 1 {
		t.Errorf("expected 1 memory on re-import (upsert), got %d", result2.MemoriesImported)
	}

	// Verify only 1 memory exists (deduplicated by key)
	entries, err := mnemoStore.ListMemories("default", "memory", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 memory after idempotent import, got %d", len(entries))
	}
}

func TestImportHermesDB_ExpiredMemories(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesMemory(t, hermesPath, "memory", "observation", "expired-key", "old content", 2, 1000)           // Expired
	insertHermesMemory(t, hermesPath, "memory", "environment", "current-key", "current content", 3, 9999999999) // Not expired

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}
	if result.MemoriesImported != 2 {
		t.Errorf("expected 2 memories imported, got %d", result.MemoriesImported)
	}

	// Run expiry — should remove the expired one
	removed, err := mnemoStore.ExpireStaleMemories("default")
	if err != nil {
		t.Fatalf("expire stale memories: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 expired memory removed, got %d", removed)
	}
}

func TestImportHermesDB_SessionsAndMessages(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	// Insert a session
	insertHermesSession(t, hermesPath, "sess-001", "cli", "Test Session", 1700000000.0, 0, 2)
	insertHermesMessage(t, hermesPath, "sess-001", "user", "Hello", 1700000001.0)
	insertHermesMessage(t, hermesPath, "sess-001", "assistant", "Hi there!", 1700000002.0)

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}

	if result.SessionsImported != 1 {
		t.Errorf("expected 1 session imported, got %d (skipped=%d)", result.SessionsImported, result.SessionsSkipped)
	}
	if result.MessagesImported != 2 {
		t.Errorf("expected 2 messages imported, got %d", result.MessagesImported)
	}

	// Verify session exists with hermes: prefix
	sess, err := mnemoStore.GetSession("hermes:sess-001")
	if err != nil {
		t.Fatalf("get imported session: %v", err)
	}
	if sess.Title != "Test Session" {
		t.Errorf("expected title 'Test Session', got %q", sess.Title)
	}
	if sess.AgentID != "test-agent" {
		t.Errorf("expected agent_id 'test-agent', got %q", sess.AgentID)
	}

	// Verify messages exist
	msgs, err := mnemoStore.GetSessionMessages("hermes:sess-001")
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("expected first message 'user: Hello', got %q: %q", msgs[0].Role, msgs[0].Content)
	}
}

func TestImportHermesDB_SessionIDPreservation(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	// Insert a session with a specific Hermes-format ID
	insertHermesSession(t, hermesPath, "20260504_193252_f416d7", "cli", "Memory cleanup", 1700000000.0, 1700001000.0, 5)

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}

	if result.SessionsImported != 1 {
		t.Errorf("expected 1 session imported, got %d", result.SessionsImported)
	}

	// Session should be findable with hermes: prefix
	sess, err := mnemoStore.GetSession("hermes:20260504_193252_f416d7")
	if err != nil {
		t.Fatalf("get session by hermes-prefixed ID: %v", err)
	}
	if sess.AgentID != "test-agent" {
		t.Errorf("expected agent_id 'test-agent', got %q", sess.AgentID)
	}
}

func TestImportHermesDB_DefaultAgent(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesMemory(t, hermesPath, "memory", "environment", "default-agent-test", "content", 3, 0)

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	// Empty agentID should default to "hermes-import"
	result, err := ImportHermesDB(mnemoStore, hermesPath, "")
	if err != nil {
		t.Fatalf("ImportHermesDB with empty agent: %v", err)
	}
	if result.MemoriesImported != 1 {
		t.Errorf("expected 1 memory imported, got %d", result.MemoriesImported)
	}
	if result.AgentsCreated != 1 {
		t.Errorf("expected 1 agent created, got %d", result.AgentsCreated)
	}

	// Verify the memory has "hermes-import" as source_agent
	entries, err := mnemoStore.ListMemories("default", "memory", "", 0)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no memories found")
	}
	if entries[0].SourceAgent != "hermes-import" {
		t.Errorf("expected source_agent 'hermes-import', got %q", entries[0].SourceAgent)
	}
}

func TestImportHermesDB_UTF8Sanitization(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	// Insert a session with a message containing invalid UTF-8
	insertHermesSession(t, hermesPath, "sess-utf8", "cli", "UTF-8 Test", 1700000000.0, 0, 1)
	// Insert message directly with raw SQL to include invalid bytes
	hDB, err := sql.Open("sqlite", hermesPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	_, err = hDB.Exec(`INSERT INTO messages (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
		"sess-utf8", "user", "hello\x80world\xff", 1700000001.0)
	hDB.Close()
	if err != nil {
		t.Fatalf("insert invalid utf8 message: %v", err)
	}

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}

	if result.MessagesImported != 1 {
		t.Errorf("expected 1 message imported, got %d", result.MessagesImported)
	}

	// Verify the message content was sanitized
	msgs, err := mnemoStore.GetSessionMessages("hermes:sess-utf8")
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	// Invalid bytes should be replaced with U+FFFD
	if msgs[0].Content != "hello\uFFFDworld\uFFFD" {
		t.Errorf("expected sanitized content, got %q", msgs[0].Content)
	}
}

func TestImportHermesDB_LargeSessionBatch(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	// Insert a session with many messages to test batch commits
	insertHermesSession(t, hermesPath, "sess-batch", "cli", "Batch Test", 1700000000.0, 1700001000.0, 600)
	hDB, err := sql.Open("sqlite", hermesPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	for i := 0; i < 600; i++ {
		ts := 1700000001.0 + float64(i)
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		_, err := hDB.Exec(`INSERT INTO messages (session_id, role, content, timestamp) VALUES (?, ?, ?, ?)`,
			"sess-batch", role, fmt.Sprintf("Message %d", i), ts)
		if err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}
	hDB.Close()

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB: %v", err)
	}

	if result.SessionsImported != 1 {
		t.Errorf("expected 1 session imported, got %d", result.SessionsImported)
	}
	if result.MessagesImported != 600 {
		t.Errorf("expected 600 messages imported, got %d", result.MessagesImported)
	}

	msgs, err := mnemoStore.GetSessionMessages("hermes:sess-batch")
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	if len(msgs) != 600 {
		t.Errorf("expected 600 messages in store, got %d", len(msgs))
	}
}

func TestImportHermesDB_SessionIdempotent(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}
	insertHermesSession(t, hermesPath, "sess-idem", "cli", "Idempotent Test", 1700000000.0, 0, 1)
	insertHermesMessage(t, hermesPath, "sess-idem", "user", "Hello", 1700000001.0)

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	// First import
	result1, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if result1.SessionsImported != 1 {
		t.Errorf("expected 1 session on first import, got %d", result1.SessionsImported)
	}
	if result1.MessagesImported != 1 {
		t.Errorf("expected 1 message on first import, got %d", result1.MessagesImported)
	}

	// Second import — session should be skipped (OR IGNORE), but messages may be re-inserted
	// since we don't check for duplicate messages (same content can appear multiple times)
	result2, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Session should be skipped on re-import
	if result2.SessionsImported != 0 {
		t.Errorf("expected 0 sessions imported on second run, got %d (skipped=%d)", result2.SessionsImported, result2.SessionsSkipped)
	}

	// Verify only 1 session exists
	sess, err := mnemoStore.GetSession("hermes:sess-idem")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess == nil {
		t.Fatal("session should still exist after re-import")
	}
}

func TestImportHermesDB_SchemaV10(t *testing.T) {
	// Test with v10+ schema (extended columns)
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesV10TestDB(hermesPath); err != nil {
		t.Fatalf("create hermes v10 db: %v", err)
	}
	insertHermesV10Session(t, hermesPath, "sess-v10", "cli", "V10 Session", 1700000000.0, 1700001000.0, 10, "gpt-4")
	insertHermesMessage(t, hermesPath, "sess-v10", "user", "V10 test message", 1700000001.0)

	mnemoPath := filepath.Join(t.TempDir(), "mnemosyne.db")
	mnemoStore, err := store.New(mnemoPath)
	if err != nil {
		t.Fatalf("create mnemosyne store: %v", err)
	}
	defer mnemoStore.Close()

	result, err := ImportHermesDB(mnemoStore, hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("ImportHermesDB with v10 schema: %v", err)
	}

	if result.SessionsImported != 1 {
		t.Errorf("expected 1 session imported from v10, got %d", result.SessionsImported)
	}
	if result.MessagesImported != 1 {
		t.Errorf("expected 1 message imported from v10, got %d", result.MessagesImported)
	}

	// Verify session and message survived
	sess, err := mnemoStore.GetSession("hermes:sess-v10")
	if err != nil {
		t.Fatalf("get v10 session: %v", err)
	}
	if sess.Title != "V10 Session" {
		t.Errorf("expected title 'V10 Session', got %q", sess.Title)
	}
}

// --- Test helpers ---

func createHermesTestDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}

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

// createHermesV10TestDB creates a Hermes DB with v10+ schema (extra columns).
func createHermesV10TestDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return err
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version VALUES (10)`,
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
			user_id TEXT,
			model TEXT,
			model_config TEXT,
			system_prompt TEXT,
			parent_session_id TEXT,
			started_at REAL NOT NULL,
			ended_at REAL,
			end_reason TEXT,
			message_count INTEGER DEFAULT 0,
			tool_call_count INTEGER DEFAULT 0,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			title TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL REFERENCES sessions(id),
			role TEXT NOT NULL,
			content TEXT,
			tool_call_id TEXT,
			tool_calls TEXT,
			tool_name TEXT,
			timestamp REAL NOT NULL,
			token_count INTEGER,
			finish_reason TEXT,
			reasoning TEXT
		)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func insertHermesMemory(t *testing.T, dbPath, target, category, key, content string, priority int32, expiresAt float64) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	defer db.Close()

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

	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

func insertHermesSession(t *testing.T, dbPath, id, source, title string, startedAt, endedAt float64, messageCount int32) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL mode: %v", err)
	}

	var endedAtVal interface{}
	if endedAt > 0 {
		endedAtVal = endedAt
	}

	_, err = db.Exec(`
		INSERT INTO sessions (id, source, title, started_at, ended_at, message_count)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, source, title, startedAt, endedAtVal, messageCount)
	if err != nil {
		t.Fatalf("insert hermes session: %v", err)
	}
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

func insertHermesV10Session(t *testing.T, dbPath, id, source, title string, startedAt, endedAt float64, messageCount int32, model string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL mode: %v", err)
	}

	var endedAtVal interface{}
	if endedAt > 0 {
		endedAtVal = endedAt
	}

	_, err = db.Exec(`
		INSERT INTO sessions (id, source, title, started_at, ended_at, message_count, model)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, source, title, startedAt, endedAtVal, messageCount, model)
	if err != nil {
		t.Fatalf("insert hermes v10 session: %v", err)
	}
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

func insertHermesMessage(t *testing.T, dbPath, sessionID, role, content string, timestamp float64) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open hermes db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set WAL mode: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO messages (session_id, role, content, timestamp)
		VALUES (?, ?, ?, ?)`,
		sessionID, role, content, timestamp)
	if err != nil {
		t.Fatalf("insert hermes message: %v", err)
	}
	_, _ = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}
