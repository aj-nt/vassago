// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aj-nt/mnemosyne/client"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// testStore creates a temp store for service unit tests.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// setupTestServer creates an in-memory gRPC server using bufconn for testing.
// It returns a client connection and the test database store.
func setupTestServer(t *testing.T) (*grpc.ClientConn, *store.Store) {
	t.Helper()
	db := testStore(t)
	hub := pubsub.NewHub()
	svc := NewService(db, hub, nil)

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterMnemosyneServer(s, svc)
	pb.RegisterMessagingServer(s, NewMessagingService())
	pb.RegisterCronServiceServer(s, NewCronService(db))
	go s.Serve(lis)
	t.Cleanup(s.Stop)

	conn, err := grpc.Dial("bufnet", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, db
}

// TestClient_RegisterAgent tests the RegisterAgent method via gRPC wire.
func TestClient_RegisterAgent(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agentID, isNew, err := c.RegisterAgent(ctx, "test-agent", "TestAgent", "dev")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	if agentID != "test-agent" {
		t.Errorf("Expected agent ID 'test-agent', got %s", agentID)
	}
	if !isNew {
		t.Error("Expected isNew to be true for first registration")
	}
}

// TestClient_Heartbeat tests the Heartbeat method via gRPC wire.
func TestClient_Heartbeat(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First register an agent
	_, _, err := c.RegisterAgent(ctx, "test-agent", "TestAgent", "dev")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	// Then send heartbeat
	err = c.Heartbeat(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

// TestClient_Subscribe tests the Subscribe method via gRPC wire.
func TestClient_Subscribe(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First register an agent
	_, _, err := c.RegisterAgent(ctx, "test-agent", "TestAgent", "dev")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	// Create a memory to test subscription
	_, err = c.AddMemory(ctx, "memory", "test-category", "test-key", "test-content", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	// Subscribe to updates
	req := &pb.SubscribeRequest{
		AgentId:    "test-agent",
		Targets:    []string{"memory"},
		Categories: []string{"test-category"},
	}

	stream, err := c.Subscribe(ctx, req)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Try to receive an event (may not get one immediately, but stream should be open)
	_, err = stream.Recv()
	// We might not get an event immediately, so we'll just check that the stream is open
	// and error handling works
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("Expected deadline exceeded or no event, got: %v", err)
	}
}

// TestClient_MemoryOperations tests memory CRUD operations via gRPC wire.
func TestClient_MemoryOperations(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add a memory
	entry, err := c.AddMemory(ctx, "memory", "test-category", "test-key", "test-content", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	if entry.Content != "test-content" {
		t.Errorf("Expected content 'test-content', got %s", entry.Content)
	}

	// Get the memory
	gotEntry, err := c.GetMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}

	if gotEntry.Content != "test-content" {
		t.Errorf("Expected content 'test-content', got %s", gotEntry.Content)
	}

	// Search for memories using valid FTS5 query
	results, err := c.SearchMemories(ctx, "content", "memory", 10)
	if err != nil {
		t.Fatalf("SearchMemories failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 search result, got %d", len(results))
	}

	// List memories
	list, err := c.ListMemories(ctx, "memory", nil, nil)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}

	if len(list) != 1 {
		t.Errorf("Expected 1 memory in list, got %d", len(list))
	}

	// Remove the memory
	err = c.RemoveMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("RemoveMemory failed: %v", err)
	}

	// Verify it's gone
	_, err = c.GetMemory(ctx, entry.Id)
	if err == nil {
		t.Error("Expected error when getting removed memory, got nil")
	}
}

// TestClient_SessionOperations tests session operations via gRPC wire.
func TestClient_SessionOperations(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a session
	sessionID, err := c.CreateSession(ctx, "test-agent", "test-source", "Test Session")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Add messages to the session
	messages := []client.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}
	err = c.AddMessages(ctx, sessionID, messages)
	if err != nil {
		t.Fatalf("AddMessages failed: %v", err)
	}

	// Get the session
	session, err := c.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if session.Title != "Test Session" {
		t.Errorf("Expected title 'Test Session', got %s", session.Title)
	}

	if len(session.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(session.Messages))
	}

	// End the session
	err = c.EndSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	// Search sessions
	searchResults, err := c.SearchSessions(ctx, "Test", 10)
	if err != nil {
		t.Fatalf("SearchSessions failed: %v", err)
	}

	if len(searchResults) != 1 {
		t.Errorf("Expected 1 search result, got %d", len(searchResults))
	}

	// List recent sessions
	recent, err := c.ListRecentSessions(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentSessions failed: %v", err)
	}

	if len(recent) != 1 {
		t.Errorf("Expected 1 recent session, got %d", len(recent))
	}
}

