// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

// Schema version for the Mnemosyne database.
// Increment on any schema change. The daemon applies migrations
// sequentially on startup.
const SchemaVersion = 35

// SchemaStmts are executed sequentially to create a fresh database.
// Each entry is a complete SQL statement (no semicolons inside —
// trigger bodies must be on a single line or use exec directly).
var SchemaStmts = []string{
	// --- Core tables ---

	`CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL DEFAULT 'default',
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
    config TEXT NOT NULL DEFAULT '{}',
    last_session_id TEXT NOT NULL DEFAULT '',
    last_session_summary TEXT NOT NULL DEFAULT ''
)`,

	`CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL DEFAULT 'default',
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

	// --- Todos ---
	`CREATE TABLE IF NOT EXISTS todos (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL DEFAULT 'default',
    content TEXT NOT NULL,
    completed INTEGER NOT NULL DEFAULT 0 CHECK(completed IN (0, 1)),
    priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    source_agent TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER NOT NULL DEFAULT 0
)`,

	`CREATE INDEX IF NOT EXISTS idx_todos_completed ON todos(completed)`,
	`CREATE INDEX IF NOT EXISTS idx_todos_priority ON todos(priority)`,
	`CREATE INDEX IF NOT EXISTS idx_todos_source_agent ON todos(source_agent)`,

	// --- Cron Jobs ---
	`CREATE TABLE IF NOT EXISTS cron_jobs (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL DEFAULT 'default',
    name TEXT NOT NULL,
    schedule TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    message TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_fire_at INTEGER NOT NULL DEFAULT 0,
    next_fire_at INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
)`,

	`CREATE INDEX IF NOT EXISTS idx_cron_jobs_agent_id ON cron_jobs(agent_id)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled ON cron_jobs(enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_jobs_next_fire ON cron_jobs(next_fire_at)`,

	// --- Skills ---
	`CREATE TABLE IF NOT EXISTS skills (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL DEFAULT 'default',
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    triggers TEXT NOT NULL DEFAULT '[]',
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    source_agent TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,

	`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category)`,
	`CREATE INDEX IF NOT EXISTS idx_skills_source_agent ON skills(source_agent)`,
	`CREATE INDEX IF NOT EXISTS idx_skills_priority ON skills(priority)`,

	// --- Skills FTS5 virtual table ---
	`CREATE VIRTUAL TABLE IF NOT EXISTS skills_fts USING fts5(name, description, content, triggers, category, content=skills, content_rowid=rowid, tokenize='unicode61')`,

	// --- Skills FTS5 sync triggers ---
	`CREATE TRIGGER IF NOT EXISTS skills_fts_insert AFTER INSERT ON skills BEGIN INSERT INTO skills_fts(rowid, name, description, content, triggers, category) VALUES (new.rowid, new.name, new.description, new.content, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS skills_fts_update AFTER UPDATE ON skills BEGIN INSERT INTO skills_fts(skills_fts, rowid, name, description, content, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.content, old.triggers, old.category); INSERT INTO skills_fts(rowid, name, description, content, triggers, category) VALUES (new.rowid, new.name, new.description, new.content, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS skills_fts_delete AFTER DELETE ON skills BEGIN INSERT INTO skills_fts(skills_fts, rowid, name, description, content, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.content, old.triggers, old.category); END`,

	// Migration 4: Add saved_tools table (Phase 25A)
	`CREATE TABLE IF NOT EXISTS saved_tools (
    id TEXT PRIMARY KEY,
    owner TEXT NOT NULL DEFAULT 'default',
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('script')),
    parameters TEXT NOT NULL,
    script TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    triggers TEXT NOT NULL DEFAULT '[]',
    author TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    version INTEGER NOT NULL DEFAULT 1,
    requires_approval_every_call INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,

	`CREATE INDEX IF NOT EXISTS idx_saved_tools_category ON saved_tools(category)`,
	`CREATE INDEX IF NOT EXISTS idx_saved_tools_author ON saved_tools(author)`,
	`CREATE INDEX IF NOT EXISTS idx_saved_tools_priority ON saved_tools(priority)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS saved_tools_fts USING fts5(name, description, triggers, category, content=saved_tools, content_rowid=rowid, tokenize='unicode61')`,

	`CREATE TRIGGER IF NOT EXISTS saved_tools_fts_insert AFTER INSERT ON saved_tools BEGIN INSERT INTO saved_tools_fts(rowid, name, description, triggers, category) VALUES (new.rowid, new.name, new.description, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS saved_tools_fts_update AFTER UPDATE ON saved_tools BEGIN INSERT INTO saved_tools_fts(saved_tools_fts, rowid, name, description, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.triggers, old.category); INSERT INTO saved_tools_fts(rowid, name, description, triggers, category) VALUES (new.rowid, new.name, new.description, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS saved_tools_fts_delete AFTER DELETE ON saved_tools BEGIN INSERT INTO saved_tools_fts(saved_tools_fts, rowid, name, description, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.triggers, old.category); END`,

	// --- API key to owner mapping (Phase 28) ---
	`CREATE TABLE IF NOT EXISTS api_keys (
		api_key TEXT PRIMARY KEY,
		owner TEXT NOT NULL DEFAULT 'default',
		created_at INTEGER NOT NULL
	)`,

	`CREATE INDEX IF NOT EXISTS idx_api_keys_owner ON api_keys(owner)`,
}

// Migrations is a list of SQL statements to apply sequentially.
// Each migration is applied in order, and the schema_version table
// tracks which migrations have been applied.
var Migrations = []string{
	// Migration 1: Add todos table (Phase 17C)
	`CREATE TABLE IF NOT EXISTS todos (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    completed INTEGER NOT NULL DEFAULT 0 CHECK(completed IN (0, 1)),
    priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    source_agent TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER NOT NULL DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS idx_todos_completed ON todos(completed)`,
	`CREATE INDEX IF NOT EXISTS idx_todos_priority ON todos(priority)`,
	`CREATE INDEX IF NOT EXISTS idx_todos_source_agent ON todos(source_agent)`,

	// Migration 2: Add cron_jobs table (Phase 16A)
	`CREATE TABLE IF NOT EXISTS cron_jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    schedule TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    message TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_fire_at INTEGER NOT NULL DEFAULT 0,
    next_fire_at INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_jobs_agent_id ON cron_jobs(agent_id)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled ON cron_jobs(enabled)`,
	`CREATE INDEX IF NOT EXISTS idx_cron_jobs_next_fire ON cron_jobs(next_fire_at)`,

	// Migration 3: Add skills table (Phase 18A)
	`CREATE TABLE IF NOT EXISTS skills (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    triggers TEXT NOT NULL DEFAULT '[]',
    category TEXT NOT NULL,
    content TEXT NOT NULL,
    source_agent TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,
	`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category)`,
	`CREATE INDEX IF NOT EXISTS idx_skills_source_agent ON skills(source_agent)`,
	`CREATE INDEX IF NOT EXISTS idx_skills_priority ON skills(priority)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS skills_fts USING fts5(name, description, content, triggers, category, content=skills, content_rowid=rowid, tokenize='unicode61')`,

	`CREATE TRIGGER IF NOT EXISTS skills_fts_insert AFTER INSERT ON skills BEGIN INSERT INTO skills_fts(rowid, name, description, content, triggers, category) VALUES (new.rowid, new.name, new.description, new.content, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS skills_fts_update AFTER UPDATE ON skills BEGIN INSERT INTO skills_fts(skills_fts, rowid, name, description, content, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.content, old.triggers, old.category); INSERT INTO skills_fts(rowid, name, description, content, triggers, category) VALUES (new.rowid, new.name, new.description, new.content, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS skills_fts_delete AFTER DELETE ON skills BEGIN INSERT INTO skills_fts(skills_fts, rowid, name, description, content, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.content, old.triggers, old.category); END`,

	// Migration 4: Add saved_tools table (Phase 25)
	`CREATE TABLE IF NOT EXISTS saved_tools (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('script')),
    parameters TEXT NOT NULL,
    script TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    triggers TEXT NOT NULL DEFAULT '[]',
    author TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 3 CHECK(priority BETWEEN 1 AND 5),
    version INTEGER NOT NULL DEFAULT 1,
    requires_approval_every_call INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
)`,

	`CREATE INDEX IF NOT EXISTS idx_saved_tools_category ON saved_tools(category)`,
	`CREATE INDEX IF NOT EXISTS idx_saved_tools_author ON saved_tools(author)`,
	`CREATE INDEX IF NOT EXISTS idx_saved_tools_priority ON saved_tools(priority)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS saved_tools_fts USING fts5(name, description, triggers, category, content=saved_tools, content_rowid=rowid, tokenize='unicode61')`,

	`CREATE TRIGGER IF NOT EXISTS saved_tools_fts_insert AFTER INSERT ON saved_tools BEGIN INSERT INTO saved_tools_fts(rowid, name, description, triggers, category) VALUES (new.rowid, new.name, new.description, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS saved_tools_fts_update AFTER UPDATE ON saved_tools BEGIN INSERT INTO saved_tools_fts(saved_tools_fts, rowid, name, description, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.triggers, old.category); INSERT INTO saved_tools_fts(rowid, name, description, triggers, category) VALUES (new.rowid, new.name, new.description, new.triggers, new.category); END`,

	`CREATE TRIGGER IF NOT EXISTS saved_tools_fts_delete AFTER DELETE ON saved_tools BEGIN INSERT INTO saved_tools_fts(saved_tools_fts, rowid, name, description, triggers, category) VALUES ('delete', old.rowid, old.name, old.description, old.triggers, old.category); END`,
	// Migration 5: Add owner to memories (Phase 28)
	`ALTER TABLE memories ADD COLUMN owner TEXT NOT NULL DEFAULT 'default'`,

	// Migration 6: Add owner to remaining tables (Phase 28)
	`ALTER TABLE sessions ADD COLUMN owner TEXT NOT NULL DEFAULT 'default'`,
	`ALTER TABLE todos ADD COLUMN owner TEXT NOT NULL DEFAULT 'default'`,
	`ALTER TABLE cron_jobs ADD COLUMN owner TEXT NOT NULL DEFAULT 'default'`,
	`ALTER TABLE skills ADD COLUMN owner TEXT NOT NULL DEFAULT 'default'`,
	`ALTER TABLE saved_tools ADD COLUMN owner TEXT NOT NULL DEFAULT 'default'`,

	// Migration 7: No-op — safety net for migration 6 partial failures.
	// Migration 6 added owner columns to 5 tables. If it succeeded (which
	// is the normal case), re-adding those columns in migration 7 would
	// fail with "duplicate column name" and halt startup. So migration 7
	// is a no-op: it advances the schema version without modifying tables.
	// If your database is stuck at v6 with missing owner columns, you
	// need to apply migration 6's ALTER TABLE statements manually.
	`SELECT 1`,

	// Migration 8: Add api_keys table for API key → owner mapping (Phase 28)
	`CREATE TABLE IF NOT EXISTS api_keys (
		api_key TEXT PRIMARY KEY,
		owner TEXT NOT NULL DEFAULT 'default',
		created_at INTEGER NOT NULL
	)`,

	`CREATE INDEX IF NOT EXISTS idx_api_keys_owner ON api_keys(owner)`,

	// Migration 9: Add last-session tracking to agents
	`ALTER TABLE agents ADD COLUMN last_session_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE agents ADD COLUMN last_session_summary TEXT NOT NULL DEFAULT ''`,
}
