#!/usr/bin/env python3
"""
Migrate Hermes state.db → Mnemosyne mnemosyne.db

Handles schema differences:
- memories: Hermes INTEGER id → Mnemosyne UUID (deterministic via namespace UUID)
- sessions: Preserve Hermes TEXT ids directly
- messages: Hermes has more columns; we map what Mnemosyne needs
- agents: Create a "hermes" agent for all imported data

Usage:
  1. Stop the Mnemosyne daemon
  2. Back up existing mnemosyne.db
  3. Run: python3 migrate_hermes_to_mnemosyne.py [--fresh]
     --fresh: Drop all existing data and start fresh
     --merge: Keep existing Mnemosyne data and merge Hermes data in (default)
  4. Restart the daemon
"""

import sqlite3
import uuid
import time
import sys
import os
import shutil

HERMES_DB = os.path.expanduser("~/.hermes/state.db")
MNEMOSYNE_DB = os.path.expanduser("~/.mnemosyne/mnemosyne.db")
BACKUP_SUFFIX = f".pre-migration-{int(time.time())}"

# Namespace UUID for deterministic Hermes→Mnemosyne ID mapping
NAMESPACE = uuid.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")  # standard DNS namespace

def hermes_memory_id_to_uuid(hid: int) -> str:
    """Deterministically map Hermes INTEGER memory id to Mnemosyne UUID string."""
    return str(uuid.uuid5(NAMESPACE, f"hermes-memory-{hid}"))

def create_hermes_agent(cursor) -> str:
    """Create or get the hermes-import agent ID."""
    agent_id = str(uuid.uuid5(NAMESPACE, "hermes-agent"))
    now = int(time.time())
    cursor.execute(
        "INSERT OR IGNORE INTO agents (id, name, role, created_at, last_seen_at, config) VALUES (?, ?, ?, ?, ?, ?)",
        (agent_id, "hermes", "imported", now, now, "{}")
    )
    return agent_id

def migrate_memories(src_cursor, dst_cursor, agent_id: str) -> int:
    """Migrate memories from Hermes to Mnemosyne."""
    src_cursor.execute("""
        SELECT id, target, category, key, content, priority, 
               created_at, updated_at, last_accessed, access_count, source_session
        FROM memories ORDER BY id
    """)
    rows = src_cursor.fetchall()
    count = 0
    for row in rows:
        hid, target, category, key, content, priority = row[:6]
        created_at, updated_at, last_accessed = int(row[6]), int(row[7]), int(row[8])
        access_count = row[9]
        # source_session = row[10]  # Mnemosyne doesn't have this field
        
        new_id = hermes_memory_id_to_uuid(hid)
        
        # Mnemosyne memories table schema:
        # id, target, category, key, content, priority, source_agent, created_at, updated_at, accessed_at, expires_at
        # NOTE: Mnemosyne doesn't have access_count or source_session
        
        dst_cursor.execute("""
            INSERT OR REPLACE INTO memories 
            (id, target, category, key, content, priority, source_agent, created_at, updated_at, accessed_at, expires_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
        """, (new_id, target, category, key, content, priority, agent_id, created_at, updated_at, last_accessed))
        count += 1
    
    return count

def migrate_sessions(src_cursor, dst_cursor, agent_id: str) -> dict:
    """Migrate sessions from Hermes to Mnemosyne.
    Returns mapping of session_id → agent_id for use in message migration.
    """
    src_cursor.execute("""
        SELECT id, source, title, started_at, ended_at, message_count
        FROM sessions ORDER BY started_at
    """)
    rows = src_cursor.fetchall()
    count = 0
    for row in rows:
        sid, source, title, started_at, ended_at, message_count = row
        
        # Preserve the Hermes session ID directly (it's already TEXT)
        # Use ended_at if available, otherwise started_at as last_active_at
        last_active = int(ended_at) if ended_at else int(started_at)
        ended_int = int(ended_at) if ended_at else 0
        
        src = source or "cli"
        ttl = title or ""
        
        dst_cursor.execute("""
            INSERT OR IGNORE INTO sessions 
            (id, agent_id, source, title, created_at, last_active_at, message_count, ended_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """, (sid, agent_id, src, ttl, int(started_at), last_active, message_count or 0, ended_int))
        count += 1
    
    return count

