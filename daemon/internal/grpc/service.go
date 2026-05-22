// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"github.com/aj-nt/vassago/daemon/internal/pubsub"
	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago-sdk/proto"
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

// Service implements the Vassago gRPC service.
type Service struct {
	pb.UnimplementedVassagoServer
	store     *store.Store
	hub       *pubsub.Hub
	messaging *MessagingService

	// OnMessagesAdded is called after messages are persisted to a session.
	// The daemon wires this to trigger debounced memory extraction.
	// May be nil (no extraction configured).
	OnMessagesAdded func(sessionID, owner string)

	// summaryClient is an optional LLM client used to generate session
	// summaries when EndSession is called. If nil, falls back to a
	// metadata-based summary (message count + duration).
	summaryClient LLMClient
}

// LLMClient is the interface for generating text from prompts.
// extraction.HTTPClient satisfies this.
type LLMClient interface {
	Complete(prompt string) (string, error)
}

// SetSummaryClient sets the LLM client for session summary generation.
func (s *Service) SetSummaryClient(c LLMClient) {
	s.summaryClient = c
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

func memoryEntriesToProto(entries []*store.MemoryEntry) []*pb.MemoryEntry {
	pbEntries := make([]*pb.MemoryEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = memoryEntryToProto(e)
	}
	return pbEntries
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
