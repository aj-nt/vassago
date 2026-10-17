// This file is part of Vassago.
// See LICENSE for license information.

package replication

import (
	"testing"
	"time"

	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEngine_applyDelta_lastWriteWins(t *testing.T) {
	s := testStore(t)
	eng := &Engine{store: s, peers: make(map[string]*Peer)}

	// Local: older version
	s.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "test",
		Key: "conflict-test", Content: "local-older", Priority: 3,
		SourceAgent: "agent-a",
	})
	time.Sleep(1100 * time.Millisecond)

	// Remote: newer version (higher updated_at)
	now := time.Now().Unix()
	remoteEntries := []*pb.MemoryEntry{{
		Id:          "remote-id",
		Target:      "memory",
		Category:    "test",
		Key:         "conflict-test",
		Content:     "remote-newer",
		Priority:    3,
		SourceAgent: "peer-agent",
		UpdatedAt:   now, // newer
	}}

	applied, skipped := eng.applyDelta("default", remoteEntries)
	if applied != 1 {
		t.Errorf("applied = %d, want 1 (remote should win)", applied)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}

	// Verify local has remote content
	entries, _ := s.ListMemories("default", "memory", "test", 0)
	var found *store.MemoryEntry
	for _, e := range entries {
		if e.Key == "conflict-test" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatal("memory not found after sync")
	}
	if found.Content != "remote-newer" {
		t.Errorf("content = %q, want %q", found.Content, "remote-newer")
	}
}

func TestEngine_applyDelta_localWins(t *testing.T) {
	s := testStore(t)
	eng := &Engine{store: s, peers: make(map[string]*Peer)}

	// Local: newer version
	s.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "test",
		Key: "conflict-local", Content: "local-newer", Priority: 3,
		SourceAgent: "agent-a",
	})
	time.Sleep(1100 * time.Millisecond)

	now := time.Now().Unix()

	// Remote: older version
	remoteEntries := []*pb.MemoryEntry{{
		Id:          "remote-old-id",
		Target:      "memory",
		Category:    "test",
		Key:         "conflict-local",
		Content:     "remote-older",
		Priority:    3,
		SourceAgent: "peer-agent",
		UpdatedAt:   now - 100, // older than local
	}}

	applied, skipped := eng.applyDelta("default", remoteEntries)
	if applied != 0 {
		t.Errorf("applied = %d, want 0 (local is newer)", applied)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}

	// Verify local still has its content
	entries, _ := s.ListMemories("default", "memory", "test", 0)
	var found *store.MemoryEntry
	for _, e := range entries {
		if e.Key == "conflict-local" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatal("memory not found after sync")
	}
	if found.Content != "local-newer" {
		t.Errorf("content = %q, want %q", found.Content, "local-newer")
	}
}

func TestEngine_applyDelta_insertNew(t *testing.T) {
	s := testStore(t)
	eng := &Engine{store: s, peers: make(map[string]*Peer)}

	remoteEntries := []*pb.MemoryEntry{{
		Id:          "new-remote-id",
		Target:      "memory",
		Category:    "test",
		Key:         "no-conflict",
		Content:     "remote-only",
		Priority:    3,
		SourceAgent: "peer-agent",
		UpdatedAt:   time.Now().Unix(),
	}}

	applied, skipped := eng.applyDelta("default", remoteEntries)
	if applied != 1 {
		t.Errorf("applied = %d, want 1", applied)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}

	entries, _ := s.ListMemories("default", "memory", "test", 0)
	var found *store.MemoryEntry
	for _, e := range entries {
		if e.Key == "no-conflict" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatal("new remote memory was not inserted")
	}
	if found.Content != "remote-only" {
		t.Errorf("content = %q, want %q", found.Content, "remote-only")
	}
}
