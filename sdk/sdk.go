// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// Package sdk provides a batteries-included client for the Vassago memory
// daemon. It handles connection lifecycle, agent registration, heartbeats,
// pub/sub event streaming, and offers a clean MemoryStore interface for
// CRUD, search, sessions, tools, skills, todos, cron, and messaging.
//
// Quick start:
//
//	store, err := sdk.Connect(ctx, sdk.Config{
//	    Address:   "localhost:50051",
//	    AgentName: "hermes",
//	})
//	if err != nil { ... }
//	defer store.Close()
//
//	hotBlock, _ := store.HotBlock(ctx)
//	mem, _ := store.Remember(ctx, "aj uses neovim", "project", 3)
//	results, _ := store.Search(ctx, "vim", 5)
package sdk

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
	memoformat "github.com/aj-nt/vassago-sdk/messaging/format"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// Config holds everything needed to connect to the daemon and identify
// the calling agent. Fields are intentionally flat — no nesting, no
// fallback chains.
type Config struct {
	// Address is the daemon's gRPC address (e.g., "localhost:50051").
	Address string

	// AgentName identifies this agent to the daemon (e.g., "hermes").
	// Must be non-empty.
	AgentName string

	// AgentRole describes the agent's role (e.g., "dev", "personal").
	// Defaults to "assistant" if empty.
	AgentRole string

	// HeartbeatInterval is how often to ping the daemon.
	// Defaults to 30s if zero.
	HeartbeatInterval time.Duration

	// HotBlockCharLimit caps the hot block size returned by HotBlock().
	// Defaults to 8000 if zero.
	HotBlockCharLimit int32

	// OnEvent is an optional callback for memory change events from the
	// daemon's pub/sub system. Called in a background goroutine, so it
	// must be thread-safe and non-blocking.
	OnEvent func(event *pb.UpdateEvent)
}

// MemoryStore is the primary interface agents use to interact with the
// daemon. It bundles memory CRUD, search, sessions, and configuration
// into a single value.
type MemoryStore struct {
	client client.MnemoClient
	cfg    Config
	agent  *client.AgentSession

	// Cached hot block, refreshed on each HotBlock() call.
	mu       sync.RWMutex
	hotBlock string
}

// Connect dials the daemon, registers the agent, starts heartbeats,
// subscribes to pub/sub events, and returns a ready-to-use MemoryStore.
func Connect(ctx context.Context, cfg Config) (*MemoryStore, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("sdk: config.Address is required")
	}
	if cfg.AgentName == "" {
		return nil, fmt.Errorf("sdk: config.AgentName is required")
	}
	if cfg.AgentRole == "" {
		cfg.AgentRole = "assistant"
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 30 * time.Second
	}
	if cfg.HotBlockCharLimit <= 0 {
		cfg.HotBlockCharLimit = 8000
	}

	// Dial the daemon.
	c, err := client.Connect(ctx, client.Config{Address: cfg.Address})
	if err != nil {
		return nil, fmt.Errorf("sdk: connect to daemon at %s: %w", cfg.Address, err)
	}

	// Set up agent lifecycle (registration, heartbeat, pub/sub).
	agent := client.NewAgentSession(c, client.AgentConfig{
		ID:   cfg.AgentName,
		Name: cfg.AgentName,
		Role: cfg.AgentRole,
	})
	agent.HeartbeatInterval = cfg.HeartbeatInterval
	agent.OnEvent = cfg.OnEvent

	if err := agent.Register(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("sdk: register agent %q: %w", cfg.AgentName, err)
	}

	// Subscribe to memory change events.
	_, err = agent.Subscribe(ctx, []string{"memory"}, nil)
	if err != nil {
		slog.Warn("sdk: failed to subscribe to memory events (non-fatal)", "error", err)
	}

	return &MemoryStore{
		client: c,
		cfg:    cfg,
		agent:  agent,
	}, nil
}

// Close gracefully shuts down the connection. Call once when done.
func (s *MemoryStore) Close() error {
	s.agent.Close()
	return s.client.Close()
}

// AgentName returns the agent name this store was configured with.
func (s *MemoryStore) AgentName() string { return s.cfg.AgentName }

// IsConnected reports whether the daemon is reachable.
func (s *MemoryStore) IsConnected() bool { return s.client.IsConnected() }

// --- Hot Block ---

