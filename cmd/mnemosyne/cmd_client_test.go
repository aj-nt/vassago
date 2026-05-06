// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"os"
	"testing"

	memoclient "github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// testMnemoClient wraps NullMnemo and provides controlled responses for testing.
type testMnemoClient struct {
	memoclient.NullMnemo
	memories      []*pb.MemoryEntry
	sessions      []memoclient.SessionInfo
	sessionDetail *memoclient.SessionDetail
}

func (t *testMnemoClient) AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error) {
	entry := &pb.MemoryEntry{
		Id:          "test-id-" + key,
		Target:      target,
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: sourceAgent,
	}
	t.memories = append(t.memories, entry)
	return entry, nil
}

func (t *testMnemoClient) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	for _, m := range t.memories {
		if m.Id == id {
			return m, nil
		}
	}
	return nil, os.ErrNotExist
}

func (t *testMnemoClient) SearchMemories(ctx context.Context, query string, target string, limit int32) ([]*pb.MemoryEntry, error) {
	return t.memories, nil
}

func (t *testMnemoClient) ListMemories(ctx context.Context, target string, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	return t.memories, nil
}

func (t *testMnemoClient) RemoveMemory(ctx context.Context, id string) error {
	for i, m := range t.memories {
		if m.Id == id {
			t.memories = append(t.memories[:i], t.memories[i+1:]...)
			return nil
		}
	}
	return os.ErrNotExist
}

func (t *testMnemoClient) HealthCheck(ctx context.Context) error {
	return nil
}

func (t *testMnemoClient) ListRecentSessions(ctx context.Context, limit int32) ([]memoclient.SessionInfo, error) {
	return t.sessions, nil
}

func (t *testMnemoClient) GetSession(ctx context.Context, sessionID string) (*memoclient.SessionDetail, error) {
	if t.sessionDetail != nil {
		return t.sessionDetail, nil
	}
	return &memoclient.SessionDetail{
		ID:           sessionID,
		Title:        "Test Session",
		Source:       "test",
		MessageCount: 1,
	}, nil
}

func (t *testMnemoClient) Close() error {
	return nil
}

// --- Interface satisfaction tests ---

func TestTestMnemoClientSatisfiesInterface(t *testing.T) {
	// Compile-time check that testMnemoClient implements MnemoClient
	var _ memoclient.MnemoClient = &testMnemoClient{}
}

func TestNullMnemoSatisfiesInterface(t *testing.T) {
	// Compile-time check that NullMnemo implements MnemoClient
	var _ memoclient.MnemoClient = &memoclient.NullMnemo{}
}

// --- NullMnemo method tests ---

func TestNullMnemo_AddMemory(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	_, err := n.AddMemory(ctx, "memory", "environment", "key", "content", 3, "test")
	// NullMnemo returns "mnemo not connected" for write operations
	if err == nil {
		t.Error("NullMnemo.AddMemory should return error when daemon unavailable")
	}
}

func TestNullMnemo_SearchMemories(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	entries, err := n.SearchMemories(ctx, "query", "memory", 10)
	if err != nil {
		t.Errorf("NullMnemo.SearchMemories returned unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("NullMnemo.SearchMemories should return nil, got %v", entries)
	}
}

func TestNullMnemo_HealthCheck(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	err := n.HealthCheck(ctx)
	// NullMnemo returns "mnemo not connected" — there's no daemon to check
	if err == nil {
		t.Error("NullMnemo.HealthCheck should return error when daemon unavailable")
	}
}

func TestNullMnemo_ListRecentSessions(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	sessions, err := n.ListRecentSessions(ctx, 10)
	if err != nil {
		t.Errorf("NullMnemo.ListRecentSessions returned unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("NullMnemo.ListRecentSessions should return nil, got %v", sessions)
	}
}

func TestNullMnemo_GetSession(t *testing.T) {
	n := &memoclient.NullMnemo{}
	ctx := context.Background()

	detail, err := n.GetSession(ctx, "test-id")
	if err != nil {
		t.Errorf("NullMnemo.GetSession returned unexpected error: %v", err)
	}
	if detail != nil {
		t.Errorf("NullMnemo.GetSession should return nil, got %v", detail)
	}
}

func TestNullMnemo_Close(t *testing.T) {
	n := &memoclient.NullMnemo{}
	err := n.Close()
	if err != nil {
		t.Errorf("NullMnemo.Close should return nil, got %v", err)
	}
}