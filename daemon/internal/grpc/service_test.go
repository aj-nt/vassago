// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testStore creates a temp store for service unit tests.
func testStoreSvc(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestService_AddMemory(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	resp, err := svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "environment",
		Key:         "python_path",
		Content:     "Python 3.11 at /usr/local/bin/python3",
		Priority:    3,
		SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("AddMemory: %v", err)
	}
	if resp.Entry.Id == "" {
		t.Error("entry ID is empty")
	}
	if resp.Entry.Target != "memory" {
		t.Errorf("target = %q, want %q", resp.Entry.Target, "memory")
	}
	if resp.Entry.Content != "Python 3.11 at /usr/local/bin/python3" {
		t.Errorf("content = %q, want expected", resp.Entry.Content)
	}
}

func TestService_AddMemory_validation(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	tests := []struct {
		name string
		req  *pb.AddMemoryRequest
	}{
		{"empty target", &pb.AddMemoryRequest{Key: "k", SourceAgent: "a"}},
		{"empty key", &pb.AddMemoryRequest{Target: "memory", SourceAgent: "a"}},
		{"empty source_agent", &pb.AddMemoryRequest{Target: "memory", Key: "k"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddMemory(ctx, tt.req)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestService_GetMemory(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	addResp, _ := svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target: "memory", Category: "quirk", Key: "test_key",
		Content: "test content", Priority: 3, SourceAgent: "agent-1",
	})

	getResp, err := svc.GetMemory(ctx, &pb.GetMemoryRequest{Id: addResp.Entry.Id})
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if getResp.Content != "test content" {
		t.Errorf("content = %q, want %q", getResp.Content, "test content")
	}
}

func TestService_GetMemory_notFound(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	_, err := svc.GetMemory(ctx, &pb.GetMemoryRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestService_RemoveMemory(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	addResp, _ := svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target: "memory", Category: "env", Key: "rm_test",
		Content: "to be removed", Priority: 2, SourceAgent: "agent-1",
	})

	rmResp, err := svc.RemoveMemory(ctx, &pb.RemoveMemoryRequest{Id: addResp.Entry.Id})
	if err != nil {
		t.Fatalf("RemoveMemory: %v", err)
	}
	if !rmResp.Deleted {
		t.Error("expected deleted = true")
	}

	_, err = svc.GetMemory(ctx, &pb.GetMemoryRequest{Id: addResp.Entry.Id})
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestService_ListMemories(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "memory", Category: "env", Key: "k1", Content: "env note", Priority: 3, SourceAgent: "a1"})     //nolint:errcheck
	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "memory", Category: "quirk", Key: "k2", Content: "quirk note", Priority: 2, SourceAgent: "a1"})  //nolint:errcheck
	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "user", Category: "user", Key: "name", Content: "AJ", Priority: 4, SourceAgent: "a1"})         //nolint:errcheck

	resp, err := svc.ListMemories(ctx, &pb.ListMemoriesRequest{Target: "memory"})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Errorf("expected 2 memory entries, got %d", len(resp.Entries))
	}
}

func TestService_Search(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "memory", Category: "env", Key: "python", Content: "Python 3.11 on venv", Priority: 3, SourceAgent: "a1"})      //nolint:errcheck
	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "memory", Category: "project", Key: "mnemosyne", Content: "Memory daemon in Go", Priority: 2, SourceAgent: "a1"}) //nolint:errcheck

	resp, err := svc.Search(ctx, &pb.SearchRequest{Query: "Python venv"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Entries) == 0 {
		t.Error("expected search results, got 0")
	}
}

func TestService_Search_emptyQuery(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	_, err := svc.Search(ctx, &pb.SearchRequest{Query: ""})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestService_GetHotBlock(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "memory", Category: "environment", Key: "python", Content: "Python 3.11", Priority: 3, SourceAgent: "a1"}) //nolint:errcheck
	svc.AddMemory(ctx, &pb.AddMemoryRequest{Target: "memory", Category: "project", Key: "mnemosyne", Content: "Memory daemon", Priority: 2, SourceAgent: "a1"})    //nolint:errcheck

	resp, err := svc.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    "memory",
		CharLimit: 4000,
	})
	if err != nil {
		t.Fatalf("GetHotBlock: %v", err)
	}
	if resp.Content == "" {
		t.Error("hot block content is empty")
	}
	if resp.EntryCount == 0 {
		t.Error("hot block has no entries")
	}
}

