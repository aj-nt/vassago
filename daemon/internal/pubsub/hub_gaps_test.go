// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package pubsub

import (
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

func TestHub_Unsubscribe(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub)

	if hub.SubscriberCount() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", hub.SubscriberCount())
	}

	hub.Unsubscribe("sub-1")

	if hub.SubscriberCount() != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", hub.SubscriberCount())
	}

	// Channel should be closed after unsubscribe
	select {
	case _, ok := <-sub.Ch:
		if ok {
			t.Error("channel should be closed after unsubscribe")
		}
	default:
		t.Error("channel should be closed, not blocking")
	}
}

func TestHub_UnsubscribeNonexistent(t *testing.T) {
	hub := NewHub()

	// Should not panic
	hub.Unsubscribe("nonexistent")
	if hub.SubscriberCount() != 0 {
		t.Error("unsubscribing nonexistent should not change count")
	}
}

func TestHub_AgentSubscriptions(t *testing.T) {
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
	sub3 := &Subscription{
		ID:      "sub-3",
		AgentID: "agent-b",
		Ch:      make(chan Event, 10),
	}

	hub.Subscribe(sub1)
	hub.Subscribe(sub2)
	hub.Subscribe(sub3)

	// Agent A should have 2 subscriptions
	aSubs := hub.AgentSubscriptions("agent-a")
	if len(aSubs) != 2 {
		t.Errorf("agent-a: expected 2 subs, got %d", len(aSubs))
	}

	// Agent B should have 1
	bSubs := hub.AgentSubscriptions("agent-b")
	if len(bSubs) != 1 {
		t.Errorf("agent-b: expected 1 sub, got %d", len(bSubs))
	}

	// Unknown agent should have 0
	unknown := hub.AgentSubscriptions("unknown")
	if len(unknown) != 0 {
		t.Errorf("unknown: expected 0 subs, got %d", len(unknown))
	}
}

func TestHub_KeyPrefixFilter(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		KeyPrefixes: []string{"python/", "go/"},
		Ch:         make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Matching prefix — should receive
	hub.Publish(Event{
		Type:    pb.UpdateEvent_ADDED,
		Target:  "memory",
		Key:     "python/version",
		AgentID: "agent-b",
	})

	select {
	case evt := <-sub.Ch:
		if evt.Key != "python/version" {
			t.Errorf("expected python/version, got %s", evt.Key)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should receive event with matching key prefix")
	}
}

func TestHub_KeyPrefixFilterNoMatch(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		KeyPrefixes: []string{"python/"},
		Ch:         make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Non-matching prefix — should NOT receive
	hub.Publish(Event{
		Type:    pb.UpdateEvent_ADDED,
		Target:  "memory",
		Key:     "ruby/version",
		AgentID: "agent-b",
	})

	select {
	case <-sub.Ch:
		t.Error("should not receive event with non-matching key prefix")
	case <-time.After(50 * time.Millisecond):
		// Correct
	}
}

func TestHub_SlowSubscriberDrop(t *testing.T) {
	hub := NewHub()

	// Tiny buffer — subscriber can't keep up
	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Ch:      make(chan Event, 1),
	}
	hub.Subscribe(sub)

	// Fill the buffer
	hub.Publish(Event{Type: pb.UpdateEvent_ADDED, Target: "memory", AgentID: "agent-b", Key: "first"})

	// Second publish should drop (buffer full, no reader)
	// This hits the "default" branch in Publish
	hub.Publish(Event{Type: pb.UpdateEvent_ADDED, Target: "memory", AgentID: "agent-b", Key: "second"})

	// Drain — should get exactly 1 event (the first one)
	select {
	case evt := <-sub.Ch:
		if evt.Key != "first" {
			t.Errorf("expected first, got %s", evt.Key)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should have received the first event")
	}

	// No second event should be in the buffer
	select {
	case <-sub.Ch:
		t.Error("second event should have been dropped")
	case <-time.After(50 * time.Millisecond):
		// Correct
	}
}

func TestHub_EmptyTargetsMatchesAll(t *testing.T) {
	hub := NewHub()

	// No target filter = receive everything
	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Publish to "user" target
	hub.Publish(Event{
		Type:    pb.UpdateEvent_ADDED,
		Target:  "user",
		Key:     "profile",
		AgentID: "agent-b",
	})

	select {
	case evt := <-sub.Ch:
		if evt.Key != "profile" {
			t.Errorf("expected profile, got %s", evt.Key)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should receive events for any target when no filter set")
	}
}

func TestHub_EmptyCategoriesMatchesAll(t *testing.T) {
	hub := NewHub()

	sub := &Subscription{
		ID:      "sub-1",
		AgentID: "agent-a",
		Targets: map[string]bool{"memory": true},
		Ch:      make(chan Event, 10),
	}
	hub.Subscribe(sub)

	// Any category should match when Categories is empty
	hub.Publish(Event{
		Type:     pb.UpdateEvent_ADDED,
		Target:   "memory",
		Category: "arbitrary-category",
		Key:      "test",
		AgentID:  "agent-b",
	})

	select {
	case <-sub.Ch:
		// Correct
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should receive events when no category filter set")
	}
}
