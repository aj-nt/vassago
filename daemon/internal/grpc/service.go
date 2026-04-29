package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// protoToConsolidateScope converts a proto ConsolidateScope enum to the store type.
func protoToConsolidateScope(scope pb.ConsolidateScope) store.ConsolidationScope {
	switch scope {
	case pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL:
		return store.ConsolidateScopeAll
	case pb.ConsolidateScope_CONSOLIDATE_SCOPE_OBSERVATIONS:
		return store.ConsolidateScopeObservations
	case pb.ConsolidateScope_CONSOLIDATE_SCOPE_CATEGORY:
		return store.ConsolidateScopeCategory
	case pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED:
		return store.ConsolidateScopeExpired
	default:
		return store.ConsolidateScopeAll
	}
}

// Service implements the Mnemosyne gRPC service.
type Service struct {
	pb.UnimplementedMnemosyneServer
	store *store.Store
	hub  *pubsub.Hub
}

// NewService creates a new gRPC service backed by the given store.
func NewService(s *store.Store, h *pubsub.Hub) *Service {
	return &Service{store: s, hub: h}
}

// --- Memory CRUD ---

func (s *Service) AddMemory(ctx context.Context, req *pb.AddMemoryRequest) (*pb.AddMemoryResponse, error) {
	if req.Target == "" {
		return nil, status.Error(codes.InvalidArgument, "target is required")
	}
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	if req.SourceAgent == "" {
		return nil, status.Error(codes.InvalidArgument, "source_agent is required")
	}

	entry, err := s.store.AddMemory(store.AddMemoryParams{
		Target:      req.Target,
		Category:    req.Category,
		Key:         req.Key,
		Content:     req.Content,
		Priority:    req.Priority,
		SourceAgent: req.SourceAgent,
		ExpiresAt:   req.ExpiresAt,
	})
	if err != nil {
		slog.Error("AddMemory failed", "error", err, "target", req.Target, "key", req.Key)
		return nil, status.Errorf(codes.Internal, "add memory: %v", err)
	}

	// Publish event to subscribers
	if s.hub != nil {
		s.hub.Publish(pubsub.Event{
			Type:     pb.UpdateEvent_ADDED,
			Target:   req.Target,
			Category: req.Category,
			Key:      req.Key,
			AgentID:  req.SourceAgent,
			Entry:    memoryEntryToProto(entry),
		})
	}

	return &pb.AddMemoryResponse{Entry: memoryEntryToProto(entry)}, nil
}

func (s *Service) GetMemory(ctx context.Context, req *pb.GetMemoryRequest) (*pb.MemoryEntry, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	entry, err := s.store.GetMemoryByID(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "memory not found: %s", req.Id)
	}

	return memoryEntryToProto(entry), nil
}

func (s *Service) UpdateMemory(ctx context.Context, req *pb.UpdateMemoryRequest) (*pb.MemoryEntry, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// Get existing entry
	existing, err := s.store.GetMemoryByID(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "memory not found: %s", req.Id)
	}

	// Apply partial updates
	target := existing.Target
	category := existing.Category
	content := existing.Content
	priority := existing.Priority
	expiresAt := existing.ExpiresAt

	if req.Category != nil {
		category = *req.Category
	}
	if req.Content != nil {
		content = *req.Content
	}
	if req.Priority != nil {
		priority = *req.Priority
	}
	if req.ExpiresAt != nil {
		expiresAt = *req.ExpiresAt
	}

	// Upsert by target+key (overwrites existing)
	entry, err := s.store.AddMemory(store.AddMemoryParams{
		Target:      target,
		Category:    category,
		Key:         existing.Key,
		Content:     content,
		Priority:    priority,
		SourceAgent: existing.SourceAgent,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		slog.Error("UpdateMemory failed", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "update memory: %v", err)
	}

	return memoryEntryToProto(entry), nil
}

func (s *Service) RemoveMemory(ctx context.Context, req *pb.RemoveMemoryRequest) (*pb.RemoveMemoryResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	deleted, err := s.store.RemoveMemory(req.Id)
	if err != nil {
		slog.Error("RemoveMemory failed", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "remove memory: %v", err)
	}

	return &pb.RemoveMemoryResponse{Deleted: deleted}, nil
}

func (s *Service) ListMemories(ctx context.Context, req *pb.ListMemoriesRequest) (*pb.MemoryList, error) {
	var minPriority int32
	if req.MinPriority != nil {
		minPriority = *req.MinPriority
	}

	entries, err := s.store.ListMemories(req.Target, "", minPriority)
	if err != nil {
		slog.Error("ListMemories failed", "error", err)
		return nil, status.Errorf(codes.Internal, "list memories: %v", err)
	}

	pbEntries := make([]*pb.MemoryEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = memoryEntryToProto(e)
	}

	return &pb.MemoryList{Entries: pbEntries}, nil
}

// --- Search ---