func TestService_GetHotBlock_empty(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	resp, err := svc.GetHotBlock(ctx, &pb.HotBlockRequest{
		Target:    "memory",
		CharLimit: 4000,
	})
	if err != nil {
		t.Fatalf("GetHotBlock on empty DB: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("expected empty content, got %q", resp.Content)
	}
	if resp.EntryCount != 0 {
		t.Errorf("expected 0 entries, got %d", resp.EntryCount)
	}
}

func TestService_CreateSession(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	sess, err := svc.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "agent-001",
		Source:  "cli",
		Title:   "Test Session",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.Id == "" {
		t.Error("session ID is empty")
	}
	if sess.AgentId != "agent-001" {
		t.Errorf("agent_id = %q, want %q", sess.AgentId, "agent-001")
	}
}

func TestService_CreateSession_validation(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	_, err := svc.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "",
		Source:  "cli",
		Title:   "Test",
	})
	if err == nil {
		t.Error("expected error for empty agent_id")
	}
}

func TestService_AddMessages(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "agent-001", Source: "cli", Title: "Chat",
	})

	updated, err := svc.AddMessages(ctx, &pb.AddMessagesRequest{
		SessionId: sess.Id,
		Messages: []*pb.Message{
			{Role: "user", Content: "Hello", Timestamp: 1700000000},
			{Role: "assistant", Content: "Hi there!", Timestamp: 1700000001},
		},
	})
	if err != nil {
		t.Fatalf("AddMessages: %v", err)
	}
	if updated.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", updated.MessageCount)
	}
}

func TestService_AddMessages_empty(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "agent-001", Source: "cli", Title: "Chat",
	})

	_, err := svc.AddMessages(ctx, &pb.AddMessagesRequest{
		SessionId: sess.Id,
		Messages:  []*pb.Message{},
	})
	if err == nil {
		t.Error("expected error for empty messages")
	}
}

func TestService_EndSession(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	sess, _ := svc.CreateSession(ctx, &pb.CreateSessionRequest{
		AgentId: "agent-001", Source: "cli", Title: "Test",
	})

	resp, err := svc.EndSession(ctx, &pb.EndSessionRequest{SessionId: sess.Id})
	if err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if !resp.Ended {
		t.Error("expected ended = true")
	}
}

func TestService_UpdateMemory(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	addResp, _ := svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target: "memory", Category: "env", Key: "up_test",
		Content: "original", Priority: 2, SourceAgent: "a1",
	})

	newContent := "updated content"
	updated, err := svc.UpdateMemory(ctx, &pb.UpdateMemoryRequest{
		Id:      addResp.Entry.Id,
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}
	if updated.Content != "updated content" {
		t.Errorf("content = %q, want %q", updated.Content, "updated content")
	}
	if updated.Key != "up_test" {
		t.Errorf("key changed from up_test to %q", updated.Key)
	}
}

func TestService_Consolidate_expired(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	// Add an expired memory
	if _, err := svc.AddMemory(ctx, &pb.AddMemoryRequest{ //nolint:errcheck
		Target:      "memory",
		Category:    "environment",
		Key:         "expired-entry",
		Content:     "old content",
		Priority:    3,
		SourceAgent: "test",
		ExpiresAt:   1000, // Past
	}); err != nil {
		t.Fatalf("AddMemory: %v", err)
	}

	resp, err := svc.Consolidate(ctx, &pb.ConsolidateRequest{
		Scope: pb.ConsolidateScope_CONSOLIDATE_SCOPE_EXPIRED,
	})
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if resp.Removed != 1 {
		t.Errorf("expected 1 removed, got %d", resp.Removed)
	}
	if resp.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestService_checkDB(t *testing.T) {
	s := testStoreSvc(t)
	svc := NewService(s, nil, nil)
	ctx := context.Background()

	// Healthy DB should succeed
	resp, err := svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "test",
		Key:         "health",
		Content:     "ok",
		Priority:    3,
		SourceAgent: "test",
	})
	if err != nil {
		t.Fatalf("AddMemory with healthy DB: %v", err)
	}
	if resp.Entry.Id == "" {
		t.Error("expected entry ID")
	}

	// Closed DB should return Unavailable
	s.Close()
	_, err = svc.AddMemory(ctx, &pb.AddMemoryRequest{
		Target:      "memory",
		Category:    "test",
		Key:         "health2",
		Content:     "ok",
		Priority:    3,
		SourceAgent: "test",
	})
	if err == nil {
		t.Fatal("expected error with closed DB, got nil")
	}
	// Should be Unavailable code, not Internal
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("expected Unavailable code, got %s", st.Code())
	}
}