// HotBlock returns the agent's current hot block (cached memories +
// environment context) from the daemon. The cache is invalidated
// automatically when the daemon publishes memory change events through
// the pub/sub subscription.
func (s *MemoryStore) HotBlock(ctx context.Context) (string, error) {
	hot, err := s.client.GetHotBlock(ctx, "memory", s.cfg.AgentName,
		s.cfg.HotBlockCharLimit, nil, nil, 0)
	if err != nil {
		return "", fmt.Errorf("hot block: %w", err)
	}
	s.mu.Lock()
	s.hotBlock = hot
	s.mu.Unlock()
	return hot, nil
}

// CachedHotBlock returns the last fetched hot block without a network
// round-trip. Returns "" if HotBlock() was never called.
func (s *MemoryStore) CachedHotBlock() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hotBlock
}

// --- Memory CRUD ---

// Remember adds or updates a memory entry. Key is optional — leave empty
// to auto-derive one from the content. Returns the stored entry.
func (s *MemoryStore) Remember(ctx context.Context, content, category string, priority int32) (*pb.MemoryEntry, error) {
	return s.client.AddMemory(ctx, "memory", category, "", content, priority, s.cfg.AgentName)
}

// RememberWithKey adds a memory with an explicit key for later update.
func (s *MemoryStore) RememberWithKey(ctx context.Context, key, content, category string, priority int32) (*pb.MemoryEntry, error) {
	return s.client.AddMemory(ctx, "memory", category, key, content, priority, s.cfg.AgentName)
}

// Forget removes a memory by ID. Returns an error if not found.
func (s *MemoryStore) Forget(ctx context.Context, id string) error {
	return s.client.RemoveMemory(ctx, id)
}

// Search finds memories matching a query using FTS5 full-text search.
func (s *MemoryStore) Search(ctx context.Context, query string, limit int32) ([]*pb.MemoryEntry, error) {
	return s.client.SearchMemories(ctx, query, "memory", limit)
}

// List returns memories, optionally filtered by category and min priority.
func (s *MemoryStore) List(ctx context.Context, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	return s.client.ListMemories(ctx, "memory", category, minPriority)
}

// Consolidate triggers memory consolidation with the given scope.
func (s *MemoryStore) Consolidate(ctx context.Context, scope pb.ConsolidateScope, batchSize int32) (*pb.ConsolidateResponse, error) {
	return s.client.Consolidate(ctx, scope, batchSize)
}

// GetMemory retrieves a single memory by ID.
func (s *MemoryStore) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	return s.client.GetMemory(ctx, id)
}

// --- Session Management ---

// CreateSession creates a new session on the daemon and returns its ID.
func (s *MemoryStore) CreateSession(ctx context.Context, source, title string) (string, error) {
	return s.client.CreateSession(ctx, s.cfg.AgentName, source, title)
}

// AppendMessages adds messages to an existing session on the daemon.
func (s *MemoryStore) AppendMessages(ctx context.Context, sessionID string, messages []client.Message) error {
	return s.client.AddMessages(ctx, sessionID, messages)
}

// EndSession marks a session as ended.
func (s *MemoryStore) EndSession(ctx context.Context, sessionID string) error {
	return s.client.EndSession(ctx, sessionID)
}

// GetSession retrieves a session with its messages from the daemon.
func (s *MemoryStore) GetSession(ctx context.Context, sessionID string) (*client.SessionDetail, error) {
	return s.client.GetSession(ctx, sessionID)
}

// SearchSessions finds sessions by title text.
func (s *MemoryStore) SearchSessions(ctx context.Context, query string, limit int32) ([]client.SessionInfo, error) {
	return s.client.SearchSessions(ctx, query, limit)
}

// RecentSessions returns the most recently active sessions.
func (s *MemoryStore) RecentSessions(ctx context.Context, limit int32) ([]client.SessionInfo, error) {
	return s.client.ListRecentSessions(ctx, limit)
}

// --- Messaging ---

// SendMessage sends a message to a platform target via the daemon's
// messaging adapters.
func (s *MemoryStore) SendMessage(ctx context.Context, platform, target, message string) (string, error) {
	return s.client.SendMessage(ctx, platform, target, message)
}

// ListChannels returns available channels for a platform.
func (s *MemoryStore) ListChannels(ctx context.Context, platform string) ([]*pb.Channel, error) {
	return s.client.ListChannels(ctx, platform)
}

