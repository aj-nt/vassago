// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// v0Schema is a minimal SQLite schema matching a fresh Vassago v0 database
// with no migrations applied. It includes the core tables (memories, agents,
// sessions, messages, subscriptions, memories_fts) without any columns added
// by migrations (owner, config, last_session, etc.).
//
// FIXME(breaking): SchemaVersion=9 but the Migrations array has ~35 entries
// (one per SQL statement, not one per logical migration). The migration
// loop iterates `i < SchemaVersion` which only applies the first 9
// statements. This means upgrading from a v0 database skips most tables
// (saved_tools, api_keys) and columns (owner, last_session, config).
// Fix by bumping SchemaVersion to len(Migrations) and making ALTER TABLE
// statements idempotent (wrap in column-existence checks).
const v0Schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;

CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    target TEXT NOT NULL CHECK(target IN ('memory', 'user')),
    category TEXT NOT NULL,
    key TEXT NOT NULL,
    content TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 2 CHECK(priority BETWEEN 1 AND 5),
    source_agent TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    accessed_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_memories_target ON memories(target);
CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
CREATE INDEX IF NOT EXISTS idx_memories_key ON memories(key);
CREATE INDEX IF NOT EXISTS idx_memories_priority ON memories(priority);
CREATE INDEX IF NOT EXISTS idx_memories_source_agent ON memories(source_agent);
CREATE INDEX IF NOT EXISTS idx_memories_expires_at ON memories(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_target_key ON memories(target, key);

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'cli',
    title TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    last_active_at INTEGER NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0,
    ended_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system', 'tool')),
    content TEXT NOT NULL DEFAULT '',
    tool_call_id TEXT NOT NULL DEFAULT '',
    tool_name TEXT NOT NULL DEFAULT '',
    timestamp INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);

CREATE TABLE subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    categories TEXT NOT NULL DEFAULT '[]',
    key_prefixes TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL
);

CREATE VIRTUAL TABLE memories_fts USING fts5(content, key, category, content=memories, content_rowid=rowid, tokenize='unicode61');

CREATE TRIGGER memories_fts_insert AFTER INSERT ON memories BEGIN INSERT INTO memories_fts(rowid, content, key, category) VALUES (new.rowid, new.content, new.key, new.category); END;
CREATE TRIGGER memories_fts_update AFTER UPDATE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, content, key, category) VALUES ('delete', old.rowid, old.content, old.key, old.category); INSERT INTO memories_fts(rowid, content, key, category) VALUES (new.rowid, new.content, new.key, new.category); END;
CREATE TRIGGER memories_fts_delete AFTER DELETE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, content, key, category) VALUES ('delete', old.rowid, old.content, old.key, old.category); END;

CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);
INSERT INTO schema_version VALUES (0, 1234567890);
`

// TestMigration_v0_opensWithoutCrash verifies that New() can open a v0-schema
// database without crashing. Due to the FIXME above, only the first 9 migration
// statements apply — the test verifies what actually works and documents gaps.
func TestMigration_v0_opensWithoutCrash(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v0.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open v0 database: %v", err)
	}
	if _, err := db.Exec(v0Schema); err != nil {
		db.Close()
		t.Fatalf("create v0 schema: %v", err)
	}
	db.Close()

	// Open through New() — applies whatever migrations fit within SchemaVersion=9.
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() on v0 database: %v", err)
	}
	defer s.Close()

	// Schema version should advance to SchemaVersion.
	var version int
	if err := s.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("after migration: schema version = %d, want %d", version, SchemaVersion)
	}

	// Verify all migration-added tables exist.
	allTables := []string{"todos", "cron_jobs", "skills", "saved_tools", "api_keys"}
	for _, table := range allTables {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("table %q should exist after migration (count=%d, err=%v)", table, count, err)
		}
	}

	// Verify owner columns were added by migration.
	ownerTables := []string{"memories", "sessions", "todos", "cron_jobs", "skills", "saved_tools"}
	for _, table := range ownerTables {
		var hasCol int
		err := s.db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name='owner'", table).Scan(&hasCol)
		if err != nil || hasCol != 1 {
			t.Errorf("table %q should have owner column after all migrations (hasCol=%d, err=%v)", table, hasCol, err)
		}
	}

	t.Logf("Successfully migrated v0 -> v%d with all tables and columns", SchemaVersion)
}

// TestMigration_NoOpMigration7 verifies migration 7 (SELECT 1) applies
// without error for a database at version 6.
func TestMigration_NoOpMigration7(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "v6.db")

	// Seed a database where todos, cron_jobs, skills tables exist
	// and schema_version=6, simulating the state after migration 6.
	// Need an agent row so cron_jobs FK constraint is satisfied.
	const seed = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;

CREATE TABLE memories (id TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', target TEXT NOT NULL, category TEXT NOT NULL, key TEXT NOT NULL, content TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 2, source_agent TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, accessed_at INTEGER NOT NULL, expires_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE agents (id TEXT PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, last_seen_at INTEGER NOT NULL, config TEXT NOT NULL DEFAULT '{}', last_session_id TEXT NOT NULL DEFAULT '', last_session_summary TEXT NOT NULL DEFAULT '');
CREATE TABLE sessions (id TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', agent_id TEXT NOT NULL, source TEXT NOT NULL DEFAULT 'cli', title TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, last_active_at INTEGER NOT NULL, message_count INTEGER NOT NULL DEFAULT 0, ended_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE messages (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL DEFAULT '', tool_call_id TEXT NOT NULL DEFAULT '', tool_name TEXT NOT NULL DEFAULT '', timestamp INTEGER NOT NULL);
CREATE TABLE subscriptions (id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, target TEXT NOT NULL DEFAULT '', categories TEXT NOT NULL DEFAULT '[]', key_prefixes TEXT NOT NULL DEFAULT '[]', created_at INTEGER NOT NULL);
CREATE TABLE todos (id TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', content TEXT NOT NULL, completed INTEGER NOT NULL DEFAULT 0, priority INTEGER NOT NULL DEFAULT 3, source_agent TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, completed_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE cron_jobs (id TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', name TEXT NOT NULL, schedule TEXT NOT NULL, agent_id TEXT NOT NULL, message TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, last_fire_at INTEGER NOT NULL DEFAULT 0, next_fire_at INTEGER NOT NULL DEFAULT 0);
CREATE TABLE skills (id TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', name TEXT NOT NULL, description TEXT NOT NULL, triggers TEXT NOT NULL DEFAULT '[]', category TEXT NOT NULL, content TEXT NOT NULL, source_agent TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 3, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE saved_tools (id TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', name TEXT NOT NULL UNIQUE, description TEXT NOT NULL, type TEXT NOT NULL, parameters TEXT NOT NULL, script TEXT NOT NULL DEFAULT '', category TEXT NOT NULL DEFAULT '', triggers TEXT NOT NULL DEFAULT '[]', author TEXT NOT NULL, priority INTEGER NOT NULL DEFAULT 3, version INTEGER NOT NULL DEFAULT 1, requires_approval_every_call INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE api_keys (api_key TEXT PRIMARY KEY, owner TEXT NOT NULL DEFAULT 'default', created_at INTEGER NOT NULL);
CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL);
INSERT INTO schema_version VALUES (6, 1234567890);

INSERT INTO agents (id, name, role, created_at, last_seen_at) VALUES ('mig-test', 'mig-test', 'dev', 1, 1);
`

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	if _, err := db.Exec(seed); err != nil {
		db.Close()
		t.Fatalf("seed database: %v", err)
	}
	db.Close()

	// Re-open through New() — should apply migrations 7, 8, 9.
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() on version-6 database: %v", err)
	}
	defer s.Close()

	var version int
	if err := s.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("after migration: schema version = %d, want %d", version, SchemaVersion)
	}

	// Verify basic functionality: can write/read memories.
	_, err = s.AddMemory(AddMemoryParams{
		Owner: "default", Target: "memory", Category: "test",
		Key: "mig-test", Content: "migration test entry",
		Priority: 3, SourceAgent: "mig-test",
	})
	if err != nil {
		t.Errorf("AddMemory after migration: %v", err)
	}

	entries, err := s.ListMemories("default", "memory", "test", 0)
	if err != nil {
		t.Errorf("ListMemories after migration: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Key == "mig-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("migration test memory not found after re-open")
	}
}
