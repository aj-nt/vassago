// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
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
	store    *store.Store
	hub      *pubsub.Hub
	messaging *MessagingService
}

// NewService creates a new gRPC service backed by the given store.
func NewService(s *store.Store, h *pubsub.Hub, m *MessagingService) *Service {
	return &Service{store: s, hub: h, messaging: m}
}

// checkDB verifies database connectivity before critical operations.
func (s *Service) checkDB() error {
	if err := s.store.Ping(); err != nil {
		return status.Errorf(codes.Unavailable, "database unavailable: %v", err)
	}
	return nil
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