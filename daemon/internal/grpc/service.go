package grpc

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service implements the Mnemosyne gRPC service.
type Service struct {
	pb.UnimplementedMnemosyneServer
	store *store.Store
}

// NewService creates a new gRPC service backed by the given store.
func NewService(s *store.Store) *Service {
	return &Service{store: s}
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

// --- Consolidation (stub — Phase 4) ---

func (s *Service) Consolidate(ctx context.Context, req *pb.ConsolidateRequest) (*pb.ConsolidateResponse, error) {
	// TODO: Phase 4 — implement consolidation logic
	return nil, status.Error(codes.Unimplemented, "consolidation not yet implemented")
}

func (s *Service) GetConsolidationCandidates(ctx context.Context, req *pb.GetConsolidationCandidatesRequest) (*pb.ConsolidationCandidatesResponse, error) {
	// TODO: Phase 4 — implement candidate listing
	return nil, status.Error(codes.Unimplemented, "consolidation candidates not yet implemented")
}

// --- Pub/Sub (stub — Phase 3) ---

func (s *Service) Subscribe(req *pb.SubscribeRequest, stream pb.Mnemosyne_SubscribeServer) error {
	// TODO: Phase 3 — implement pub/sub
	return status.Error(codes.Unimplemented, "subscription not yet implemented")
}

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

