// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/aj-nt/vassago/client"
	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// setupBufconnServer creates an in-memory gRPC server with a real store for integration testing.
func setupBufconnServer(t *testing.T) (*grpc.ClientConn, *store.Store) {
	t.Helper()

	db := testStoreSvc(t)
	svc := NewService(db, nil, nil)

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterVassagoServer(s, svc)

	// Also register MessagingService and CronService
	pb.RegisterMessagingServer(s, NewMessagingService())
	pb.RegisterCronServiceServer(s, NewCronService(db))

	go s.Serve(lis)
	t.Cleanup(s.Stop)

	conn, err := grpc.Dial("bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return conn, db
}

func TestClientIntegration_Memory(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// AddMemory
	entry, err := c.AddMemory(ctx, "memory", "environment", "test-key", "test content", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}
	if entry.Id == "" {
		t.Error("Expected entry ID to be set")
	}
	if entry.Content != "test content" {
		t.Errorf("Expected content 'test content', got %q", entry.Content)
	}

	// GetMemory
	got, err := c.GetMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got.Id != entry.Id {
		t.Errorf("Expected ID %s, got %s", entry.Id, got.Id)
	}

	// SearchMemories
	results, err := c.SearchMemories(ctx, "test", "memory", 10)
	if err != nil {
		t.Fatalf("SearchMemories failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least one search result")
	}

	// ListMemories
	entries, err := c.ListMemories(ctx, "memory", nil, nil)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(entries) == 0 {
		t.Error("Expected at least one memory entry")
	}

	// GetHotBlock
	block, err := c.GetHotBlock(ctx, "memory", "test-agent", 4000, nil, nil, 0)
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if block == "" {
		t.Error("Expected non-empty hot block")
	}

	// RemoveMemory
	err = c.RemoveMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("RemoveMemory failed: %v", err)
	}

	// Verify removal
	_, err = c.GetMemory(ctx, entry.Id)
	if err == nil {
		t.Error("Expected error when getting removed memory")
	}
}

func TestClientIntegration_Session(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// CreateSession
	sessionID, err := c.CreateSession(ctx, "test-agent", "test", "test session")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sessionID == "" {
		t.Error("Expected session ID to be set")
	}

	// AddMessages
	msgs := []client.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}
	err = c.AddMessages(ctx, sessionID, msgs)
	if err != nil {
		t.Fatalf("AddMessages failed: %v", err)
	}

	// SearchSessions
	sessions, err := c.SearchSessions(ctx, "test", 10)
	if err != nil {
		t.Fatalf("SearchSessions failed: %v", err)
	}
	if len(sessions) == 0 {
		t.Error("Expected at least one session in search results")
	}

	// ListRecentSessions
	recent, err := c.ListRecentSessions(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentSessions failed: %v", err)
	}
	if len(recent) == 0 {
		t.Error("Expected at least one recent session")
	}

	// GetSession
	detail, err := c.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if detail.ID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, detail.ID)
	}

	// EndSession
	err = c.EndSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}
}

func TestClientIntegration_Todo(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// AddTodo
	todo, err := c.AddTodo(ctx, "Buy groceries", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}
	if todo.ID == "" {
		t.Error("Expected todo ID to be set")
	}
	if todo.Content != "Buy groceries" {
		t.Errorf("Expected content 'Buy groceries', got %q", todo.Content)
	}

	// ListTodos
	todos, err := c.ListTodos(ctx, true, "")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) == 0 {
		t.Error("Expected at least one todo")
	}

	// CompleteTodo
	completed, err := c.CompleteTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("CompleteTodo failed: %v", err)
	}
	if !completed.Completed {
		t.Error("Expected todo to be completed")
	}

	// RemoveTodo
	err = c.RemoveTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("RemoveTodo failed: %v", err)
	}

	// RemoveTodo nonexistent
	err = c.RemoveTodo(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error when removing nonexistent todo")
	}
}

