package store

// Schema version for the Mnemosyne database.
// Increment on any schema change. The daemon applies migrations
// sequentially on startup.
const SchemaVersion = 1

// SchemaStmts are executed sequentially to create a fresh database.
// Each entry is a complete SQL statement (no semicolons inside —
// trigger bodies must be on a single line or use exec directly).
var SchemaStmts = []string{
	// --- Core tables ---

	`CREATE TABLE IF NOT EXISTS memories (
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
)`,

	`CREATE INDEX IF NOT EXISTS idx_memories_target ON memories(target)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_key ON memories(key)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_priority ON memories(priority)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_source_agent ON memories(source_agent)`,
	`CREATE INDEX IF NOT EXISTS idx_memories_expires_at ON memories(expires_at)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_memories_target_key ON memories(target, key)`,

	`CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    config TEXT NOT NULL DEFAULT '{}'
)`,

	`CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'cli',
    title TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    last_active_at INTEGER NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0,
    ended_at INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
)`,

	`CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system', 'tool')),
    content TEXT NOT NULL DEFAULT '',
    tool_call_id TEXT NOT NULL DEFAULT '',
    tool_name TEXT NOT NULL DEFAULT '',
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
)`,

	`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp)`,

	`CREATE TABLE IF NOT EXISTS subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    categories TEXT NOT NULL DEFAULT '[]',
    key_prefixes TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
)`,

	// --- FTS5 virtual table ---
	`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(content, key, category, content=memories, content_rowid=rowid, tokenize='unicode61')`,

	// --- FTS5 sync triggers ---
	`CREATE TRIGGER IF NOT EXISTS memories_fts_insert AFTER INSERT ON memories BEGIN INSERT INTO memories_fts(rowid, content, key, category) VALUES (new.rowid, new.content, new.key, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS memories_fts_update AFTER UPDATE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, content, key, category) VALUES ('delete', old.rowid, old.content, old.key, old.category); INSERT INTO memories_fts(rowid, content, key, category) VALUES (new.rowid, new.content, new.key, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS memories_fts_delete AFTER DELETE ON memories BEGIN INSERT INTO memories_fts(memories_fts, rowid, content, key, category) VALUES ('delete', old.rowid, old.content, old.key, old.category); END`,

	// --- Schema version tracking ---
	`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`,
}

// Migrations is a list of SQL statements to apply sequentially.
// Each migration is applied in order, and the schema_version table
// tracks which migrations have been applied.
var Migrations = []string{}