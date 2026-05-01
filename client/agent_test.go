package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
)

// mockAgentAPI implements AgentAPI for testing.
type mockAgentAPI struct {
	mu           sync.Mutex
	registeredID string
	heartbeatN   int
	closed       bool

	// Customizable behavior
	registerFunc  func(ctx context.Context, agentID, name, role string) (string, bool, error)
	heartbeatFunc func(ctx context.Context, agentID string) error
	subscribeFunc func(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error)
}

func (m *mockAgentAPI) RegisterAgent(ctx context.Context, agentID, name, role string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registerFunc != nil {
		return m.registerFunc(ctx, agentID, name, role)
	}
	m.registeredID = agentID
	return agentID, true, nil
}

func (m *mockAgentAPI) Heartbeat(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.heartbeatN++
	if m.heartbeatFunc != nil {
		return m.heartbeatFunc(ctx, agentID)
	}
	return nil
}

func (m *mockAgentAPI) Subscribe(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subscribeFunc != nil {
		return m.subscribeFunc(ctx, req)
	}
	return nil, errors.New("subscribe not implemented in mock")
}

// mockSubscribeStream is a mock for pb.Mnemosyne_SubscribeClient.
type mockSubscribeStream struct {
	grpc.ClientStream
	events []*pb.UpdateEvent
	index  int
	ctx    context.Context // respected by Recv for cancellation
}

func (m *mockSubscribeStream) Recv() (*pb.UpdateEvent, error) {
	if m.index >= len(m.events) {
		// Wait for context cancellation (simulates a long-lived stream)
		if m.ctx != nil {
			<-m.ctx.Done()
			return nil, m.ctx.Err()
		}
		select {} // block forever if no context
	}
	evt := m.events[m.index]
	m.index++
	return evt, nil
}

func (m *mockSubscribeStream) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *mockSubscribeStream) CloseSend() error { return nil }

func TestAgentSession_Register(t *testing.T) {
	api := &mockAgentAPI{}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if as.AgentID() != "test-agent" {
		t.Errorf("Expected agent ID 'test-agent', got %s", as.AgentID())
	}
	if !as.IsRegistered() {
		t.Error("Expected IsRegistered() to be true")
	}

	// Cleanup
	as.Close()
}

func TestAgentSession_RegisterGeneratesID(t *testing.T) {
	api := &mockAgentAPI{}
	config := AgentConfig{ID: "", Name: "TestAgent", Role: "dev"} // empty ID
	as := NewAgentSession(api, config)

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got := as.AgentID()
	if got == "" {
		t.Error("Expected auto-generated agent ID, got empty string")
	}
	if len(got) < 6 {
		t.Errorf("Auto-generated ID seems too short: %s", got)
	}

	as.Close()
}

func TestAgentSession_RegisterTwice(t *testing.T) {
	api := &mockAgentAPI{}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("First Register failed: %v", err)
	}

	err := as.Register(context.Background())
	if err == nil {
		t.Fatal("Expected error when registering twice, got nil")
	}

	as.Close()
}

func TestAgentSession_SubscribeBeforeRegister(t *testing.T) {
	api := &mockAgentAPI{}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	_, err := as.Subscribe(context.Background(), []string{"memory"}, []string{"project"})
	if err == nil {
		t.Fatal("Expected error when subscribing before registering, got nil")
	}
}

func TestAgentSession_Subscribe(t *testing.T) {
	subCtx := context.Background()
	api := &mockAgentAPI{
		subscribeFunc: func(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
			return &mockSubscribeStream{
				ctx: subCtx,
				events: []*pb.UpdateEvent{
					{EventType: pb.UpdateEvent_ADDED, Target: "memory", Key: "test-key"},
				},
			}, nil
		},
	}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	eventCh, err := as.Subscribe(context.Background(), []string{"memory"}, []string{"project"})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Should receive the mock event
	select {
	case event := <-eventCh:
		if event.Target != "memory" {
			t.Errorf("Expected target 'memory', got %s", event.Target)
		}
		if event.Key != "test-key" {
			t.Errorf("Expected key 'test-key', got %s", event.Key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}

	as.Close()
}

func TestAgentSession_Close(t *testing.T) {
	api := &mockAgentAPI{}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)
	as.HeartbeatInterval = 100 * time.Millisecond

	if err := as.Register(context.Background()); err != nil {
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

func TestAgentSession_AgentID(t *testing.T) {
	api := &mockAgentAPI{}
	config := AgentConfig{ID: "my-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	// AgentID is available even before Register (it's from config)
	if as.AgentID() != "my-agent" {
		t.Errorf("Expected 'my-agent', got %s", as.AgentID())
	}

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// After register, should still return the ID
	if as.AgentID() != "my-agent" {
		t.Errorf("Expected 'my-agent' after register, got %s", as.AgentID())
	}

	as.Close()
}

func TestAgentSession_HeartbeatFails(t *testing.T) {
	callCount := 0
	api := &mockAgentAPI{
		heartbeatFunc: func(ctx context.Context, agentID string) error {
			callCount++
			return errors.New("daemon unreachable")
		},
	}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)
	as.HeartbeatInterval = 50 * time.Millisecond

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Wait for a couple of heartbeat intervals
	time.Sleep(200 * time.Millisecond)
	as.Close()

	// Heartbeats should have been attempted even though they failed
	api.mu.Lock()
	n := api.heartbeatN
	api.mu.Unlock()
	if n == 0 {
		t.Error("Expected heartbeat attempts even on failure")
	}
}

func TestAgentSession_SubscribeCancel(t *testing.T) {
	subCtx, subCancel := context.WithCancel(context.Background())
	api := &mockAgentAPI{
		subscribeFunc: func(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
			// Return a stream that blocks until context is cancelled
			return &mockSubscribeStream{ctx: subCtx}, nil
		},
	}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	eventCh, err := as.Subscribe(ctx, []string{"memory"}, nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Cancel the subscription context, which should unblock the mock stream
	cancel()
	subCancel() // also cancel the mock stream's context

	// The event channel should close after cancel
	select {
	case _, ok := <-eventCh:
		if ok {
			t.Error("Expected channel to be closed after cancel")
		}
		// Channel closed — success
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event channel to close after cancel")
	}

	as.Close()
}