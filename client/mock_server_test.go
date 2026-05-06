// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// mockMnemosyneServer implements pb.MnemosyneServer with controllable responses.
// It embeds UnimplementedMnemosyneServer and overrides the methods we need for testing.
type mockMnemosyneServer struct {
	pb.UnimplementedMnemosyneServer

	mu       sync.Mutex
	memories map[string]*pb.MemoryEntry
	sessions map[string]*pb.Session
	agents   map[string]*pb.RegisterAgentResponse
	todos    map[string]*pb.TodoItem
	cronJobs map[string]*pb.CronJob
	nextID   int
}

func newMockMnemosyneServer() *mockMnemosyneServer {
	return &mockMnemosyneServer{
		memories: make(map[string]*pb.MemoryEntry),
		sessions: make(map[string]*pb.Session),
		agents:   make(map[string]*pb.RegisterAgentResponse),
		todos:    make(map[string]*pb.TodoItem),
		cronJobs: make(map[string]*pb.CronJob),
		nextID:   1,
	}
}

func (m *mockMnemosyneServer) genID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("mock-%d", m.nextID)
	m.nextID++
	return id
}

func (m *mockMnemosyneServer) AddMemory(ctx context.Context, req *pb.AddMemoryRequest) (*pb.AddMemoryResponse, error) {
	if req.Target == "" || req.Key == "" || req.SourceAgent == "" {
		return nil, fmt.Errorf("validation: target, key, and source_agent required")
	}
	id := m.genID()
	entry := &pb.MemoryEntry{
		Id:          id,
		Target:      req.Target,
		Category:    req.Category,
		Key:         req.Key,
		Content:     req.Content,
		Priority:    req.Priority,
		SourceAgent: req.SourceAgent,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	m.mu.Lock()
	m.memories[id] = entry
	m.mu.Unlock()
	return &pb.AddMemoryResponse{Entry: entry}, nil
}

func (m *mockMnemosyneServer) GetMemory(ctx context.Context, req *pb.GetMemoryRequest) (*pb.MemoryEntry, error) {
	m.mu.Lock()
	entry, ok := m.memories[req.Id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("memory not found: %s", req.Id)
	}
	return entry, nil
}

func (m *mockMnemosyneServer) RemoveMemory(ctx context.Context, req *pb.RemoveMemoryRequest) (*pb.RemoveMemoryResponse, error) {
	m.mu.Lock()
	delete(m.memories, req.Id)
	m.mu.Unlock()
	return &pb.RemoveMemoryResponse{}, nil
}

func (m *mockMnemosyneServer) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var results []*pb.MemoryEntry
	for _, entry := range m.memories {
		if req.Target == "" || entry.Target == req.Target {
			results = append(results, entry)
		}
	}
	return &pb.SearchResponse{Entries: results}, nil
}

func (m *mockMnemosyneServer) ListMemories(ctx context.Context, req *pb.ListMemoriesRequest) (*pb.MemoryList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var entries []*pb.MemoryEntry
	for _, entry := range m.memories {
		if req.Target == "" || entry.Target == req.Target {
			entries = append(entries, entry)
		}
	}
	return &pb.MemoryList{Entries: entries}, nil
}

func (m *mockMnemosyneServer) GetHotBlock(ctx context.Context, req *pb.HotBlockRequest) (*pb.HotBlockResponse, error) {
	return &pb.HotBlockResponse{Content: "mock hot block", CharCount: 15}, nil
}

func (m *mockMnemosyneServer) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.Session, error) {
	id := m.genID()
	session := &pb.Session{
		Id:           id,
		AgentId:      req.AgentId,
		Source:        req.Source,
		Title:         req.Title,
		MessageCount: 0,
		CreatedAt:    time.Now().Unix(),
		LastActiveAt: time.Now().Unix(),
	}
	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()
	return session, nil
}

func (m *mockMnemosyneServer) AddMessages(ctx context.Context, req *pb.AddMessagesRequest) (*pb.Session, error) {
	m.mu.Lock()
	session, ok := m.sessions[req.SessionId]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", req.SessionId)
	}
	session.MessageCount += int32(len(req.Messages))
	return session, nil
}

func (m *mockMnemosyneServer) EndSession(ctx context.Context, req *pb.EndSessionRequest) (*pb.EndSessionResponse, error) {
	m.mu.Lock()
	_, ok := m.sessions[req.SessionId]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", req.SessionId)
	}
	return &pb.EndSessionResponse{}, nil
}

func (m *mockMnemosyneServer) GetSession(ctx context.Context, req *pb.GetSessionRequest) (*pb.Session, error) {
	m.mu.Lock()
	session, ok := m.sessions[req.SessionId]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", req.SessionId)
	}
	return session, nil
}

func (m *mockMnemosyneServer) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if resp, ok := m.agents[req.AgentId]; ok {
		// Already registered — return existing with IsNew=false
		return &pb.RegisterAgentResponse{AgentId: resp.AgentId, IsNew: false}, nil
	}
	resp := &pb.RegisterAgentResponse{AgentId: req.AgentId, IsNew: true}
	m.agents[req.AgentId] = resp
	return resp, nil
}

func (m *mockMnemosyneServer) Heartbeat(ctx context.Context, req *pb.AgentId) (*pb.Empty, error) {
	return &pb.Empty{}, nil
}

