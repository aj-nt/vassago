package pubsub

import (
	"fmt"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
)

func TestHub_subscribeAndPublish(t *testing.T) {
	hub := NewHub()

	// Agent A subscribes to "memory" target
	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Targets: map[string]bool{"memory": true},
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Agent B writes a memory — Agent A should get notified
	hub.Publish(Event{
		Type:     pb.UpdateEvent_ADDED,
		Target:   "memory",
		Category: "environment",
		Key:      "python",
		AgentID:  "agent-b",
	})

	select {
	case evt := <-sub.Ch:
		if evt.Type != pb.UpdateEvent_ADDED {
			t.Errorf("expected ADDED, got %v", evt.Type)
		}
		if evt.Key != "python" {
			t.Errorf("expected key=python, got %s", evt.Key)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestHub_noSelfNotification(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Targets: map[string]bool{"memory": true},
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Agent A writes — should NOT notify itself
	hub.Publish(Event{
		Type:    pb.UpdateEvent_ADDED,
		Target:  "memory",
		AgentID: "agent-a",
	})

	select {
	case <-sub.Ch:
		t.Error("agent should not receive its own events")
	case <-time.After(50 * time.Millisecond):
		// Correct: no event received
	}
}

func TestHub_targetFilter(t *testing.T) {
	hub := NewHub()

	// Subscribe to "memory" only
	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Targets: map[string]bool{"memory": true},
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Publish to "user" target — should NOT match
	hub.Publish(Event{
		Type:    pb.UpdateEvent_ADDED,
		Target:  "user",
		AgentID: "agent-b",
	})

	select {
	case <-sub.Ch:
		t.Error("should not receive events for non-subscribed target")
	case <-time.After(50 * time.Millisecond):
		// Correct
	}

	// Publish to "memory" target — SHOULD match
	hub.Publish(Event{
		Type:    pb.UpdateEvent_ADDED,
		Target:  "memory",
		AgentID: "agent-b",
	})

	select {
	case <-sub.Ch:
		// Correct
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should receive events for subscribed target")
	}
}

func TestHub_categoryFilter(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:         "sub-1",
		AgentID:    "agent-a",
		Targets:    map[string]bool{"memory": true},
		Categories: map[string]bool{"project": true, "environment": true},
		Ch:         make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Publish "observation" — should NOT match
	hub.Publish(Event{
		Type:     pb.UpdateEvent_ADDED,
		Target:   "memory",
		Category: "observation",
		AgentID:  "agent-b",
	})

	select {
	case <-sub.Ch:
		t.Error("should not receive events for non-subscribed category")
	case <-time.After(50 * time.Millisecond):
		// Correct
	}

	// Publish "project" — SHOULD match
	hub.Publish(Event{
		Type:     pb.UpdateEvent_ADDED,
		Target:   "memory",
		Category: "project",
		AgentID:  "agent-b",
	})

	select {
	case <-sub.Ch:
		// Correct
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should receive events for subscribed category")
	}
}

func TestHub_unsubscribeAll(t *testing.T) {
	hub := NewHub()

	sub1 := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Ch:      make(chan Event, 10),
	}
	sub2 := &Subscription{
		ID:      "sub-2",
		AgentID: "agent-a",
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub1)
	hub.Subscribe(sub2)

	if hub.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", hub.SubscriberCount())
	}

	hub.UnsubscribeAll("agent-a")

	if hub.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe all, got %d", hub.SubscriberCount())
	}
}

func TestHub_1000RapidWrites(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Ch:      make(chan Event, 1000), // Large buffer
	}
	hub.Subscribe(sub)

	// Publish 1000 events rapidly
	for i := 0; i < 1000; i++ {
		hub.Publish(Event{
			Type:    pb.UpdateEvent_ADDED,
			Target:  "memory",
			Key:     fmt.Sprintf("key-%d", i),
			AgentID: "agent-b",
		})
	}

	// Count received events
	received := 0
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-sub.Ch:
			received++
			if received == 1000 {
				return // Success
			}
		case <-timeout:
			t.Errorf("received %d/1000 events before timeout", received)
			return
		}
	}
}
