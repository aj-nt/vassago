// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Memory CRUD ---

func (s *Service) AddMemory(ctx context.Context, req *pb.AddMemoryRequest) (*pb.AddMemoryResponse, error) {
	if err := s.checkDB(); err != nil {
		return nil, err
	}
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
		Owner:       ownerFromCtx(ctx),
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
	if err := s.checkDB(); err != nil {
		return nil, err
	}
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	entry, err := s.store.UpdateMemory(req.Id, ownerFromCtx(ctx), store.MemoryPatch{
		Category:  req.Category,
		Content:   req.Content,
		Priority:  req.Priority,
		ExpiresAt: req.ExpiresAt,
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

	deleted, err := s.store.RemoveMemory(req.Id, ownerFromCtx(ctx))
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

	entries, err := s.store.ListMemories(ownerFromCtx(ctx), req.Target, "", minPriority)
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