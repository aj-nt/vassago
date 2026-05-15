// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
)

// AgentConfig holds agent identity for registration.
type AgentConfig struct {
	ID   string // Unique agent ID (required)
	Name string // Human-readable name
	Role string // Agent role: "dev", "personal", "monitor"
}

// AgentAPI is the subset of daemon RPCs needed for agent lifecycle.
// Extracted as an interface so tests can provide a mock.
type AgentAPI interface {
	RegisterAgent(ctx context.Context, agentID, name, role string) (returnedAgentID string, isNew bool, err error)
	Heartbeat(ctx context.Context, agentID string) error
	Subscribe(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error)
}

// OnEventFunc is called when a Telepathy event arrives.
// The callback is invoked in the subscription goroutine, so it must be
// thread-safe and non-blocking.
type OnEventFunc func(event *pb.UpdateEvent)

// AgentSession manages an agent's lifecycle with the Mnemosyne daemon:
// registration, heartbeat, and subscription.
type AgentSession struct {
	api               AgentAPI
	config            AgentConfig
	HeartbeatInterval time.Duration
	OnEvent           OnEventFunc // Optional callback for Telepathy events

	mu           sync.Mutex
	registered   bool
	cancelHB     context.CancelFunc // stops heartbeat goroutine
	done         chan struct{}      // closed when heartbeat goroutine exits
	stream       pb.Mnemosyne_SubscribeClient
	streamCancel context.CancelFunc
}

// NewAgentSession creates a new agent session.
// If config.ID is empty, a unique ID is generated.
func NewAgentSession(api AgentAPI, config AgentConfig) *AgentSession {
	interval := 30 * time.Second
	if config.ID == "" {
		config.ID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	return &AgentSession{
		api:               api,
		config:            config,
		HeartbeatInterval: interval,
		done:              make(chan struct{}),
	}
}

// Register registers the agent with the daemon and starts the heartbeat loop.
func (as *AgentSession) Register(ctx context.Context) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	if as.registered {
		return fmt.Errorf("agent already registered")
	}

	agentID, _, err := as.api.RegisterAgent(ctx, as.config.ID, as.config.Name, as.config.Role)
	if err != nil {
		return fmt.Errorf("register agent: %w", err)
	}
	as.config.ID = agentID
	as.registered = true

	// Start heartbeat goroutine
	hbCtx, cancel := context.WithCancel(context.Background())
	as.cancelHB = cancel
	go as.heartbeatLoop(hbCtx)

	log.Printf("Agent registered: id=%s name=%s role=%s", as.config.ID, as.config.Name, as.config.Role)
	return nil
}

// Subscribe subscribes to memory change events from the daemon.
// Returns a channel of UpdateEvents. Call Close() to unsubscribe.
func (as *AgentSession) Subscribe(ctx context.Context, targets, categories []string) (<-chan *pb.UpdateEvent, error) {
	as.mu.Lock()
	defer as.mu.Unlock()

	if !as.registered {
		return nil, fmt.Errorf("agent not registered — call Register first")
	}

	subCtx, cancel := context.WithCancel(ctx)
	as.streamCancel = cancel

	stream, err := as.api.Subscribe(subCtx, &pb.SubscribeRequest{
		AgentId:    as.config.ID,
		Targets:    targets,
		Categories: categories,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe: %w", err)
	}
	as.stream = stream

	// Read events from stream and forward to channel + callback
	eventCh := make(chan *pb.UpdateEvent, 50)
	go func() {
		defer close(eventCh)
		for {
			event, err := stream.Recv()
			if err != nil {
				// Stream ended (disconnect, cancel, etc.)
				return
			}
			// Invoke callback if set (e.g., to invalidate hot block cache)
			if as.OnEvent != nil {
				as.OnEvent(event)
			}
			select {
			case eventCh <- event:
			default:
				log.Printf("AgentSession: dropping event for slow consumer")
			}
		}
	}()

	log.Printf("Agent subscribed: id=%s targets=%v categories=%v", as.config.ID, targets, categories)
	return eventCh, nil
}

// Close gracefully shuts down the agent session: stops heartbeats,
// cancels subscription, and unsubscribes from the hub.
func (as *AgentSession) Close() {
	as.mu.Lock()
	registered := as.registered
	cancelHB := as.cancelHB
	streamCancel := as.streamCancel
	as.mu.Unlock()

	if cancelHB != nil {
		cancelHB()
	}
	if streamCancel != nil {
		streamCancel()
	}

	// Wait for heartbeat goroutine to exit
	if registered {
		<-as.done
	}

	as.mu.Lock()
	as.registered = false
	as.mu.Unlock()
}

// AgentID returns the registered agent ID.
func (as *AgentSession) AgentID() string {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.config.ID
}

// IsRegistered returns whether the agent has registered with the daemon.
func (as *AgentSession) IsRegistered() bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.registered
}

// heartbeatLoop sends periodic heartbeats to the daemon.
func (as *AgentSession) heartbeatLoop(ctx context.Context) {
	defer close(as.done)

	ticker := time.NewTicker(as.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := as.api.Heartbeat(hbCtx, as.config.ID)
			cancel()
			if err != nil {
				log.Printf("AgentSession: heartbeat failed: %v", err)
			}
		}
	}
}
