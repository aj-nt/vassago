// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago/proto"
)

// newTestService creates a service with a temp store for testing.
func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	svc := NewService(s, nil, nil)
	return svc, func() { s.Close() }
}

func TestConsolidateRPC(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	t.Run("ConsolidateExpired_empty", func(t *testing.T) {
		resp, err := svc.Consolidate(context.Background(), &pb.ConsolidateRequest{
			Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED,
		})
		if err != nil {
			t.Fatalf("Consolidate: %v", err)
		}
		if resp.Removed != 0 {
			t.Errorf("expected 0 removed, got %d", resp.Removed)
		}
	})

	t.Run("GetConsolidationCandidates_empty", func(t *testing.T) {
		resp, err := svc.GetConsolidationCandidates(context.Background(), &pb.GetConsolidationCandidatesRequest{
			Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED,
		})
		if err != nil {
			t.Fatalf("GetConsolidationCandidates: %v", err)
		}
		if len(resp.Candidates) != 0 {
			t.Errorf("expected 0 candidates, got %d", len(resp.Candidates))
		}
	})

	t.Run("ConsolidateExpired_with_data", func(t *testing.T) {
		// Add an expired memory
		_, err := svc.store.AddMemory(store.AddMemoryParams{
			Target:      "memory",
			Category:    "environment",
			Key:         "expired-entry",
			Content:     "old content",
			Priority:    3,
			SourceAgent: "test",
			ExpiresAt:   1000, // Past
		})
		if err != nil {
			t.Fatalf("AddMemory: %v", err)
		}

		resp, err := svc.Consolidate(context.Background(), &pb.ConsolidateRequest{
			Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED,
		})
		if err != nil {
			t.Fatalf("Consolidate: %v", err)
		}
		if resp.Removed != 1 {
			t.Errorf("expected 1 removed, got %d", resp.Removed)
		}
	})

	t.Run("Category_required", func(t *testing.T) {
		_, err := svc.Consolidate(context.Background(), &pb.ConsolidateRequest{
			Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_CATEGORY,
		})
		if err == nil {
			t.Error("expected error for CATEGORY scope without category")
		}
	})
}

func TestSearchSessionsRPC(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	t.Run("Empty_query_rejected", func(t *testing.T) {
		_, err := svc.SearchSessions(context.Background(), &pb.SearchSessionsRequest{
			Query: "",
		})
		if err == nil {
			t.Error("expected error for empty query")
		}
	})

	t.Run("Search_existing_session", func(t *testing.T) {
		// Create a session first
		_, err := svc.CreateSession(context.Background(), &pb.CreateSessionRequest{
			AgentId: "test-agent",
			Source:  "cli",
			Title:   "Kore Extraction Project",
		})
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		resp, err := svc.SearchSessions(context.Background(), &pb.SearchSessionsRequest{
			Query: "Kore",
		})
		if err != nil {
			t.Fatalf("SearchSessions: %v", err)
		}
		if len(resp.Sessions) != 1 {
			t.Errorf("expected 1 session, got %d", len(resp.Sessions))
		} else if resp.Sessions[0].Title != "Kore Extraction Project" {
			t.Errorf("expected title 'Kore Extraction Project', got %s", resp.Sessions[0].Title)
		}
	})
}
