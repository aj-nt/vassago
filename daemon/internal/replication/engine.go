// This file is part of Vassago.
// See LICENSE for license information.

package replication

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aj-nt/vassago/daemon/internal/pubsub"
	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// EngineConfig holds configuration for the replication engine.
type EngineConfig struct {
	PollInterval time.Duration
}

// Engine manages peer connections and periodic delta sync.
type Engine struct {
	cfg    EngineConfig
	store  *store.Store
	hub    *pubsub.Hub
	peers  map[string]*Peer
	mu     sync.RWMutex
	cancel context.CancelFunc
}

// NewEngine creates a new replication engine.
func NewEngine(st *store.Store, hub *pubsub.Hub, cfg EngineConfig) *Engine {
	return &Engine{
		cfg:   cfg,
		store: st,
		hub:   hub,
		peers: make(map[string]*Peer),
	}
}

// Start connects to peers and begins the sync loop. Blocks until ctx is cancelled.
func (e *Engine) Start(ctx context.Context, peers []PeerConfig) error {
	ctx, e.cancel = context.WithCancel(ctx)
	defer e.cancel()

	// Connect to all configured peers
	for _, pc := range peers {
		p := NewPeer(pc)
		if err := p.Connect(ctx); err != nil {
			slog.Error("replication: failed to connect to peer", "peer_id", pc.ID, "addr", pc.Addr, "error", err)
			continue
		}
		e.peers[pc.ID] = p
	}

	if len(e.peers) == 0 {
		return fmt.Errorf("replication: failed to connect to any peers")
	}

	// Initial sync
	e.syncAll(ctx)

	// Periodic poll
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("replication: shutting down")
			for _, p := range e.peers {
				p.Close()
			}
			return ctx.Err()
		case <-ticker.C:
			e.syncAll(ctx)
		}
	}
}

// syncAll polls every peer for every configured namespace.
func (e *Engine) syncAll(ctx context.Context) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, p := range e.peers {
		for _, ns := range p.Namespaces {
			e.syncNamespace(ctx, p, ns)
		}
	}
}

// syncNamespace pulls deltas from a peer for one namespace.
func (e *Engine) syncNamespace(ctx context.Context, peer *Peer, namespace string) {
	state, err := e.store.GetReplicationState(peer.ID, namespace)
	if err != nil {
		slog.Error("replication: failed to get sync state", "peer_id", peer.ID, "namespace", namespace, "error", err)
		return
	}

	resp, err := peer.SyncChanges(ctx, namespace, state.LastSyncAt)
	if err != nil {
		slog.Warn("replication: sync failed", "peer_id", peer.ID, "namespace", namespace, "error", err)
		return
	}

	if len(resp.Entries) == 0 {
		return
	}

	applied, skipped := e.applyDelta(namespace, resp.Entries)

	// Update watermark
	if err := e.store.SetReplicationState(peer.ID, namespace, resp.ServerTime); err != nil {
		slog.Error("replication: failed to update sync state", "peer_id", peer.ID, "error", err)
		return
	}

	if applied > 0 || skipped > 0 {
		slog.Info("replication: synced", "peer_id", peer.ID, "namespace", namespace,
			"applied", applied, "skipped", skipped)
	}
}

// applyDelta applies remote changes to the local store. Returns (applied, skipped).
func (e *Engine) applyDelta(namespace string, entries []*pb.MemoryEntry) (int, int) {
	var applied, skipped int

	for _, remote := range entries {
		existing, err := e.findExisting(namespace, remote.Target, remote.Key)
		if err != nil {
			slog.Error("replication: find existing failed", "key", remote.Key, "error", err)
			skipped++
			continue
		}

		if existing != nil {
			if remote.UpdatedAt > existing.UpdatedAt {
				if err := e.overwriteMemory(existing, remote); err != nil {
					slog.Error("replication: overwrite failed", "key", remote.Key, "error", err)
					skipped++
					continue
				}
				applied++
			} else {
				skipped++
			}
		} else {
			if err := e.insertRemote(namespace, remote); err != nil {
				slog.Error("replication: insert failed", "key", remote.Key, "error", err)
				skipped++
				continue
			}
			applied++
		}

		// Publish event so local agents pick up the synced memory
		if e.hub != nil {
			e.hub.Publish(pubsub.Event{
				Type:     pb.UpdateEvent_ADDED,
				Target:   remote.Target,
				Category: remote.Category,
				Key:      remote.Key,
				AgentID:  remote.SourceAgent,
				Entry:    remote,
			})
		}
	}

	return applied, skipped
}

// findExisting looks for a memory with the given target+key in the namespace.
func (e *Engine) findExisting(namespace, target, key string) (*store.MemoryEntry, error) {
	entries, err := e.store.ListMemories(namespace, target, "", 0)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Key == key {
			return entry, nil
		}
	}
	return nil, nil
}

// overwriteMemory replaces local content with remote content.
func (e *Engine) overwriteMemory(local *store.MemoryEntry, remote *pb.MemoryEntry) error {
	content := remote.Content
	category := remote.Category
	priority := remote.Priority
	expiresAt := remote.ExpiresAt
	_, err := e.store.UpdateMemory(local.ID, local.Owner, store.MemoryPatch{
		Content:   &content,
		Category:  &category,
		Priority:  &priority,
		ExpiresAt: &expiresAt,
	})
	return err
}

// insertRemote inserts a remote memory entry into the local store.
func (e *Engine) insertRemote(namespace string, remote *pb.MemoryEntry) error {
	_, err := e.store.AddMemory(store.AddMemoryParams{
		Owner:       namespace,
		Target:      remote.Target,
		Category:    remote.Category,
		Key:         remote.Key,
		Content:     remote.Content,
		Priority:    remote.Priority,
		SourceAgent: remote.SourceAgent,
		ExpiresAt:   remote.ExpiresAt,
	})
	return err
}
