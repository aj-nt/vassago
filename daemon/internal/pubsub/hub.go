package pubsub

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
)

// Event represents a memory change event.
type Event struct {
	Type     pb.UpdateEvent_EventType
	Target   string
	Category string
	Key      string
	AgentID  string
	Entry    *pb.MemoryEntry
	CronFire *pb.CronFireEvent
}

// Subscription represents an agent's subscription to change events.
type Subscription struct {
	ID         string
	AgentID    string
	Targets    map[string]bool   // "memory", "user"
	Categories map[string]bool  // empty = all
	KeyPrefixes []string
	Ch         chan Event
	CreatedAt  int64
}

// Hub is the in-process pub/sub hub for memory change events.
type Hub struct {
	mu           sync.RWMutex
	subscriptions map[string]*Subscription
	agentSubs    map[string][]string // agent_id -> subscription IDs
}

// NewHub creates a new pub/sub hub.
func NewHub() *Hub {
	return &Hub{
		subscriptions: make(map[string]*Subscription),
		agentSubs:    make(map[string][]string),
	}
}

// Subscribe adds a subscription for an agent.
func (h *Hub) Subscribe(sub *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sub.CreatedAt = time.Now().Unix()
	h.subscriptions[sub.ID] = sub

	h.agentSubs[sub.AgentID] = append(h.agentSubs[sub.AgentID], sub.ID)
}

// Unsubscribe removes a subscription.
func (h *Hub) Unsubscribe(subID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if sub, ok := h.subscriptions[subID]; ok {
		close(sub.Ch)
		delete(h.subscriptions, subID)

		// Remove from agent's subscription list
		subs := h.agentSubs[sub.AgentID]
		for i, id := range subs {
			if id == subID {
				h.agentSubs[sub.AgentID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
}

// UnsubscribeAll removes all subscriptions for an agent (on disconnect).
func (h *Hub) UnsubscribeAll(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	subIDs := h.agentSubs[agentID]
	for _, id := range subIDs {
		if sub, ok := h.subscriptions[id]; ok {
			close(sub.Ch)
			delete(h.subscriptions, id)
		}
	}
	delete(h.agentSubs, agentID)
}

// Publish sends an event to all matching subscribers.
func (h *Hub) Publish(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, sub := range h.subscriptions {
		if h.matches(sub, event) {
			select {
			case sub.Ch <- event:
			default:
				// Drop if subscriber is too slow (non-blocking)
				fmt.Printf("pubsub: dropping event for slow subscriber %s\n", sub.AgentID)
			}
		}
	}
}

// matches checks if a subscription matches an event.
func (h *Hub) matches(sub *Subscription, event Event) bool {
	// Don't notify the agent that made the change
	if sub.AgentID == event.AgentID {
		return false
	}

	// Check target filter
	if len(sub.Targets) > 0 && !sub.Targets[event.Target] {
		return false
	}

	// Check category filter
	if len(sub.Categories) > 0 && !sub.Categories[event.Category] {
		return false
	}

	// Check key prefix filter
	if len(sub.KeyPrefixes) > 0 {
		matched := false
		for _, prefix := range sub.KeyPrefixes {
			if len(event.Key) >= len(prefix) && event.Key[:len(prefix)] == prefix {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// SubscriberCount returns the number of active subscriptions.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscriptions)
}

// AgentSubscriptions returns subscription IDs for an agent.
func (h *Hub) AgentSubscriptions(agentID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.agentSubs[agentID]
}