// --- Todos ---

// AddTodo creates a todo item.
func (s *MemoryStore) AddTodo(ctx context.Context, content string, priority int32) (*client.TodoItem, error) {
	return s.client.AddTodo(ctx, content, priority, s.cfg.AgentName)
}

// ListTodos returns todos, optionally including completed ones.
func (s *MemoryStore) ListTodos(ctx context.Context, includeCompleted bool) ([]client.TodoItem, error) {
	return s.client.ListTodos(ctx, includeCompleted, s.cfg.AgentName)
}

// CompleteTodo marks a todo as done.
func (s *MemoryStore) CompleteTodo(ctx context.Context, id string) (*client.TodoItem, error) {
	return s.client.CompleteTodo(ctx, id)
}

// RemoveTodo deletes a todo.
func (s *MemoryStore) RemoveTodo(ctx context.Context, id string) error {
	return s.client.RemoveTodo(ctx, id)
}

// --- Skills ---

// AddSkill registers a skill with the daemon.
func (s *MemoryStore) AddSkill(ctx context.Context, name, description string, triggers []string, category, content string, priority int32) (*pb.SkillEntry, error) {
	return s.client.AddSkill(ctx, name, description, triggers, category, content, s.cfg.AgentName, priority)
}

// GetSkill retrieves a skill by ID.
func (s *MemoryStore) GetSkill(ctx context.Context, id string) (*pb.SkillEntry, error) {
	return s.client.GetSkill(ctx, id)
}

// ListSkills returns skills, optionally filtered.
func (s *MemoryStore) ListSkills(ctx context.Context, category string, limit int32) ([]*pb.SkillEntry, error) {
	return s.client.ListSkills(ctx, category, s.cfg.AgentName, limit)
}

// SearchSkills searches skills via FTS5.
func (s *MemoryStore) SearchSkills(ctx context.Context, query, category string, limit int32) ([]*pb.SkillEntry, error) {
	return s.client.SearchSkills(ctx, query, category, limit)
}

// RemoveSkill deletes a skill.
func (s *MemoryStore) RemoveSkill(ctx context.Context, id string) (bool, error) {
	return s.client.RemoveSkill(ctx, id)
}

// --- Cron ---

// ScheduleJob creates a cron job for this agent.
func (s *MemoryStore) ScheduleJob(ctx context.Context, name, schedule, message string, enabled bool) (*client.CronJobInfo, error) {
	return s.client.CreateCronJob(ctx, name, schedule, s.cfg.AgentName, message, &enabled)
}

// ListJobs returns cron jobs for this agent.
func (s *MemoryStore) ListJobs(ctx context.Context, includeDisabled bool) ([]client.CronJobInfo, error) {
	return s.client.ListCronJobs(ctx, s.cfg.AgentName, includeDisabled)
}

// UpdateJob modifies a cron job.
func (s *MemoryStore) UpdateJob(ctx context.Context, id string, name, schedule, message *string, enabled *bool) (*client.CronJobInfo, error) {
	return s.client.UpdateCronJob(ctx, id, name, schedule, message, enabled)
}

// DeleteJob removes a cron job.
func (s *MemoryStore) DeleteJob(ctx context.Context, id string) error {
	return s.client.DeleteCronJob(ctx, id)
}

// --- Incoming platform messages ---

// SubscribeToMessages starts receiving message events from the daemon's
// messaging adapters (Discord, Telegram, etc.). Returns the raw gRPC
// stream — the caller should loop Recv() to process events.
//
// For a higher-level interface, use ConversationManager.
func (s *MemoryStore) SubscribeToMessages(ctx context.Context) (pb.Messaging_SubscribeToMessagesClient, error) {
	realClient, ok := s.client.(*client.Client)
	if !ok {
		return nil, fmt.Errorf("SubscribeToMessages: daemon connection required (got %T)", s.client)
	}
	return realClient.SubscribeToMessages(ctx, s.cfg.AgentName)
}

// --- Platform-aware message formatting ---

// ComposePlatformContext returns a platform-aware context prefix for a
// message, so the agent knows where the message came from.
func ComposePlatformContext(platform, channelName, senderName string) string {
	return memoformat.ComposePlatformContext(platform, channelName, senderName)
}
