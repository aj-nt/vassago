// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
)

func TestAgentSession_OnEventCallback(t *testing.T) {
	var eventCount int32

	subCtx := context.Background()
	api := &mockAgentAPI{
		subscribeFunc: func(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
			return &mockSubscribeStream{
				ctx: subCtx,
				events: []*pb.UpdateEvent{
					{EventType: pb.UpdateEvent_ADDED, Target: "memory", Key: "test-key", AgentId: "other-agent"},
					{EventType: pb.UpdateEvent_UPDATED, Target: "memory", Key: "test-key", AgentId: "other-agent"},
				},
			}, nil
		},
	}
	config := AgentConfig{ID: "test-agent", Name: "TestAgent", Role: "dev"}
	as := NewAgentSession(api, config)

	// Set OnEvent callback
	as.OnEvent = func(event *pb.UpdateEvent) {
		atomic.AddInt32(&eventCount, 1)
	}

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	eventCh, err := as.Subscribe(context.Background(), []string{"memory"}, nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Read both events from the channel to ensure they're processed
	<-eventCh
	<-eventCh

	// Wait a bit for the callback to fire
	time.Sleep(100 * time.Millisecond)

	// Both events should have triggered the callback
	if count := atomic.LoadInt32(&eventCount); count != 2 {
		t.Errorf("Expected OnEvent callback to fire 2 times, got %d", count)
	}

	as.Close()
}

func TestAgentSession_OnEventNil(t *testing.T) {
	// Test that nil OnEvent doesn't panic
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
	// OnEvent is nil by default

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	eventCh, err := as.Subscribe(context.Background(), []string{"memory"}, nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Should receive event without panicking
	select {
	case <-eventCh:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}

	as.Close()
}

func TestAgentSession_OnEventUsedForHotBlockInvalidation(t *testing.T) {
	// Simulate the pattern used in main.go: OnEvent invalidates a hot block cache
	var invalidated int32

	subCtx := context.Background()
	api := &mockAgentAPI{
		subscribeFunc: func(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
			return &mockSubscribeStream{
				ctx: subCtx,
				events: []*pb.UpdateEvent{
					{EventType: pb.UpdateEvent_ADDED, Target: "memory", Key: "important-data", AgentId: "agent-b"},
				},
			}, nil
		},
	}
	config := AgentConfig{ID: "agent-a", Name: "Agent A", Role: "dev"}
	as := NewAgentSession(api, config)

	// This mirrors the main.go pattern:
	// agentSession.OnEvent = func(event) { agent.InvalidateHotBlock() }
	as.OnEvent = func(event *pb.UpdateEvent) {
		atomic.StoreInt32(&invalidated, 1)
	}

	if err := as.Register(context.Background()); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	eventCh, err := as.Subscribe(context.Background(), []string{"memory"}, nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Drain the event
	<-eventCh
	time.Sleep(100 * time.Millisecond)

	// The invalidation callback should have fired
	if atomic.LoadInt32(&invalidated) != 1 {
		t.Error("Expected hot block to be invalidated after event")
	}

	as.Close()
}
