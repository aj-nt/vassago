// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestService_checkDB_RemoveMemory verifies RemoveMemory checks DB health.
func TestService_checkDB_RemoveMemory(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	// Add a memory to remove
	resp, _ := svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "test",
		Key:         "remove_health",
		Content:     "ok",
		Priority:    2,
		SourceAgent: "test",
	})

	// Close DB
	s.Close()

	_, err := svc.RemoveMemory(ctx, &pb.RemoveMemoryRequest{Id: resp.Entry.Id})
	if err == nil {
		t.Fatal("expected error with closed DB, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable code, got %s", st.Code())
	}
}

// TestService_checkDB_EndSession verifies EndSession checks DB health.
func TestService_checkDB_EndSession(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	// Create a session to end
	sess, _ := svc.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "test-agent",
		Source:  "cli",
		Title:   "health-test",
	})

	// Close DB
	s.Close()

	_, err := svc.EndSession(ctx, &pb.EndSessionRequest{SessionId: sess.Id})
	if err == nil {
		t.Fatal("expected error with closed DB, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable code, got %s", st.Code())
	}
}

// TestService_checkDB_Consolidate verifies Consolidate checks DB health.
func TestService_checkDB_Consolidate(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	// Close DB
	s.Close()

	_, err := svc.Consolidate(ctx, &pb.ConsolidateRequest{
		Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED,
	})
	if err == nil {
		t.Fatal("expected error with closed DB, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable code, got %s", st.Code())
	}
}