func (m *mockMnemosyneServer) Consolidate(ctx context.Context, req *pb.ConsolidateRequest) (*pb.ConsolidateResponse, error) {
	return &pb.ConsolidateResponse{Removed: 0, Merged: 0, Summary: "mock consolidation"}, nil
}

func (m *mockMnemosyneServer) SearchSessions(ctx context.Context, req *pb.SearchSessionsRequest) (*pb.SearchSessionsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sessions []*pb.Session
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return &pb.SearchSessionsResponse{Sessions: sessions}, nil
}

func (m *mockMnemosyneServer) ListSessions(ctx context.Context, req *pb.ListSessionsRequest) (*pb.ListSessionsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sessions []*pb.Session
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return &pb.ListSessionsResponse{Sessions: sessions}, nil
}

func (m *mockMnemosyneServer) AddTodo(ctx context.Context, req *pb.AddTodoRequest) (*pb.TodoItem, error) {
	id := m.genID()
	todo := &pb.TodoItem{
		Id:          id,
		Content:     req.Content,
		Priority:    req.Priority,
		SourceAgent: req.SourceAgent,
		Completed:   false,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	m.mu.Lock()
	m.todos[id] = todo
	m.mu.Unlock()
	return todo, nil
}

func (m *mockMnemosyneServer) ListTodos(req *pb.ListTodosRequest, stream pb.Mnemosyne_ListTodosServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, todo := range m.todos {
		if err := stream.Send(todo); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockMnemosyneServer) CompleteTodo(ctx context.Context, req *pb.CompleteTodoRequest) (*pb.TodoItem, error) {
	m.mu.Lock()
	todo, ok := m.todos[req.Id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("todo not found: %s", req.Id)
	}
	todo.Completed = true
	todo.CompletedAt = time.Now().Unix()
	m.mu.Unlock()
	return todo, nil
}

func (m *mockMnemosyneServer) RemoveTodo(ctx context.Context, req *pb.RemoveTodoRequest) (*pb.Empty, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.todos, req.Id)
	return &pb.Empty{}, nil
}

func (m *mockMnemosyneServer) Ping(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{Status: "ok"}, nil
}

// mockMessagingServer implements pb.MessagingServer with controllable responses.
type mockMessagingServer struct {
	pb.UnimplementedMessagingServer
}

func (m *mockMessagingServer) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	return &pb.SendMessageResponse{MessageId: "msg-1"}, nil
}

func (m *mockMessagingServer) ListChannels(ctx context.Context, req *pb.ListChannelsRequest) (*pb.ListChannelsResponse, error) {
	return &pb.ListChannelsResponse{Channels: []*pb.Channel{
		{Id: "ch-1", Name: "test-channel", Platform: req.Platform},
	}}, nil
}

// mockCronServiceServer implements pb.CronServiceServer with controllable responses.
type mockCronServiceServer struct {
	pb.UnimplementedCronServiceServer
	mu      sync.Mutex
	jobs    map[string]*pb.CronJob
	nextID  int
}

func newMockCronServiceServer() *mockCronServiceServer {
	return &mockCronServiceServer{
		jobs:   make(map[string]*pb.CronJob),
		nextID: 1,
	}
}

func (m *mockCronServiceServer) CreateCronJob(ctx context.Context, req *pb.CreateCronJobRequest) (*pb.CronJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("cron-%d", m.nextID)
	m.nextID++
	job := &pb.CronJob{
		Id:        id,
		Name:      req.Name,
		Schedule:  req.Schedule,
		AgentId:   req.AgentId,
		Message:   req.Message,
		Enabled:   true,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}
	m.jobs[id] = job
	return job, nil
}

func (m *mockCronServiceServer) ListCronJobs(ctx context.Context, req *pb.ListCronJobsRequest) (*pb.CronJobList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var jobs []*pb.CronJob
	for _, job := range m.jobs {
		if req.AgentId == "" || job.AgentId == req.AgentId {
			jobs = append(jobs, job)
		}
	}
	return &pb.CronJobList{Jobs: jobs}, nil
}

func (m *mockCronServiceServer) UpdateCronJob(ctx context.Context, req *pb.UpdateCronJobRequest) (*pb.CronJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[req.Id]
	if !ok {
		return nil, fmt.Errorf("cron job not found: %s", req.Id)
	}
	if req.Name != nil {
		job.Name = *req.Name
	}
	if req.Schedule != nil {
		job.Schedule = *req.Schedule
	}
	if req.Message != nil {
		job.Message = *req.Message
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}
	job.UpdatedAt = time.Now().Unix()
	return job, nil
}

func (m *mockCronServiceServer) DeleteCronJob(ctx context.Context, req *pb.DeleteCronJobRequest) (*pb.Empty, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, req.Id)
	return &pb.Empty{}, nil
}

// setupMockServer creates a bufconn-based gRPC server with mock implementations
// and returns a connected Client.
func setupMockServer(t *testing.T) (*Client, *mockMnemosyneServer, *grpc.ClientConn) {
	t.Helper()

	memoSrv := newMockMnemosyneServer()
	msgSrv := &mockMessagingServer{}
	cronSrv := newMockCronServiceServer()

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	pb.RegisterMnemosyneServer(s, memoSrv)
	pb.RegisterMessagingServer(s, msgSrv)
	pb.RegisterCronServiceServer(s, cronSrv)

	go s.Serve(lis)
	t.Cleanup(s.Stop)

	conn, err := grpc.Dial("bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	client := NewClientFromConn(conn)
	return client, memoSrv, conn
}