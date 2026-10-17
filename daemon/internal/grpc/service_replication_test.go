// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
	"github.com/aj-nt/vassago/daemon/internal/store"
)

func TestSyncChanges_returnsOnlyNewEntries(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)

	_, err := svc.store.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "test",
		Key: "k1", Content: "hello", Priority: 3, SourceAgent: "agent-a",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	since := time.Now().Unix()
	time.Sleep(1100 * time.Millisecond)

	_, err = svc.store.AddMemory(store.AddMemoryParams{
		Owner: "default", Target: "memory", Category: "test",
		Key: "k2", Content: "world", Priority: 3, SourceAgent: "agent-b",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	resp, err := svc.SyncChanges(context.Background(), &pb.SyncChangesRequest{
		PeerId:    "test-peer",
		Namespace: "default",
		Since:     since,
	})
	if err != nil {
		t.Fatalf("SyncChanges: %v", err)
	}

	if len(resp.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(resp.Entries))
	}
	if resp.Entries[0].Key != "k2" {
		t.Errorf("got key %q, want %q", resp.Entries[0].Key, "k2")
	}
	if resp.ServerTime == 0 {
		t.Error("ServerTime should be non-zero")
	}
}

func TestSyncChanges_missingNamespace(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)

	_, err := svc.SyncChanges(context.Background(), &pb.SyncChangesRequest{
		Namespace: "",
	})
	if err == nil {
		t.Error("expected error for missing namespace, got nil")
	}
}
