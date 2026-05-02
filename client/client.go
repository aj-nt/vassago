// Package client provides a gRPC client for the Mnemosyne memory daemon.
package client

import (
	"context"
	"fmt"
	"io"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a gRPC client for the Mnemosyne daemon.
type Client struct {
    conn      *grpc.ClientConn
    api       pb.MnemosyneClient
    messaging pb.MessagingClient
}

// RegisterAgent registers the agent with the daemon.
func (c *Client) RegisterAgent(ctx context.Context, agentID, name, role string) (returnedAgentID string, isNew bool, err error) {
	resp, err := c.api.RegisterAgent(ctx, &pb.RegisterAgentRequest{
		AgentId: agentID,
		Name:    name,
		Role:    role,
	})
	if err != nil {
		return "", false, fmt.Errorf("register agent: %w", err)
	}
	return resp.AgentId, resp.IsNew, nil
}

// Heartbeat sends a keep-alive to the daemon.
func (c *Client) Heartbeat(ctx context.Context, agentID string) error {
	_, err := c.api.Heartbeat(ctx, &pb.AgentId{AgentId: agentID})
	return err
}

// Subscribe opens a server-streaming subscription for memory change events.
func (c *Client) Subscribe(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error) {
	return c.api.Subscribe(ctx, req)
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

	return &Client{conn: conn, api: c, messaging: pb.NewMessagingClient(conn)}, nil
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

// SessionInfo is a brief summary of a session.
type SessionInfo struct {
	ID           string
	Title        string
	Source       string
	MessageCount int32
	LastActive   int64
}

// SearchSessions searches session titles matching the query.
func (c *Client) SearchSessions(ctx context.Context, query string, limit int32) ([]SessionInfo, error) {
	resp, err := c.api.SearchSessions(ctx, &pb.SearchSessionsRequest{
		Query: query,
		Limit: &limit,
	})
	if err != nil {
		return nil, fmt.Errorf("SearchSessions: %w", err)
	}
	result := make([]SessionInfo, len(resp.Sessions))
	for i, s := range resp.Sessions {
		result[i] = SessionInfo{
			ID:           s.Id,
			Title:        s.Title,
			Source:       s.Source,
			MessageCount: s.MessageCount,
			LastActive:   s.LastActiveAt,
		}
	}
	return result, nil
}

// ListRecentSessions returns the most recently active sessions.
func (c *Client) ListRecentSessions(ctx context.Context, limit int32) ([]SessionInfo, error) {
	resp, err := c.api.ListSessions(ctx, &pb.ListSessionsRequest{
		Limit: &limit,
	})
	if err != nil {
		return nil, fmt.Errorf("ListSessions: %w", err)
	}
	result := make([]SessionInfo, len(resp.Sessions))
	for i, s := range resp.Sessions {
		result[i] = SessionInfo{
			ID:           s.Id,
			Title:        s.Title,
			Source:       s.Source,
			MessageCount: s.MessageCount,
			LastActive:   s.LastActiveAt,
		}
	}
	return result, nil
}

// GetSession retrieves a session with its messages.
func (c *Client) GetSession(ctx context.Context, sessionID string) (*SessionDetail, error) {
	includeMsgs := true
	resp, err := c.api.GetSession(ctx, &pb.GetSessionRequest{
		SessionId:       sessionID,
		IncludeMessages: &includeMsgs,
	})
	if err != nil {
		return nil, fmt.Errorf("GetSession: %w", err)
	}

	detail := &SessionDetail{
		ID:           resp.Id,
		Title:        resp.Title,
		Source:       resp.Source,
		MessageCount: resp.MessageCount,
		CreatedAt:    resp.CreatedAt,
		LastActiveAt: resp.LastActiveAt,
	}

	for _, m := range resp.Messages {
		detail.Messages = append(detail.Messages, Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return detail, nil
}

// SendMessage sends a message via the messaging service.
func (c *Client) SendMessage(ctx context.Context, platform, target, message string) (string, error) {
	resp, err := c.messaging.SendMessage(ctx, &pb.SendMessageRequest{
		Platform: platform,
		Target:   target,
		Message:  message,
	})
	if err != nil {
		return "", fmt.Errorf("SendMessage: %w", err)
	}
	return resp.MessageId, nil
}

// ListChannels lists available channels for a platform.
func (c *Client) ListChannels(ctx context.Context, platform string) ([]*pb.Channel, error) {
	resp, err := c.messaging.ListChannels(ctx, &pb.ListChannelsRequest{
		Platform: platform,
	})
	if err != nil {
		return nil, fmt.Errorf("ListChannels: %w", err)
	}
	return resp.Channels, nil
}

// SessionDetail is a session with its messages.
type SessionDetail struct {
	ID           string
	Title        string
	Source       string
	MessageCount int32
	CreatedAt    int64
	LastActiveAt int64
	Messages     []Message
}

// --- Todo management ---

// TodoItem represents a todo task.
type TodoItem struct {
	ID          string
	Content     string
	Completed   bool
	Priority    int32
	SourceAgent string
	CreatedAt   int64
	UpdatedAt   int64
	CompletedAt int64
}

// AddTodo creates a new todo item.
func (c *Client) AddTodo(ctx context.Context, content string, priority int32, sourceAgent string) (*TodoItem, error) {
	resp, err := c.api.AddTodo(ctx, &pb.AddTodoRequest{
		Content:     content,
		Priority:    priority,
		SourceAgent: sourceAgent,
	})
	if err != nil {
		return nil, fmt.Errorf("AddTodo: %w", err)
	}
	return protoTodoToItem(resp), nil
}

// ListTodos returns todos, optionally filtering by completion status and agent.
func (c *Client) ListTodos(ctx context.Context, includeCompleted bool, sourceAgent string) ([]TodoItem, error) {
	stream, err := c.api.ListTodos(ctx, &pb.ListTodosRequest{
		IncludeCompleted: includeCompleted,
		SourceAgent:      optionalString(sourceAgent),
	})
	if err != nil {
		return nil, fmt.Errorf("ListTodos: %w", err)
	}

	var todos []TodoItem
	for {
		todo, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ListTodos stream: %w", err)
		}
		todos = append(todos, *protoTodoToItem(todo))
	}
	return todos, nil
}

// CompleteTodo marks a todo as completed.
func (c *Client) CompleteTodo(ctx context.Context, id string) (*TodoItem, error) {
	resp, err := c.api.CompleteTodo(ctx, &pb.CompleteTodoRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("CompleteTodo: %w", err)
	}
	return protoTodoToItem(resp), nil
}

// RemoveTodo removes a todo item.
func (c *Client) RemoveTodo(ctx context.Context, id string) error {
	_, err := c.api.RemoveTodo(ctx, &pb.RemoveTodoRequest{Id: id})
	if err != nil {
		return fmt.Errorf("RemoveTodo: %w", err)
	}
	return nil
}

func protoTodoToItem(pb *pb.TodoItem) *TodoItem {
	return &TodoItem{
		ID:          pb.Id,
		Content:     pb.Content,
		Completed:   pb.Completed,
		Priority:    pb.Priority,
		SourceAgent: pb.SourceAgent,
		CreatedAt:   pb.CreatedAt,
		UpdatedAt:   pb.UpdatedAt,
		CompletedAt: pb.CompletedAt,
	}
}

// optionalString returns a pointer to the string, or nil if empty.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
