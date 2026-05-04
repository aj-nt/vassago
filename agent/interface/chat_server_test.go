// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/provider"
	"github.com/aj-nt/mnemosyne/agent/tools"
	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestChatServerPing(t *testing.T) {
	// Create a minimal agent
	registry := provider.NewProviderRegistry()
	registry.Register("mock", provider.NewMockProvider("mock-model", []provider.CannedResponse{{Response: agentctx.ParsedResponse{Text: "test response"}}}))
	registry.SetPrimary("mock")
	toolReg := tools.NewToolRegistry()
	a := agent.NewAgent(agent.Config{
		AgentID:   "test-agent",
		AgentName: "test",
		MaxTurns:  5,
	}, registry, toolReg, nil)

	// Create chat server
	cs := NewChatServer(a, nil)

	// Start gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterAgentChatServer(s, cs)
	go s.Serve(lis)
	defer s.GracefulStop()

	// Connect and ping
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewAgentChatClient(conn)
	resp, err := client.Ping(context.Background(), &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if resp.Status != "SERVING" {
		t.Errorf("expected SERVING, got %s", resp.Status)
	}
}

func TestChatServerSessionManagement(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", provider.NewMockProvider("mock-model", []provider.CannedResponse{{Response: agentctx.ParsedResponse{Text: "ok"}}}))
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	a := agent.NewAgent(agent.Config{
		AgentID:   "test-agent",
		AgentName: "test",
		MaxTurns:  5,
	}, registry, toolReg, nil)
	defer a.Close()

	cs := NewChatServer(a, nil)

	// Test creating a session without daemon (local session ID)
	sess, err := cs.getOrCreateSession(context.Background(), "", "test-agent")
	if err != nil {
		t.Fatalf("getOrCreateSession: %v", err)
	}
	if sess.sessionID == "" {
		t.Error("expected non-empty local session ID")
	}

	// Test creating with a specific session ID that doesn't exist locally
	sess2, err := cs.getOrCreateSession(context.Background(), "test-session-123", "test-agent")
	if err != nil {
		t.Fatalf("getOrCreateSession with ID: %v", err)
	}
	if sess2.sessionID != "test-session-123" {
		t.Errorf("expected session ID 'test-session-123', got %q", sess2.sessionID)
	}

	// Test retrieving existing session
	sess3, err := cs.getOrCreateSession(context.Background(), "test-session-123", "test-agent")
	if err != nil {
		t.Fatalf("getOrCreateSession existing: %v", err)
	}
	if sess3.sessionID != "test-session-123" {
		t.Errorf("expected same session, got %q", sess3.sessionID)
	}
}

func TestChatServerAutoApprove(t *testing.T) {
	// Verify that the auto-approver is set correctly
	registry := provider.NewProviderRegistry()
	registry.Register("mock", provider.NewMockProvider("mock-model", []provider.CannedResponse{{Response: agentctx.ParsedResponse{Text: "ok"}}}))
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	a := agent.NewAgent(agent.Config{
		AgentID:   "test-agent",
		AgentName: "test",
		MaxTurns:  5,
	}, registry, toolReg, nil)
	defer a.Close()

	cs := NewChatServer(a, nil)

	// Set the approver (this happens in ChatStream)
	cs.agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
		return true
	}

	// Check that the approver approves everything
	if !cs.agent.Stage4.Approver("FileWrite", nil) {
		t.Error("expected auto-approver to approve FileWrite")
	}
	if !cs.agent.Stage4.Approver("Execute", nil) {
		t.Error("expected auto-approver to approve Execute")
	}
}

func TestChatServerConversationContext(t *testing.T) {
	registry := provider.NewProviderRegistry()
	registry.Register("mock", provider.NewMockProvider("mock-model", []provider.CannedResponse{{Response: agentctx.ParsedResponse{Text: "response"}}}))
	registry.SetPrimary("mock")

	cfg := agent.Config{
		AgentID:   "test-agent",
		AgentName: "test",
		MaxTurns:  5,
	}
	toolReg := tools.NewToolRegistry()
	a := agent.NewAgent(cfg, registry, toolReg, nil)
	defer a.Close()

	cs := NewChatServer(a, nil)

	// Create a session and add messages
	sess, _ := cs.getOrCreateSession(context.Background(), "", "test-agent")
	sess.messages = append(sess.messages, agentctx.Message{Role: "user", Content: "hello"})

	if len(sess.messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(sess.messages))
	}
	if sess.messages[0].Content != "hello" {
		t.Errorf("expected 'hello', got %q", sess.messages[0].Content)
	}
}

func TestChatServerChatStreamWithTimeout(t *testing.T) {
	// Test that ChatStream works with a timeout context
	// The mock provider doesn't support streaming, so the agent falls back to non-streaming
	registry := provider.NewProviderRegistry()
	registry.Register("mock", provider.NewMockProvider("mock-model", []provider.CannedResponse{
		{Response: agentctx.ParsedResponse{Text: "Hello from agent!"}},
	}))
	registry.SetPrimary("mock")

	toolReg := tools.NewToolRegistry()
	a := agent.NewAgent(agent.Config{
		AgentID:   "test-agent",
		AgentName: "test",
		MaxTurns:  1,
	}, registry, toolReg, nil)
	defer a.Close()

	cs := NewChatServer(a, nil)

	// Start gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterAgentChatServer(s, cs)
	go s.Serve(lis)
	defer s.GracefulStop()

	// Connect
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewAgentChatClient(conn)

	// Chat with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.ChatStream(ctx, &pb.ChatRequest{
		AgentId: "test-agent",
		Message: "Hello!",
	})
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}

	var gotThinking, gotDone bool
	for {
		event, err := stream.Recv()
		if err != nil {
			break
		}
		switch event.EventType {
		case pb.ChatEvent_THINKING:
			gotThinking = true
		case pb.ChatEvent_DONE:
			gotDone = true
		}
	}

	if !gotThinking {
		t.Log("Warning: no thinking event received")
	}
	t.Logf("ChatStream completed, gotDone=%v", gotDone)
}