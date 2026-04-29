// Package client provides a gRPC client for the Mnemosyne memory daemon.
package client

import (
	"context"
	"fmt"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a gRPC client for the Mnemosyne daemon.
type Client struct {
	conn *grpc.ClientConn
	api  pb.MnemosyneClient
}

// Config holds client connection configuration.
type Config struct {
	Address string // "unix:///path/to/socket" or "localhost:50051"
}

// Connect creates a new client connection to the Mnemosyne daemon.
func Connect(ctx context.Context, cfg Config) (*Client, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	conn, err := grpc.NewClient(cfg.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	// Verify connection is alive
	c := pb.NewMnemosyneClient(conn)
	_, err = c.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    "memory",
		AgentId:   "health-check",
		CharLimit: 1,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("health check: %w", err)
	}

	return &Client{conn: conn, api: c}, nil
}

// Close closes the connection to the daemon.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// AddMemory adds or updates a memory entry.
func (c *Client) AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error) {
	resp, err := c.api.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      target,
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: sourceAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("AddMemory: %w", err)
	}
	return resp.Entry, nil
}

// GetMemory retrieves a memory by ID.
func (c *Client) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	resp, err := c.api.GetMemory(ctx, &pb.GetMemoryRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("GetMemory: %w", err)
	}
	return resp, nil
}

// RemoveMemory deletes a memory by ID.
func (c *Client) RemoveMemory(ctx context.Context, id string) error {
	_, err := c.api.RemoveMemory(ctx, &pb.RemoveMemoryRequest{Id: id})
	if err != nil {
		return fmt.Errorf("RemoveMemory: %w", err)
	}
	return nil
}

// SearchMemories searches memories using FTS5.
func (c *Client) SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error) {
	resp, err := c.api.Search(ctx, &pb.SearchRequest{Query: query, Target: target, Limit: &limit})
	if err != nil {
		return nil, fmt.Errorf("SearchMemories: %w", err)
	}
	return resp.Entries, nil
}

// ListMemories lists memories with optional filtering.
func (c *Client) ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	resp, err := c.api.ListMemories(ctx, &pb.ListMemoriesRequest{
		Target:      target,
		Category:    category,
		MinPriority: minPriority,
	})
	if err != nil {
		return nil, fmt.Errorf("ListMemories: %w", err)
	}
	return resp.Entries, nil
}

// GetHotBlock retrieves the computed hot block for an agent.
func (c *Client) GetHotBlock(ctx context.Context, target, agentID string, charLimit int32, boostCategories, suppressCategories []string, boostPriority int32) (string, error) {
	resp, err := c.api.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    target,
		AgentId:   agentID,
		CharLimit: charLimit,
		Config: &pb.HotBlockConfig{
			BoostCategories:    boostCategories,
			SuppressCategories: suppressCategories,
			BoostPriority:      boostPriority,
		},
	})
	if err != nil {
		return "", fmt.Errorf("GetHotBlock: %w", err)
	}
	return resp.Content, nil
}

// CreateSession creates a new session.
func (c *Client) CreateSession(ctx context.Context, agentID, source, title string) (string, error) {
	resp, err := c.api.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: agentID,
		Source:  source,
		Title:   title,
	})
	if err != nil {
		return "", fmt.Errorf("CreateSession: %w", err)
	}
	return resp.Id, nil
}

// AddMessages adds messages to a session.
func (c *Client) AddMessages(ctx context.Context, sessionID string, messages []Message) error {
	pbMsgs := make([]*pb.Message, len(messages))
	for i, m := range messages {
		pbMsgs[i] = &pb.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	_, err := c.api.AddMessages(ctx, &pb.AddMessagesRequest{
		SessionId: sessionID,
		Messages:  pbMsgs,
	})
	if err != nil {
		return fmt.Errorf("AddMessages: %w", err)
	}
	return nil
}

// EndSession ends a session.
func (c *Client) EndSession(ctx context.Context, sessionID string) error {
	_, err := c.api.EndSession(ctx, &pb.EndSessionRequest{SessionId: sessionID})
	if err != nil {
		return fmt.Errorf("EndSession: %w", err)
	}
	return nil
}

// Message is a conversation message (role + content).
type Message struct {
	Role    string
	Content string
}

// Consolidate triggers memory consolidation with the given scope.
func (c *Client) Consolidate(ctx context.Context, scope pb.ConsolidateScope, batchSize int32) (*pb.ConsolidateResponse, error) {
	resp, err := c.api.Consolidate(ctx, &pb.ConsolidateRequest{
		Scope:     scope,
		BatchSize: &batchSize,
	})
	if err != nil {
		return nil, fmt.Errorf("Consolidate: %w", err)
	}
	return resp, nil
}

// HealthCheck verifies the daemon is reachable and responding.
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.api.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    "memory",
		AgentId:   "health-check",
		CharLimit: 1,
	})
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	return nil
}