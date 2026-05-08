// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

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

// ClientInterface defines the interface for client operations
type ClientInterface interface {
	RegisterAgent(ctx context.Context, agentID, name, role string) (string, bool, error)
	Heartbeat(ctx context.Context, agentID string) error
	Subscribe(ctx context.Context, req *pb.SubscribeRequest) (pb.Mnemosyne_SubscribeClient, error)
	Close() error
	AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error)
	GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error)
	RemoveMemory(ctx context.Context, id string) error
	SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error)
	ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error)
	GetHotBlock(ctx context.Context, target, agentID string, charLimit int32, boostCategories, suppressCategories []string, boostPriority int32) (string, error)
	AddSkill(ctx context.Context, name, description string, triggers []string, category, content, sourceAgent string, priority int32) (*pb.SkillEntry, error)
	GetSkill(ctx context.Context, id string) (*pb.SkillEntry, error)
	ListSkills(ctx context.Context, category, sourceAgent string, limit int32) ([]*pb.SkillEntry, error)
	SearchSkills(ctx context.Context, query, category string, limit int32) ([]*pb.SkillEntry, error)
	RemoveSkill(ctx context.Context, id string) (bool, error)
	CreateSession(ctx context.Context, agentID, source, title string) (string, error)
	AddMessages(ctx context.Context, sessionID string, messages []Message) error
	EndSession(ctx context.Context, sessionID string) error
	Consolidate(ctx context.Context, scope pb.ConsolidateScope, batchSize int32) (*pb.ConsolidateResponse, error)
	HealthCheck(ctx context.Context) error
	SearchSessions(ctx context.Context, query string, limit int32) ([]SessionInfo, error)
	ListRecentSessions(ctx context.Context, limit int32) ([]SessionInfo, error)
	GetSession(ctx context.Context, sessionID string) (*SessionDetail, error)
	SendMessage(ctx context.Context, platform, target, message string) (string, error)
	ListChannels(ctx context.Context, platform string) ([]*pb.Channel, error)
	SubscribeToMessages(ctx context.Context, agentID string) (pb.Messaging_SubscribeToMessagesClient, error)
	AddTodo(ctx context.Context, content string, priority int32, sourceAgent string) (*TodoItem, error)
	ListTodos(ctx context.Context, includeCompleted bool, sourceAgent string) ([]TodoItem, error)
	CompleteTodo(ctx context.Context, id string) (*TodoItem, error)
	RemoveTodo(ctx context.Context, id string) error
	CreateCronJob(ctx context.Context, name, schedule, agentID, message string, enabled *bool) (*CronJobInfo, error)
	ListCronJobs(ctx context.Context, agentID string, includeDisabled bool) ([]CronJobInfo, error)
	UpdateCronJob(ctx context.Context, id string, name, schedule, message *string, enabled *bool) (*CronJobInfo, error)
	DeleteCronJob(ctx context.Context, id string) error
}

// Ensure Client implements ClientInterface
var _ ClientInterface = &Client{}

// Client is a gRPC client for the Mnemosyne daemon.
type Client struct {
	conn      *grpc.ClientConn
	api       pb.MnemosyneClient
	messaging pb.MessagingClient
	cron      pb.CronServiceClient
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

// NewClientFromConn creates a Client from an existing gRPC connection.
// This is useful for testing with bufconn or other in-memory connections.
func NewClientFromConn(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:      conn,
		api:       pb.NewMnemosyneClient(conn),
		messaging: pb.NewMessagingClient(conn),
		cron:      pb.NewCronServiceClient(conn),
	}
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

	return &Client{conn: conn, api: c, messaging: pb.NewMessagingClient(conn), cron: pb.NewCronServiceClient(conn)}, nil
}