def migrate_messages(src_cursor, dst_cursor) -> int:
    """Migrate messages from Hermes to Mnemosyne.
    Mnemosyne schema: id (auto), session_id, role, content, tool_call_id, tool_name, timestamp
    Hermes schema: id (auto), session_id, role, content, tool_call_id, tool_calls, tool_name, timestamp, ...
    """
    # Process in batches to avoid memory issues with 76K+ messages
    BATCH_SIZE = 5000
    offset = 0
    total = 0
    
    # Get total count
    src_cursor.execute("SELECT COUNT(*) FROM messages")
    total_count = src_cursor.fetchone()[0]
    
    while offset < total_count:
        src_cursor.execute("""
            SELECT session_id, role, content, tool_call_id, tool_name, timestamp
            FROM messages ORDER BY id LIMIT ? OFFSET ?
        """, (BATCH_SIZE, offset))
        rows = src_cursor.fetchall()
        
        for row in rows:
            session_id, role, content, tool_call_id, tool_name, timestamp = row
            
            # Handle NULL/empty content - Mnemosyne requires empty string default
            content = content or ""
            tool_call_id = tool_call_id or ""
            tool_name = tool_name or ""
            role = role or "assistant"
            
            dst_cursor.execute("""
                INSERT INTO messages 
                (session_id, role, content, tool_call_id, tool_name, timestamp)
                VALUES (?, ?, ?, ?, ?, ?)
            """, (session_id, role, content, tool_call_id, tool_name, int(timestamp)))
        
        total += len(rows)
        offset += BATCH_SIZE
        print(f"  Messages: {total}/{total_count}...")
    
    return total

def clear_mnemosyne_data(cursor):
    """Clear all existing data from Mnemosyne tables (for --fresh mode)."""
    # Delete in reverse dependency order
    # Note: FTS tables already dropped before this function is called
    cursor.execute("DELETE FROM messages")
    cursor.execute("DELETE FROM sessions")
    cursor.execute("DELETE FROM memories")
    cursor.execute("DELETE FROM agents")
    # Reset autoincrement
    cursor.execute("DELETE FROM sqlite_sequence WHERE name IN ('messages')")

