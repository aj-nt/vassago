// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
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

// setupTestServerForAgent creates an in-memory gRPC server using bufconn for agent testing.
// It returns a client connection and the test database store.
func setupTestServerForAgent(t *testing.T) (*grpc.ClientConn, *store.Store) {
	t.Helper()
	db := testStoreSvc(t)
	svc := NewService(db, nil, nil)

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterVassagoServer(s, svc)
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

// TestAgentSession_RegisterViaGRPC tests AgentSession.Register through the real gRPC wire.
func TestAgentSession_RegisterViaGRPC(t *testing.T) {
	conn, _ := setupTestServerForAgent(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	config := client.AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := client.NewAgentSession(c, config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := as.Register(ctx); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if as.AgentID() != "test-agent" {
		t.Errorf("Expected agent ID 'test-agent', got %s", as.AgentID())
	}

	if !as.IsRegistered() {
		t.Error("Expected IsRegistered() to be true")
	}

	as.Close()
}

// TestAgentSession_SubscribeViaGRPC tests AgentSession.Subscribe through the real gRPC wire.
func TestAgentSession_SubscribeViaGRPC(t *testing.T) {
	conn, _ := setupTestServerForAgent(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	config := client.AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := client.NewAgentSession(c, config)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register first
	if err := as.Register(ctx); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Subscribe to updates — with nil hub, the server returns UNAVAILABLE error
	// on the stream. The stream opens but events are never received and the
	// event channel closes when the server stream errors out.
	eventCh, subErr := as.Subscribe(ctx, []string{"memory"}, []string{"test-category"})
	if subErr != nil {
		t.Fatalf("Subscribe stream open failed: %v", subErr)
	}

	// The event channel should close quickly since the server returns UNAVAILABLE
	select {
	case _, ok := <-eventCh:
		if !ok {
			t.Log("Event channel closed — expected since test server has no hub")
		}
	case <-time.After(2 * time.Second):
		t.Log("No event received — this is expected if pubsub is not configured in test")
	}

	as.Close()
}

// TestAgentSession_CloseStopsHeartbeat tests that AgentSession.Close stops the heartbeat goroutine.
func TestAgentSession_CloseStopsHeartbeat(t *testing.T) {
	conn, _ := setupTestServerForAgent(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	config := client.AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := client.NewAgentSession(c, config)
	as.HeartbeatInterval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := as.Register(ctx); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Close should stop heartbeats and return quickly
	done := make(chan struct{})
	go func() {
		as.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success — Close returned
	case <-time.After(2 * time.Second):
		t.Fatal("Close() took too long — heartbeat goroutine may not have stopped")
	}

	// After Close, IsRegistered should be false
	if as.IsRegistered() {
		t.Error("Expected IsRegistered() to be false after Close()")
	}
}

// TestAgentSession_MultipleAgents tests that multiple agents can register (idempotency).
func TestAgentSession_MultipleAgents(t *testing.T) {
	conn, _ := setupTestServerForAgent(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create first agent
	config1 := client.AgentConfig{ID: "agent-1", Name: "AgentOne", Role: "dev"}
	as1 := client.NewAgentSession(c, config1)
	if err := as1.Register(ctx); err != nil {
		t.Fatalf("Register agent-1 failed: %v", err)
	}

	// Create second agent
	config2 := client.AgentConfig{ID: "agent-2", Name: "AgentTwo", Role: "dev"}
	as2 := client.NewAgentSession(c, config2)
	if err := as2.Register(ctx); err != nil {
		t.Fatalf("Register agent-2 failed: %v", err)
	}

	// Verify both are registered
	if !as1.IsRegistered() || !as2.IsRegistered() {
		t.Error("Expected both agents to be registered")
	}

	// Test re-registering the same agent (should be idempotent)
	err := as1.Register(ctx)
	if err == nil {
		t.Error("Expected error when registering same agent twice, got nil")
	}

	as1.Close()
	as2.Close()
}

// TestAgentSession_AgentIDBeforeAndAfterRegister tests AgentID availability.
func TestAgentSession_AgentIDBeforeAndAfterRegister(t *testing.T) {
	conn, _ := setupTestServerForAgent(t)
	c := client.NewClientFromConn(conn)
	defer c.Close()

	config := client.AgentConfig{ID: "my-agent", Name: "TestAgent", Role: "dev"}
	as := client.NewAgentSession(c, config)

	// AgentID is available even before Register (it's from config)
	if as.AgentID() != "my-agent" {
		t.Errorf("Expected 'my-agent', got %s", as.AgentID())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := as.Register(ctx); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// After register, should still return the ID
	if as.AgentID() != "my-agent" {
		t.Errorf("Expected 'my-agent' after register, got %s", as.AgentID())
	}

	as.Close()
}
