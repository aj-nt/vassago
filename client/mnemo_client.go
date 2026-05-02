// Package client provides a gRPC client for the Mnemosyne memory daemon.
package client

import (
	"context"

	pb "github.com/aj-nt/mnemosyne/proto"
)

// MnemoClient is an interface for interacting with the Mnemosyne daemon.
type MnemoClient interface {
	// RegisterAgent registers the agent with the daemon.
	RegisterAgent(ctx context.Context, agentID, name, role string) (returnedAgentID string, isNew bool, err error)

	// Heartbeat sends a keep-alive to the daemon.
	Heartbeat(ctx context.Context, agentID string) error

	// Subscribe opens a server-streaming subscription for memory change events.
	Subscribe(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error)

	// AddMemory adds or updates a memory entry.
	AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error)

	// GetMemory retrieves a memory by ID.
	GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error)

	// RemoveMemory deletes a memory by ID.
	RemoveMemory(ctx context.Context, id string) error

	// SearchMemories searches memories using FTS5.
	SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error)

	// ListMemories lists memories with optional filtering.
	ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error)

	// GetHotBlock retrieves the computed hot block for an agent.
	GetHotBlock(ctx context.Context, target, agentID string, charLimit int32, boostCategories, suppressCategories []string, boostPriority int32) (string, error)

	// CreateSession creates a new session.
	CreateSession(ctx context.Context, agentID, source, title string) (string, error)

	// AddMessages adds messages to a session.
	AddMessages(ctx context.Context, sessionID string, messages []Message) error

	// EndSession ends a session.
	EndSession(ctx context.Context, sessionID string) error

	// Consolidate triggers memory consolidation with the given scope.
	Consolidate(ctx context.Context, scope pb.ConsolidateScope, batchSize int32) (*pb.ConsolidateResponse, error)

	// HealthCheck verifies the daemon is reachable and responding.
	HealthCheck(ctx context.Context) error

	// SearchSessions searches session titles matching the query.
	SearchSessions(ctx context.Context, query string, limit int32) ([]SessionInfo, error)

	// ListRecentSessions returns the most recently active sessions.
	ListRecentSessions(ctx context.Context, limit int32) ([]SessionInfo, error)

	// GetSession retrieves a session with its messages.
	GetSession(ctx context.Context, sessionID string) (*SessionDetail, error)

	// SendMessage sends a message via the messaging service.
	SendMessage(ctx context.Context, platform, target, message string) (string, error)

	// ListChannels lists available channels for a platform.
	ListChannels(ctx context.Context, platform string) ([]*pb.Channel, error)

	// AddTodo creates a new todo item.
	AddTodo(ctx context.Context, content string, priority int32, sourceAgent string) (*TodoItem, error)

	// ListTodos returns todos, optionally filtering by completion status and agent.
	ListTodos(ctx context.Context, includeCompleted bool, sourceAgent string) ([]TodoItem, error)

	// CompleteTodo marks a todo as completed.
	CompleteTodo(ctx context.Context, id string) (*TodoItem, error)

	// RemoveTodo removes a todo item.
	RemoveTodo(ctx context.Context, id string) error

	// Close closes the connection to the daemon.
	Close() error
}

// Ensure that Client implements the MnemoClient interface.
var _ MnemoClient = (*Client)(nil)