def main():
    mode = "--merge"
    if len(sys.argv) > 1:
        if sys.argv[1] == "--fresh":
            mode = "--fresh"
        elif sys.argv[1] == "--merge":
            mode = "--merge"
        else:
            print(f"Unknown option: {sys.argv[1]}")
            print("Usage: python3 migrate_hermes_to_mnemosyne.py [--fresh|--merge]")
            sys.exit(1)
    
    # Verify source DB exists
    if not os.path.exists(HERMES_DB):
        print(f"ERROR: Hermes DB not found at {HERMES_DB}")
        sys.exit(1)
    
    if not os.path.exists(MNEMOSYNE_DB):
        print(f"ERROR: Mnemosyne DB not found at {MNEMOSYNE_DB}")
        sys.exit(1)
    
    # Backup Mnemosyne DB
    backup_path = MNEMOSYNE_DB + BACKUP_SUFFIX
    print(f"Backing up {MNEMOSYNE_DB} → {backup_path}")
    shutil.copy2(MNEMOSYNE_DB, backup_path)
    
    # Open connections
    src_conn = sqlite3.connect(HERMES_DB, uri=True)
    src_conn.row_factory = None  # plain tuples
    src_cursor = src_conn.cursor()
    
    dst_conn = sqlite3.connect(MNEMOSYNE_DB)
    dst_cursor = dst_conn.cursor()
    
    # Drop and recreate FTS index for bulk insert performance
    # (FTS5 shadow tables can't be modified directly)
    print("Dropping FTS index for bulk insert...")
    dst_cursor.execute("DROP TABLE IF EXISTS memories_fts")
    dst_cursor.execute("DROP TRIGGER IF EXISTS memories_fts_insert")
    dst_cursor.execute("DROP TRIGGER IF EXISTS memories_fts_update")
    dst_cursor.execute("DROP TRIGGER IF EXISTS memories_fts_delete")
    
    if mode == "--fresh":
        print("CLEARING existing Mnemosyne data (--fresh mode)...")
        clear_mnemosyne_data(dst_cursor)
    else:
        print("MERGE mode: Keeping existing Mnemosyne data, adding Hermes data...")
    
    # Create the hermes agent
    print("\nCreating hermes agent...")
    agent_id = create_hermes_agent(dst_cursor)
    dst_conn.commit()
    
    # Migrate memories
    print("\nMigrating memories...")
    mem_count = migrate_memories(src_cursor, dst_cursor, agent_id)
    dst_conn.commit()
    print(f"  → {mem_count} memories migrated")
    
    # Migrate sessions
    print("\nMigrating sessions...")
    sess_count = migrate_sessions(src_cursor, dst_cursor, agent_id)
    dst_conn.commit()
    print(f"  → {sess_count} sessions migrated")
    
    # Migrate messages
    print("\nMigrating messages (this may take a moment with 76K+ rows)...")
    msg_count = migrate_messages(src_cursor, dst_cursor)
    dst_conn.commit()
    print(f"  → {msg_count} messages migrated")
    
    # Recreate FTS index
    print("\nRecreating FTS index...")
    dst_cursor.execute("""
        CREATE VIRTUAL TABLE memories_fts USING fts5(
            content, key, category, 
            content=memories, content_rowid=rowid, 
            tokenize='unicode61'
        )
    """)
    dst_cursor.execute("""
        CREATE TRIGGER memories_fts_insert AFTER INSERT ON memories BEGIN
            INSERT INTO memories_fts(rowid, content, key, category) 
            VALUES (new.rowid, new.content, new.key, new.category);
        END
    """)
    dst_cursor.execute("""
        CREATE TRIGGER memories_fts_update AFTER UPDATE ON memories BEGIN
            INSERT INTO memories_fts(memories_fts, rowid, content, key, category) 
            VALUES ('delete', old.rowid, old.content, old.key, old.category);
            INSERT INTO memories_fts(rowid, content, key, category) 
            VALUES (new.rowid, new.content, new.key, new.category);
        END
    """)
    dst_cursor.execute("""
        CREATE TRIGGER memories_fts_delete AFTER DELETE ON memories BEGIN
            INSERT INTO memories_fts(memories_fts, rowid, content, key, category) 
            VALUES ('delete', old.rowid, old.content, old.key, old.category);
        END
    """)
    # Populate FTS from existing data
    dst_cursor.execute("""
        INSERT INTO memories_fts(rowid, content, key, category)
        SELECT rowid, content, key, category FROM memories
    """)
    dst_conn.commit()
    
    # Verify counts
    print("\n=== Verification ===")
    dst_cursor.execute("SELECT 'memories', COUNT(*) FROM memories")
    print(f"  Memories: {dst_cursor.fetchone()[1]}")
    dst_cursor.execute("SELECT 'sessions', COUNT(*) FROM sessions")
    print(f"  Sessions: {dst_cursor.fetchone()[1]}")
    dst_cursor.execute("SELECT 'messages', COUNT(*) FROM messages")
    print(f"  Messages: {dst_cursor.fetchone()[1]}")
    dst_cursor.execute("SELECT 'agents', COUNT(*) FROM agents")
    print(f"  Agents: {dst_cursor.fetchone()[1]}")
    
    # Check for orphan messages (session_id not in sessions)
    dst_cursor.execute("""
        SELECT COUNT(*) FROM messages m 
        WHERE NOT EXISTS (SELECT 1 FROM sessions s WHERE s.id = m.session_id)
    """)
    orphans = dst_cursor.fetchone()[0]
    if orphans > 0:
        print(f"  ⚠️  {orphans} orphan messages (session_id not in sessions table)")
    else:
        print("  ✅ No orphan messages")
    
    # Close connections
    src_conn.close()
    dst_conn.close()
    
    print(f"\n✅ Migration complete! Backup at: {backup_path}")
    print("   Restart the Mnemosyne daemon to use the new data.")

if __name__ == "__main__":
    main()