func (s *Service) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	var limit int
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	entries, err := s.store.Search(req.Query, req.Target, limit)
	if err != nil {
		slog.Error("Search failed", "error", err, "query", req.Query)
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}

	pbEntries := make([]*pb.MemoryEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = memoryEntryToProto(e)
	}

	return &pb.SearchResponse{Entries: pbEntries}, nil
}

// --- Hot Block ---

func (s *Service) GetHotBlock(ctx context.Context, req *pb.HotBlockRequest) (*pb.HotBlockResponse, error) {
	if req.Target == "" {
		return nil, status.Error(codes.InvalidArgument, "target is required")
	}

	config := store.HotBlockConfig{
		CharLimit:     req.CharLimit,
		BoostPriority: 2, // Default
	}

	if req.Config != nil {
		config.BoostCategories = req.Config.BoostCategories
		config.SuppressCategories = req.Config.SuppressCategories
		if req.Config.BoostPriority > 0 {
			config.BoostPriority = req.Config.BoostPriority
		}
	}

	if config.CharLimit <= 0 {
		config.CharLimit = 4000 // Default from DESIGN.md
	}

	result, err := s.store.ComputeHotBlock(req.Target, config)
	if err != nil {
		slog.Error("GetHotBlock failed", "error", err, "target", req.Target)
		return nil, status.Errorf(codes.Internal, "compute hot block: %v", err)
	}

	return &pb.HotBlockResponse{
		Content:    result.Content,
		CharCount:  result.CharCount,
		EntryCount: result.EntryCount,
		ComputedAt: time.Now().Unix(),
	}, nil
}

// --- Session Lifecycle ---

func (s *Service) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.Session, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	sess, err := s.store.CreateSession(req.AgentId, req.Source, req.Title)
	if err != nil {
		slog.Error("CreateSession failed", "error", err, "agent_id", req.AgentId)
		return nil, status.Errorf(codes.Internal, "create session: %v", err)
	}

	return sessionToProto(sess), nil
}

func (s *Service) AddMessages(ctx context.Context, req *pb.AddMessagesRequest) (*pb.Session, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if len(req.Messages) == 0 {
		return nil, status.Error(codes.InvalidArgument, "messages must not be empty")
	}

	msgs := make([]store.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = store.Message{
			SessionID:  req.SessionId,
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallId,
			ToolName:   m.ToolName,
			Timestamp:  m.Timestamp,
		}
	}

	if err := s.store.AddMessages(req.SessionId, msgs); err != nil {
		slog.Error("AddMessages failed", "error", err, "session_id", req.SessionId)
		return nil, status.Errorf(codes.Internal, "add messages: %v", err)
	}

	sess, err := s.store.GetSession(req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get session after add messages: %v", err)
	}

	return sessionToProto(sess), nil
}

func (s *Service) EndSession(ctx context.Context, req *pb.EndSessionRequest) (*pb.EndSessionResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	if err := s.store.EndSession(req.SessionId); err != nil {
		slog.Error("EndSession failed", "error", err, "session_id", req.SessionId)
		return nil, status.Errorf(codes.Internal, "end session: %v", err)
	}

	return &pb.EndSessionResponse{Ended: true}, nil
}

func (s *Service) GetSession(ctx context.Context, req *pb.GetSessionRequest) (*pb.Session, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	sess, err := s.store.GetSession(req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "session not found: %s", req.SessionId)
	}

	return sessionToProto(sess), nil
}

// --- Consolidation ---

func (s *Service) Consolidate(ctx context.Context, req *pb.ConsolidateRequest) (*pb.ConsolidateResponse, error) {
	var batchSize int
	if req.BatchSize != nil {
		batchSize = int(*req.BatchSize)
	}

	scope := protoToConsolidateScope(req.Scope)
	var category string
	if req.Category != nil {
		category = *req.Category
	}
	result, err := s.store.Consolidate(scope, category, batchSize)
	if err != nil {
		slog.Error("Consolidate failed", "error", err, "scope", req.Scope)
		return nil, status.Errorf(codes.Internal, "consolidate: %v", err)
	}

	return &pb.ConsolidateResponse{
		Removed: int32(result.Removed),
		Merged:  int32(result.Merged),
		Summary: result.Summary,
	}, nil
}

func (s *Service) GetConsolidationCandidates(ctx context.Context, req *pb.GetConsolidationCandidatesRequest) (*pb.ConsolidationCandidatesResponse, error) {
	var batchSize int
	if req.BatchSize != nil {
		batchSize = int(*req.BatchSize)
	}

	scope := protoToConsolidateScope(req.Scope)
	var category string
	if req.Category != nil {
		category = *req.Category
	}
	candidates, err := s.store.GetConsolidationCandidates(scope, category, batchSize)
	if err != nil {
		slog.Error("GetConsolidationCandidates failed", "error", err, "scope", req.Scope)
		return nil, status.Errorf(codes.Internal, "consolidation candidates: %v", err)
	}

	pbCandidates := make([]*pb.ConsolidationCandidate, len(candidates))
	for i, c := range candidates {
		pbCandidates[i] = &pb.ConsolidationCandidate{
			Entry:  memoryEntryToProto(c.Entry),
			Reason: c.Reason,
		}
	}

	return &pb.ConsolidationCandidatesResponse{Candidates: pbCandidates}, nil
}

