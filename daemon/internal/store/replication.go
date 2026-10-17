// This file is part of Vassago.
// See LICENSE for license information.

package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// GetChangesSince returns all memories in the given namespace updated
// since the given Unix timestamp. Used for peer-to-peer delta sync.
func (s *Store) GetChangesSince(namespace string, since int64) ([]*MemoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, owner, target, category, key, content, priority,
		       source_agent, created_at, updated_at, accessed_at, expires_at
		FROM memories
		WHERE owner = ? AND updated_at > ?
		ORDER BY updated_at ASC
	`, namespace, since)
	if err != nil {
		return nil, fmt.Errorf("query changes since %d: %w", since, err)
	}
	defer func() { _ = rows.Close() }()

	var entries []*MemoryEntry
	for rows.Next() {
		e := &MemoryEntry{}
		err := rows.Scan(
			&e.ID, &e.Owner, &e.Target, &e.Category, &e.Key, &e.Content,
			&e.Priority, &e.SourceAgent, &e.CreatedAt, &e.UpdatedAt,
			&e.AccessedAt, &e.ExpiresAt,
		)
		if err != nil {
			slog.Error("GetChangesSince: scan failed", "error", err)
			continue
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// ReplicationState holds sync state for a given peer+namespace.
type ReplicationState struct {
	PeerID           string
	Namespace        string
	LastSyncAt       int64
	LastSyncMemoryID string
}

// GetReplicationState returns the sync state for a peer+namespace.
// Returns a zero-value ReplicationState if no row exists.
func (s *Store) GetReplicationState(peerID, namespace string) (*ReplicationState, error) {
	var state ReplicationState
	err := s.db.QueryRow(
		"SELECT peer_id, namespace, last_sync_at, last_sync_memory_id FROM vassago_replication WHERE peer_id = ? AND namespace = ?",
		peerID, namespace,
	).Scan(&state.PeerID, &state.Namespace, &state.LastSyncAt, &state.LastSyncMemoryID)
	if err == sql.ErrNoRows {
		return &ReplicationState{PeerID: peerID, Namespace: namespace}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get replication state: %w", err)
	}
	return &state, nil
}

// SetReplicationState upserts the sync state for a peer+namespace.
func (s *Store) SetReplicationState(peerID, namespace string, lastSyncAt int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO vassago_replication (peer_id, namespace, last_sync_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(peer_id, namespace) DO UPDATE SET
			last_sync_at = excluded.last_sync_at,
			updated_at = excluded.updated_at
	`, peerID, namespace, lastSyncAt, now)
	if err != nil {
		return fmt.Errorf("set replication state: %w", err)
	}
	return nil
}