// ConnectMnemo creates a new client connection and returns the MnemoClient interface.
// This is the preferred way to connect when you only need the MnemoClient interface
// (which includes all commonly used operations).
func ConnectMnemo(ctx context.Context, cfg Config) (MnemoClient, error) {
	c, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return c, nil
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

// SubscribeToMessages opens a server-streaming subscription to incoming messages.
// It returns the gRPC stream for receiving MessageEvents. The caller should
// call Recv() in a loop to process messages, and CloseSend() when done.
func (c *Client) SubscribeToMessages(ctx context.Context, agentID string) (pb.Messaging_SubscribeToMessagesClient, error) {
	stream, err := c.messaging.SubscribeToMessages(ctx, &pb.SubscribeToMessagesRequest{
		AgentId: agentID,
	})
	if err != nil {
		return nil, fmt.Errorf("SubscribeToMessages: %w", err)
	}
	return stream, nil
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
	if pb == nil {
		return nil
	}
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

// --- Skill management ---

// AddSkill creates a new skill entry.
func (c *Client) AddSkill(ctx context.Context, name, description string, triggers []string, category, content, sourceAgent string, priority int32) (*pb.SkillEntry, error) {
	resp, err := c.api.AddSkill(ctx, &pb.AddSkillRequest{
		Name:         name,
		Description:  description,
		Triggers:     triggers,
		Category:     category,
		Content:      content,
		SourceAgent:  sourceAgent,
		Priority:     &priority,
	})
	if err != nil {
		return nil, fmt.Errorf("AddSkill: %w", err)
	}
	return resp, nil
}

// GetSkill retrieves a skill by ID.
func (c *Client) GetSkill(ctx context.Context, id string) (*pb.SkillEntry, error) {
	resp, err := c.api.GetSkill(ctx, &pb.GetSkillRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("GetSkill: %w", err)
	}
	return resp, nil
}

// ListSkills lists skills with optional filtering.
func (c *Client) ListSkills(ctx context.Context, category, sourceAgent string, limit int32) ([]*pb.SkillEntry, error) {
	resp, err := c.api.ListSkills(ctx, &pb.ListSkillsRequest{
		Category:    category,
		SourceAgent: sourceAgent,
		Limit:       &limit,
	})
	if err != nil {
		return nil, fmt.Errorf("ListSkills: %w", err)
	}
	return resp.Skills, nil
}

// RemoveSkill removes a skill by ID.
func (c *Client) RemoveSkill(ctx context.Context, id string) (bool, error) {
	resp, err := c.api.RemoveSkill(ctx, &pb.RemoveSkillRequest{Id: id})
	if err != nil {
		return false, fmt.Errorf("RemoveSkill: %w", err)
	}
	return resp.Success, nil
}

// SearchSkills searches skills using FTS5.
func (c *Client) SearchSkills(ctx context.Context, query, category string, limit int32) ([]*pb.SkillEntry, error) {
	resp, err := c.api.SearchSkills(ctx, &pb.SearchSkillsRequest{
		Query:    query,
		Category: category,
		Limit:    &limit,
	})
	if err != nil {
		return nil, fmt.Errorf("SearchSkills: %w", err)
	}
	return resp.Skills, nil
}

// --- Cron job management ---

// CronJobInfo represents a cron job.
type CronJobInfo struct {
	ID         string
	Name       string
	Schedule   string
	AgentID    string
	Message    string
	Enabled    bool
	CreatedAt  int64
	UpdatedAt  int64
	LastFireAt int64
	NextFireAt int64
}

// CreateCronJob creates a new cron job.
func (c *Client) CreateCronJob(ctx context.Context, name, schedule, agentID, message string, enabled *bool) (*CronJobInfo, error) {
	resp, err := c.cron.CreateCronJob(ctx, &pb.CreateCronJobRequest{
		Name:     name,
		Schedule: schedule,
		AgentId:  agentID,
		Message:  message,
		Enabled:  enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("CreateCronJob: %w", err)
	}
	return protoCronJobToInfo(resp), nil
}

// ListCronJobs lists cron jobs for an agent.
func (c *Client) ListCronJobs(ctx context.Context, agentID string, includeDisabled bool) ([]CronJobInfo, error) {
	resp, err := c.cron.ListCronJobs(ctx, &pb.ListCronJobsRequest{
		AgentId:          agentID,
		IncludeDisabled:  includeDisabled,
	})
	if err != nil {
		return nil, fmt.Errorf("ListCronJobs: %w", err)
	}
	result := make([]CronJobInfo, len(resp.Jobs))
	for i, j := range resp.Jobs {
		result[i] = *protoCronJobToInfo(j)
	}
	return result, nil
}

// UpdateCronJob updates a cron job.
func (c *Client) UpdateCronJob(ctx context.Context, id string, name, schedule, message *string, enabled *bool) (*CronJobInfo, error) {
	resp, err := c.cron.UpdateCronJob(ctx, &pb.UpdateCronJobRequest{
		Id:       id,
		Name:     name,
		Schedule: schedule,
		Message:  message,
		Enabled:  enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("UpdateCronJob: %w", err)
	}
	return protoCronJobToInfo(resp), nil
}

// DeleteCronJob deletes a cron job.
func (c *Client) DeleteCronJob(ctx context.Context, id string) error {
	_, err := c.cron.DeleteCronJob(ctx, &pb.DeleteCronJobRequest{Id: id})
	if err != nil {
		return fmt.Errorf("DeleteCronJob: %w", err)
	}
	return nil
}

func protoCronJobToInfo(pb *pb.CronJob) *CronJobInfo {
	return &CronJobInfo{
		ID:         pb.Id,
		Name:       pb.Name,
		Schedule:   pb.Schedule,
		AgentID:    pb.AgentId,
		Message:    pb.Message,
		Enabled:    pb.Enabled,
		CreatedAt:  pb.CreatedAt,
		UpdatedAt:  pb.UpdatedAt,
		LastFireAt: pb.LastFireAt,
		NextFireAt: pb.NextFireAt,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalBool(b bool) *bool {
	return &b
}

func optionalInt32(value int32) *int32 {
	return &value
}