// SearchSessions searches session titles matching the query.
func (s *Service) SearchSessions(ctx context.Context, req *pb.SearchSessionsRequest) (*pb.SearchSessionsResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	var limit int
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	sessions, err := s.store.SearchSessions(req.Query, limit)
	if err != nil {
		slog.Error("SearchSessions failed", "error", err, "query", req.Query)
		return nil, status.Errorf(codes.Internal, "search sessions: %v", err)
	}

	pbSessions := make([]*pb.Session, len(sessions))
	for i, sess := range sessions {
		pbSessions[i] = sessionToProto(sess)
	}

	return &pb.SearchSessionsResponse{Sessions: pbSessions}, nil
}

// ListSessions returns the most recently active sessions.
func (s *Service) ListSessions(ctx context.Context, req *pb.ListSessionsRequest) (*pb.ListSessionsResponse, error) {
	var limit int
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	sessions, err := s.store.ListRecentSessions(limit)
	if err != nil {
		slog.Error("ListSessions failed", "error", err)
		return nil, status.Errorf(codes.Internal, "list sessions: %v", err)
	}

	pbSessions := make([]*pb.Session, len(sessions))
	for i, sess := range sessions {
		pbSessions[i] = sessionToProto(sess)
	}

	return &pb.ListSessionsResponse{Sessions: pbSessions}, nil
}

// --- Pub/Sub ---


// --- Conversion helpers ---

func memoryEntryToProto(e *store.MemoryEntry) *pb.MemoryEntry {
	return &pb.MemoryEntry{
		Id:          e.ID,
		Target:      e.Target,
		Category:    e.Category,
		Key:         e.Key,
		Content:     e.Content,
		Priority:    e.Priority,
		SourceAgent: e.SourceAgent,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		AccessedAt:  e.AccessedAt,
		ExpiresAt:   e.ExpiresAt,
	}
}

func sessionToProto(sess *store.Session) *pb.Session {
	return &pb.Session{
		Id:           sess.ID,
		AgentId:      sess.AgentID,
		Source:       sess.Source,
		Title:        sess.Title,
		CreatedAt:    sess.CreatedAt,
		LastActiveAt: sess.LastActiveAt,
		MessageCount: sess.MessageCount,
	}
}



// Subscribe implements the server-streaming Subscribe RPC.
// Agent subscribes to memory change events. The stream stays open
// until the client disconnects or cancels.
func (s *Service) Subscribe(req *pb.SubscribeRequest, stream pb.Mnemosyne_SubscribeServer) error {
	agentID := req.AgentId
	if agentID == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Build subscription
	targets := make(map[string]bool)
	for _, t := range req.Targets {
		targets[t] = true
	}
	categories := make(map[string]bool)
	for _, c := range req.Categories {
		categories[c] = true
	}

	subID := fmt.Sprintf("%s-%d", agentID, time.Now().UnixNano())
	sub := &pubsub.Subscription{
		ID:         subID,
		AgentID:    agentID,
		Targets:    targets,
		Categories: categories,
		KeyPrefixes: req.KeyPrefixes,
		Ch:         make(chan pubsub.Event, 100),
	}

	s.hub.Subscribe(sub)
	defer s.hub.Unsubscribe(subID)

	slog.Info("agent subscribed", "agent_id", agentID, "sub_id", subID)

	// Stream events until client disconnects
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			slog.Info("agent disconnected", "agent_id", agentID, "sub_id", subID)
			return ctx.Err()
		case event := <-sub.Ch:
			updateEvent := &pb.UpdateEvent{
				EventType: event.Type,
				Target:    event.Target,
				Category:  event.Category,
				Key:       event.Key,
				AgentId:   event.AgentID,
			}
			if event.Entry != nil {
				updateEvent.Entry = event.Entry
			}
			if err := stream.Send(updateEvent); err != nil {
				slog.Error("failed to send update", "agent_id", agentID, "error", err)
				return err
			}
		}
	}
}

// RegisterAgent registers an agent with the daemon.
func (s *Service) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	agentID := req.AgentId
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	_, err := s.store.EnsureAgent(agentID, req.Name, req.Role)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ensure agent: %v", err)
	}

	slog.Info("agent registered", "agent_id", agentID, "name", req.Name, "role", req.Role)

	return &pb.RegisterAgentResponse{
		AgentId: agentID,
		IsNew:   false, // TODO: track first-seen vs returning agent
	}, nil
}

// Heartbeat receives a keep-alive from an agent.
func (s *Service) Heartbeat(ctx context.Context, req *pb.AgentId) (*pb.Empty, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Just log it — the real disconnect detection is via the Subscribe stream
	slog.Debug("heartbeat received", "agent_id", req.AgentId)
	return &pb.Empty{}, nil
}
