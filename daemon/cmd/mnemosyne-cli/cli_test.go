// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	daemongrpc "github.com/aj-nt/mnemosyne/daemon/internal/grpc"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startTestDaemon creates a daemon with a temp DB and starts a gRPC server
// on a random TCP port. Returns the address and a cleanup function.
func startTestDaemon(t *testing.T) (string, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	svc := daemongrpc.NewService(st, nil, nil)
	server := grpc.NewServer()
	pb.RegisterMnemosyneServer(server, svc)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		_ = server.Serve(lis)
	}()

	addr := lis.Addr().String()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	cleanup := func() {
		server.GracefulStop()
		st.Close()
	}

	return addr, cleanup
}

// dialTestDaemon connects to a test daemon and returns the gRPC client.
func dialTestDaemon(t *testing.T, addr string) (pb.MnemosyneClient, *grpc.ClientConn) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return pb.NewMnemosyneClient(conn), conn
}

func TestCLIAddAndGet(t *testing.T) {
	addr, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api, conn := dialTestDaemon(t, addr)
	defer conn.Close()

	// Add a memory
	resp, err := api.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "environment",
		Key:         "test-key",
		Content:     "test content",
		Priority:    3,
		SourceAgent: "cli-test",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if resp.Entry.Key != "test-key" {
		t.Errorf("expected key=test-key, got %s", resp.Entry.Key)
	}

	// Get it back
	entry, err := api.GetMemory(ctx, &pb.GetMemoryRequest{Id: resp.Entry.Id})
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if entry.Key != "test-key" {
		t.Errorf("expected key=test-key, got %s", entry.Key)
	}
}

func TestCLISearchAndList(t *testing.T) {
	addr, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api, conn := dialTestDaemon(t, addr)
	defer conn.Close()

	// Add memories
	for i := range 3 {
		_, err := api.AddMemory(ctx, &pb.AddMemoryRequest{
			Target:      "memory",
			Category:    "observation",
			Key:         fmt.Sprintf("obs-%d", i),
			Content:     fmt.Sprintf("observation content %d", i),
			Priority:    3,
			SourceAgent: "cli-test",
		})
		if err != nil {
			t.Fatalf("AddMemory %d: %v", i, err)
		}
	}

	// List all memories
	listResp, err := api.ListMemories(ctx, &pb.ListMemoriesRequest{Target: "memory"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(listResp.Entries) < 3 {
		t.Errorf("expected at least 3 memories, got %d", len(listResp.Entries))
	}

	// Search
	searchResp, err := api.Search(ctx, &pb.SearchRequest{Query: "observation", Target: "memory"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(searchResp.Entries) < 3 {
		t.Errorf("expected at least 3 search results, got %d", len(searchResp.Entries))
	}
}

func TestCLIHotBlock(t *testing.T) {
	addr, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api, conn := dialTestDaemon(t, addr)
	defer conn.Close()

	// Add a memory
	_, err := api.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "environment",
		Key:         "project-dir",
		Content:     "Project directory is /home/user/project",
		Priority:    3,
		SourceAgent: "cli-test",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	// Get hot block
	hbResp, err := api.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    "memory",
		AgentId:   "cli-test",
		CharLimit: 3000,
	})
	if err != nil {
		t.Fatalf("GetHotBlock: %v", err)
	}
	if hbResp.Content == "" {
		t.Error("expected non-empty hot block content")
	}
	if !strings.Contains(hbResp.Content, "project-dir") {
		t.Errorf("expected hot block to contain 'project-dir', got: %s", hbResp.Content)
	}
}

func TestCLIRemoveMemory(t *testing.T) {
	addr, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api, conn := dialTestDaemon(t, addr)
	defer conn.Close()

	// Add and remove
	resp, err := api.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "environment",
		Key:         "to-delete",
		Content:     "will be deleted",
		Priority:    3,
		SourceAgent: "cli-test",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	_, err = api.RemoveMemory(ctx, &pb.RemoveMemoryRequest{Id: resp.Entry.Id})
	if err != nil {
		t.Fatalf("RemoveMemory: %v", err)
	}

	_, err = api.GetMemory(ctx, &pb.GetMemoryRequest{Id: resp.Entry.Id})
	if err == nil {
		t.Error("expected error getting removed memory")
	}
}

func TestCLIConsolidate(t *testing.T) {
	addr, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api, conn := dialTestDaemon(t, addr)
	defer conn.Close()

	// Consolidate on empty DB should work
	resp, err := api.Consolidate(ctx, &pb.ConsolidateRequest{
		Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED,
	})
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if resp.Removed > 0 {
		t.Errorf("expected 0 removed on empty DB, got %d", resp.Removed)
	}
}