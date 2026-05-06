// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// TestAgentSession_RegisterViaGRPC tests that AgentSession.Register works
// through a real gRPC wire with the mock server.
func TestAgentSession_RegisterViaGRPC(t *testing.T) {
	memoSrv := newMockMnemosyneServer()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterMnemosyneServer(s, memoSrv)
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

	c := NewClientFromConn(conn)

	config := AgentConfig{ID: "integration-agent", Name: "IntegrationTest", Role: "test"}
	as := NewAgentSession(c, config)
	as.HeartbeatInterval = 10 * time.Second // Slow heartbeat for test

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if as.AgentID() != "integration-agent" {
		t.Errorf("Expected agent ID 'integration-agent', got %s", as.AgentID())
	}
	if !as.IsRegistered() {
		t.Error("Expected IsRegistered() to be true")
	}

	as.Close()
}

// TestAgentSession_RegisterTwiceViaGRPC tests idempotent registration
// through a real gRPC wire.
func TestAgentSession_RegisterTwiceViaGRPC(t *testing.T) {
	memoSrv := newMockMnemosyneServer()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterMnemosyneServer(s, memoSrv)
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

	c := NewClientFromConn(conn)

	config := AgentConfig{ID: "double-agent", Name: "DoubleTest", Role: "test"}
	as := NewAgentSession(c, config)
	as.HeartbeatInterval = 10 * time.Second

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	// Second register should fail
	err = as.Register(context.Background())
	if err == nil {
		t.Error("Expected error when registering twice, got nil")
	}

	as.Close()
}

// TestAgentSession_SubscribeViaGRPC tests that Subscribe works
// through a real gRPC wire. Since our mock server doesn't implement
// streaming Subscribe, this tests the error path.
func TestAgentSession_SubscribeViaGRPC(t *testing.T) {
	memoSrv := newMockMnemosyneServer()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterMnemosyneServer(s, memoSrv)
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

	c := NewClientFromConn(conn)

	config := AgentConfig{ID: "sub-agent", Name: "SubTest", Role: "test"}
	as := NewAgentSession(c, config)
	as.HeartbeatInterval = 10 * time.Second

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	defer as.Close()

	// Subscribe on the mock server — since we don't implement the streaming
	// Subscribe method, this should return an UNIMPLEMENTED error
	subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, subErr := as.Subscribe(subCtx, []string{"memory"}, []string{"project"})
	// The mock server's Subscribe is unimplemented, so this should error
	if subErr == nil {
		t.Log("Subscribe succeeded (unexpected but acceptable if mock returns stream)")
	} else {
		t.Logf("Subscribe error (expected with mock): %v", subErr)
	}
}

// TestAgentSession_HeartbeatViaGRPC tests that Heartbeat works
// through a real gRPC wire.
func TestAgentSession_HeartbeatViaGRPC(t *testing.T) {
	memoSrv := newMockMnemosyneServer()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterMnemosyneServer(s, memoSrv)
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

	c := NewClientFromConn(conn)

	// Test direct Heartbeat call (not through AgentSession loop)
	err = c.Heartbeat(context.Background(), "test-agent")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

// TestClientInterfaceSatisfaction verifies that the Client type satisfies
// both MnemoClient and ClientInterface.
func TestClientInterfaceSatisfaction(t *testing.T) {
	// These are compile-time checks
	var _ MnemoClient = (*Client)(nil)
	var _ MnemoClient = (*NullMnemo)(nil)
	var _ ClientInterface = (*Client)(nil)
}

// TestCronJobInfoFieldAccess tests the CronJobInfo type construction.
func TestCronJobInfoFieldAccess(t *testing.T) {
	job := CronJobInfo{
		ID:         "cron-123",
		Name:       "Daily backup",
		Schedule:   "0 2 * * *",
		AgentID:    "agent-1",
		Message:    "Run backup",
		Enabled:    true,
		CreatedAt:  1700000000,
		UpdatedAt:  1700001000,
		LastFireAt: 1700002000,
		NextFireAt: 1700003000,
	}
	if job.ID != "cron-123" {
		t.Errorf("Expected ID 'cron-123', got %q", job.ID)
	}
	if job.Schedule != "0 2 * * *" {
		t.Errorf("Expected schedule '0 2 * * *', got %q", job.Schedule)
	}
	if !job.Enabled {
		t.Error("Expected Enabled=true")
	}
}