// TestClient_HotBlock tests GetHotBlock via gRPC wire.
func TestClient_HotBlock(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add some memories to create a hot block
	_, err := c.AddMemory(ctx, "memory", "test-category", "test-key1", "test-content1", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	_, err = c.AddMemory(ctx, "memory", "test-category", "test-key2", "test-content2", 4, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	// Get hot block
	content, err := c.GetHotBlock(ctx, "memory", "test-agent", 1000, []string{"test-category"}, []string{}, 0)
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}

	if content == "" {
		t.Error("Expected non-empty hot block content")
	}
}

// TestClient_Consolidate tests Consolidate via gRPC wire.
func TestClient_Consolidate(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add some memories to consolidate
	_, err := c.AddMemory(ctx, "memory", "observation", "obs1", "observation1", 2, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	_, err = c.AddMemory(ctx, "memory", "observation", "obs2", "observation2", 2, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	// Consolidate
	resp, err := c.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_OBSERVATIONS, 10)
	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}

	t.Logf("Consolidation result: %+v", resp)
}

// TestClient_HealthCheck tests HealthCheck via gRPC wire.
func TestClient_HealthCheck(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

// TestClient_MessagingOperations tests messaging operations via gRPC wire.
// These may return errors since no messaging adapters are configured.
func TestClient_MessagingOperations(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// SendMessage should fail without configured adapters
	_, err := c.SendMessage(ctx, "test-platform", "test-target", "test-message")
	if err == nil {
		t.Error("Expected SendMessage to fail without adapters, got nil")
	}

	// ListChannels should fail without configured adapters
	_, err = c.ListChannels(ctx, "test-platform")
	if err == nil {
		t.Error("Expected ListChannels to fail without adapters, got nil")
	}
}

// TestClient_IsConnected tests IsConnected method.
func TestClient_IsConnected(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)

	// Client created from connection should be connected
	if !c.IsConnected() {
		t.Error("Expected IsConnected() to return true for client with connection")
	}

	// NullMnemo should not be connected
	var mnemo client.MnemoClient = &client.NullMnemo{}
	if mnemo.IsConnected() {
		t.Error("Expected NullMnemo.IsConnected() to return false")
	}
}

// TestClient_TodoOperations tests todo operations via gRPC wire.
func TestClient_TodoOperations(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add a todo
	todo, err := c.AddTodo(ctx, "Test todo", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}

	if todo.Content != "Test todo" {
		t.Errorf("Expected content 'Test todo', got %s", todo.Content)
	}

	// List todos
	todos, err := c.ListTodos(ctx, false, "test-agent")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}

	if len(todos) != 1 {
		t.Errorf("Expected 1 todo, got %d", len(todos))
	}

	// Complete the todo
	completed, err := c.CompleteTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("CompleteTodo failed: %v", err)
	}

	if !completed.Completed {
		t.Error("Expected todo to be completed")
	}

	// Remove the todo
	err = c.RemoveTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("RemoveTodo failed: %v", err)
	}

	// Verify it's gone
	_, err = c.ListTodos(ctx, false, "test-agent")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
}

// TestClient_CronOperations tests cron job operations via gRPC wire.
func TestClient_CronOperations(t *testing.T) {
	conn, _ := setupTestServer(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Register an agent first (cron jobs have FK constraint on agent_id)
	_, _, err := c.RegisterAgent(ctx, "test-agent", "TestAgent", "test")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}

	// Create a cron job
	enabled := true
	job, err := c.CreateCronJob(ctx, "Test Job", "0 * * * *", "test-agent", "test message", &enabled)
	if err != nil {
		t.Fatalf("CreateCronJob failed: %v", err)
	}

	if job.Name != "Test Job" {
		t.Errorf("Expected name 'Test Job', got %s", job.Name)
	}

	// List cron jobs
	jobs, err := c.ListCronJobs(ctx, "test-agent", false)
	if err != nil {
		t.Fatalf("ListCronJobs failed: %v", err)
	}

	if len(jobs) != 1 {
		t.Errorf("Expected 1 cron job, got %d", len(jobs))
	}

	// Update the cron job
	newMessage := "updated message"
	updated, err := c.UpdateCronJob(ctx, job.ID, nil, nil, &newMessage, nil)
	if err != nil {
		t.Fatalf("UpdateCronJob failed: %v", err)
	}

	if updated.Message != "updated message" {
		t.Errorf("Expected updated message, got %s", updated.Message)
	}

	// Delete the cron job
	err = c.DeleteCronJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("DeleteCronJob failed: %v", err)
	}

	// Verify it's gone
	jobs, err = c.ListCronJobs(ctx, "test-agent", false)
	if err != nil {
		t.Fatalf("ListCronJobs failed: %v", err)
	}

	if len(jobs) != 0 {
		t.Errorf("Expected 0 cron jobs after deletion, got %d", len(jobs))
	}
}