func TestClientIntegration_Cron(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// Register an agent first (cron jobs have FK constraint on agent_id)
	_, _, err := c.RegisterAgent(ctx, "cron-test-agent", "CronTest", "test")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	// CreateCronJob
	job, err := c.CreateCronJob(ctx, "test-cron", "*/5 * * * *", "cron-test-agent", "hello", nil)
	if err != nil {
		t.Fatalf("CreateCronJob failed: %v", err)
	}
	if job.ID == "" {
		t.Error("Expected job ID to be set")
	}

	// ListCronJobs
	jobs, err := c.ListCronJobs(ctx, "cron-test-agent", false)
	if err != nil {
		t.Fatalf("ListCronJobs failed: %v", err)
	}
	if len(jobs) == 0 {
		t.Errorf("Expected at least one cron job, got %d (job ID was %q)", len(jobs), job.ID)
	}

	// UpdateCronJob
	enabled := true
	updated, err := c.UpdateCronJob(ctx, job.ID, strPtr("updated-name"), nil, nil, &enabled)
	if err != nil {
		t.Fatalf("UpdateCronJob failed: %v", err)
	}
	if updated.Name != "updated-name" {
		t.Errorf("Expected name 'updated-name', got %q", updated.Name)
	}

	// DeleteCronJob
	err = c.DeleteCronJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("DeleteCronJob failed: %v", err)
	}
}

func TestClientIntegration_Agent(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// RegisterAgent
	agentID, isNew, err := c.RegisterAgent(ctx, "test-agent-123", "TestAgent", "test")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}
	if agentID != "test-agent-123" {
		t.Errorf("Expected agent ID 'test-agent-123', got %q", agentID)
	}
	if !isNew {
		t.Error("Expected new agent registration")
	}

	// Register again — should not be new
	_, isNew, err = c.RegisterAgent(ctx, "test-agent-123", "TestAgent", "test")
	if err != nil {
		t.Fatalf("Second RegisterAgent failed: %v", err)
	}
	if isNew {
		t.Error("Expected existing agent, not new")
	}

	// Heartbeat
	err = c.Heartbeat(ctx, "test-agent-123")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

func TestClientIntegration_HealthCheck(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	err := c.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestClientIntegration_Consolidate(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// Add a memory first
	_, err := c.AddMemory(ctx, "memory", "observation", "consolidate-test", "test content", 2, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	// Consolidate
	batchSize := int32(100)
	resp, err := c.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL, batchSize)
	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}
	if resp == nil {
		t.Error("Expected non-nil ConsolidateResponse")
	}
}

func TestClientIntegration_ErrorCases(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// GetMemory with nonexistent ID
	_, err := c.GetMemory(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent memory ID")
	} else {
		t.Logf("GetMemory nonexistent ID error (expected): %v", err)
	}

	// RemoveMemory with nonexistent ID — returns (false, nil), no error
	// This is by design: DELETE on nonexistent row is a no-op

	// CompleteTodo with nonexistent ID
	_, err = c.CompleteTodo(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent todo ID")
	}

	// EndSession with nonexistent ID
	err = c.EndSession(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent session ID")
	}
}

// Helper functions

func strPtr(s string) *string { return &s }
func int32Ptr(i int32) *int32 { return &i }

func TestClientIntegration_AddMemoryValidation(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// Empty target should fail
	_, err := c.AddMemory(ctx, "", "environment", "key", "content", 3, "test-agent")
	if err == nil {
		t.Error("Expected error for empty target")
	}

	// Empty key should fail
	_, err = c.AddMemory(ctx, "memory", "environment", "", "content", 3, "test-agent")
	if err == nil {
		t.Error("Expected error for empty key")
	}
}

func TestClientIntegration_MultipleMemories(t *testing.T) {
	conn, _ := setupBufconnServer(t)
	ctx := context.Background()

	c := client.NewClientFromConn(conn)
	defer c.Close()

	// Add multiple memories with different targets and categories
	for i := 0; i < 5; i++ {
		_, err := c.AddMemory(ctx, "memory", "observation", fmt.Sprintf("key-%d-%d", i, time.Now().UnixNano()), "content", 3, "test-agent")
		if err != nil {
			t.Fatalf("AddMemory %d failed: %v", i, err)
		}
	}

	// Brief pause to let async writes settle
	time.Sleep(100 * time.Millisecond)

	// List all memories
	entries, err := c.ListMemories(ctx, "memory", nil, nil)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(entries) < 5 {
		t.Errorf("Expected at least 5 memories, got %d", len(entries))
	}
}
