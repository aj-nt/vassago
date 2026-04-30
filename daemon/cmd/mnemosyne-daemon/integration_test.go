package main

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/daemon"
	daemongrpc "github.com/aj-nt/mnemosyne/daemon/internal/grpc"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startTestDaemon creates a daemon with a temp DB and starts a gRPC server
// on a random TCP port. Returns the daemon, client, and a cleanup function.
func startTestDaemon(t *testing.T) (*daemon.Daemon, pb.MnemosyneClient, func()) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	cfg := &daemon.Config{
		Daemon: daemon.DaemonConfig{
			Socket: "", // No Unix socket
			TCP:    "127.0.0.1:0", // Will be overridden by listener
		},
		Store: daemon.StoreConfig{
			Path:        dbPath,
			WALMode:     true,
			BusyTimeout: 5000,
		},
		HotBlock: daemon.HotBlockConfig{
			MemoryCharLimit:    4000,
			UserCharLimit:      2000,
			MinPriority:        2,
			ObservationTTLDays: 30,
		},
	}

	d, err := daemon.New(cfg)
	if err != nil {
		t.Fatalf("create daemon: %v", err)
	}

	// Listen on random TCP port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() { _ = d.Server().Serve(lis) }()

	// Connect client
	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		d.Server().GracefulStop()
		t.Fatalf("dial: %v", err)
	}

	client := pb.NewMnemosyneClient(conn)
	cleanup := func() {
		conn.Close()
		d.Server().GracefulStop()
		d.Store().Close()
	}

	return d, client, cleanup
}

func TestIntegration_DaemonRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add memory
	addResp, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
		Target: "memory", Category: "environment", Key: "integration_test",
		Content: "Integration test content", Priority: 3, SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if addResp.Entry.Id == "" {
		t.Fatal("empty entry ID")
	}

	// Get memory
	getResp, err := client.GetMemory(ctx, &pb.GetMemoryRequest{Id: addResp.Entry.Id})
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if getResp.Content != "Integration test content" {
		t.Errorf("content = %q, want %q", getResp.Content, "Integration test content")
	}

	// Get hot block
	hbResp, err := client.GetHotBlock(ctx, &pb.HotBlockRequest{Target: "memory", CharLimit: 4000})
	if err != nil {
		t.Fatalf("GetHotBlock: %v", err)
	}
	if hbResp.EntryCount == 0 {
		t.Error("hot block should have entries")
	}

	// Search
	searchResp, err := client.Search(ctx, &pb.SearchRequest{Query: "integration", Target: "memory"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(searchResp.Entries) == 0 {
		t.Error("search should find the integration test memory")
	}

	// Update memory
	newContent := "Updated integration test content"
	updateResp, err := client.UpdateMemory(ctx, &pb.UpdateMemoryRequest{Id: addResp.Entry.Id, Content: &newContent})
	if err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}
	if updateResp.Content != "Updated integration test content" {
		t.Errorf("content = %q, want updated", updateResp.Content)
	}

	// Remove memory
	rmResp, err := client.RemoveMemory(ctx, &pb.RemoveMemoryRequest{Id: addResp.Entry.Id})
	if err != nil {
		t.Fatalf("RemoveMemory: %v", err)
	}
	if !rmResp.Deleted {
		t.Error("expected deleted = true")
	}

	// Verify it's gone
	_, err = client.GetMemory(ctx, &pb.GetMemoryRequest{Id: addResp.Entry.Id})
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestIntegration_MalformedRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Empty target
	_, err := client.AddMemory(ctx, &pb.AddMemoryRequest{Target: "", Key: "k", SourceAgent: "a"})
	if err == nil {
		t.Error("expected error for empty target")
	}

	// Empty ID
	_, err = client.GetMemory(ctx, &pb.GetMemoryRequest{Id: ""})
	if err == nil {
		t.Error("expected error for empty ID")
	}

	// Empty query
	_, err = client.Search(ctx, &pb.SearchRequest{Query: ""})
	if err == nil {
		t.Error("expected error for empty search query")
	}

	// Empty agent_id for session
	_, err = client.CreateSession(ctx, &pb.CreateSessionRequest{AgentId: ""})
	if err == nil {
		t.Error("expected error for empty agent_id")
	}

	// Hot block on empty database should not panic or error
	hb, err := client.GetHotBlock(ctx, &pb.HotBlockRequest{Target: "memory", CharLimit: 4000})
	if err != nil {
		t.Errorf("GetHotBlock on empty DB should not error: %v", err)
	}
	if hb.EntryCount != 0 {
		t.Errorf("expected 0 entries, got %d", hb.EntryCount)
	}
}

func TestIntegration_ConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx := context.Background()

	// Write 10 memories concurrently with the same key (upsert)
	done := make(chan error, 10)
	for i := range 10 {
		go func(idx int) {
			_, err := client.AddMemory(ctx, &pb.AddMemoryRequest{
				Target: "memory", Category: "concurrent", Key: "concurrent_test",
				Content: "concurrent write", Priority: int32(idx%5 + 1), SourceAgent: "concurrent-tester",
			})
			done <- err
		}(i)
	}

	errors := 0
	for range 10 {
		if err := <-done; err != nil {
			t.Logf("concurrent write error: %v", err)
			errors++
		}
	}

	if errors > 0 {
		t.Errorf("expected 0 errors from concurrent writes, got %d", errors)
	}

	// Verify only one entry exists (upsert)
	listResp, err := client.ListMemories(ctx, &pb.ListMemoriesRequest{Target: "memory"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	count := 0
	for _, e := range listResp.Entries {
		if e.Key == "concurrent_test" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 entry with key 'concurrent_test', got %d", count)
	}
}

// Service wiring unit test (no network)
func TestUnit_ServiceWiring(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	svc := daemongrpc.NewService(st, nil)
	resp, err := svc.AddMemory(context.Background(), &pb.AddMemoryRequest{
		Target: "memory", Category: "test", Key: "wiring",
		Content: "wiring test", Priority: 3, SourceAgent: "tester",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if resp.Entry.Id == "" {
		t.Error("expected non-empty entry ID")
	}
}

func TestIntegration_SessionRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create session
	sess, err := client.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "test-agent",
		Source:  "cli",
		Title:   "Integration test session",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.Id == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Add messages
	msgs := []*pb.Message{
		{Role: "user", Content: "Hello from integration test"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "user", Content: "Goodbye"},
	}
	_, err = client.AddMessages(ctx, &pb.AddMessagesRequest{
		SessionId: sess.Id,
		Messages:  msgs,
	})
	if err != nil {
		t.Fatalf("AddMessages: %v", err)
	}

	// Get session with messages
	includeMsgs := true
	gotSess, err := client.GetSession(ctx, &pb.GetSessionRequest{
		SessionId:       sess.Id,
		IncludeMessages:  &includeMsgs,
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if len(gotSess.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(gotSess.Messages))
	}
	if gotSess.Messages[0].Content != "Hello from integration test" {
		t.Errorf("first message = %q, want 'Hello from integration test'", gotSess.Messages[0].Content)
	}
	if gotSess.Messages[2].Role != "user" {
		t.Errorf("third message role = %q, want 'user'", gotSess.Messages[2].Role)
	}
}

func TestIntegration_DaemonHealthCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, client, cleanup := startTestDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping the daemon
	resp, err := client.Ping(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("Ping status = %q, want 'healthy'", resp.Status)
	}
}