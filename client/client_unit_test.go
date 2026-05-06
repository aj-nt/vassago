// This file is part of Mnemosyne.
// See LICENSE-APache-2.0 for license information.

package client

import (
	"testing"

	pb "github.com/aj-nt/mnemosyne/proto"
)

// --- ProtoTodoToItem tests ---

func TestProtoTodoToItem_NilInput(t *testing.T) {
	result := protoTodoToItem(nil)
	if result != nil {
		t.Errorf("protoTodoToItem(nil) = %v, want nil", result)
	}
}

func TestProtoTodoToItem_CompleteItem(t *testing.T) {
	pbItem := &pb.TodoItem{
		Id:          "todo-123",
		Content:     "Test task",
		Completed:   true,
		Priority:    2,
		SourceAgent: "test-agent",
		CreatedAt:   1700000000,
		UpdatedAt:   1700001000,
		CompletedAt: 1700002000,
	}
	result := protoTodoToItem(pbItem)
	if result.ID != "todo-123" {
		t.Errorf("ID = %q, want %q", result.ID, "todo-123")
	}
	if result.Content != "Test task" {
		t.Errorf("Content = %q, want %q", result.Content, "Test task")
	}
	if !result.Completed {
		t.Error("Completed should be true")
	}
	if result.Priority != 2 {
		t.Errorf("Priority = %d, want %d", result.Priority, 2)
	}
	if result.SourceAgent != "test-agent" {
		t.Errorf("SourceAgent = %q, want %q", result.SourceAgent, "test-agent")
	}
	if result.CreatedAt != 1700000000 {
		t.Errorf("CreatedAt = %d, want %d", result.CreatedAt, 1700000000)
	}
	if result.UpdatedAt != 1700001000 {
		t.Errorf("UpdatedAt = %d, want %d", result.UpdatedAt, 1700001000)
	}
	if result.CompletedAt != 1700002000 {
		t.Errorf("CompletedAt = %d, want %d", result.CompletedAt, 1700002000)
	}
}

func TestProtoTodoToItem_IncompleteItem(t *testing.T) {
	pbItem := &pb.TodoItem{
		Id:          "todo-456",
		Content:     "Pending task",
		Completed:   false,
		Priority:    1,
		SourceAgent: "bot",
		CreatedAt:   1700000000,
		UpdatedAt:   1700000000,
		CompletedAt: 0,
	}
	result := protoTodoToItem(pbItem)
	if result.Completed {
		t.Error("Completed should be false")
	}
	if result.CompletedAt != 0 {
		t.Errorf("CompletedAt = %d, want 0 for incomplete item", result.CompletedAt)
	}
}

// --- OptionalString tests ---

func TestOptionalString_Empty(t *testing.T) {
	result := optionalString("")
	if result != nil {
		t.Errorf("optionalString('') = %v, want nil", result)
	}
}

func TestOptionalString_NonEmpty(t *testing.T) {
	result := optionalString("hello")
	if result == nil {
		t.Error("optionalString('hello') = nil, want non-nil")
	}
	if *result != "hello" {
		t.Errorf("optionalString('hello') = %q, want %q", *result, "hello")
	}
}

// --- OptionalBool tests ---

func TestOptionalBool_True(t *testing.T) {
	result := optionalBool(true)
	if result == nil {
		t.Error("optionalBool(true) = nil, want non-nil")
	}
	if !*result {
		t.Error("optionalBool(true) = false, want true")
	}
}

func TestOptionalBool_False(t *testing.T) {
	result := optionalBool(false)
	if result == nil {
		t.Error("optionalBool(false) = nil, want non-nil")
	}
	if *result {
		t.Error("optionalBool(false) = true, want false")
	}
}

// --- Message and SessionInfo types ---

func TestMessage_Fields(t *testing.T) {
	msg := Message{Role: "user", Content: "hello"}
	if msg.Role != "user" {
		t.Errorf("Role = %q, want %q", msg.Role, "user")
	}
	if msg.Content != "hello" {
		t.Errorf("Content = %q, want %q", msg.Content, "hello")
	}
}

func TestSessionDetail_Fields(t *testing.T) {
	detail := SessionDetail{
		ID:           "session-123",
		Title:        "Test Session",
		Source:       "cli",
		MessageCount: 5,
		CreatedAt:    1700000000,
		LastActiveAt: 1700001000,
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}
	if detail.ID != "session-123" {
		t.Errorf("ID = %q, want %q", detail.ID, "session-123")
	}
	if len(detail.Messages) != 2 {
		t.Errorf("len(Messages) = %d, want %d", len(detail.Messages), 2)
	}
}

func TestSessionInfo_Fields(t *testing.T) {
	info := SessionInfo{
		ID:           "session-456",
		Title:        "Another Session",
		Source:       "discord",
		MessageCount: 10,
		LastActive:   1700002000,
	}
	if info.ID != "session-456" {
		t.Errorf("ID = %q, want %q", info.ID, "session-456")
	}
	if info.Source != "discord" {
		t.Errorf("Source = %q, want %q", info.Source, "discord")
	}
}

func TestTodoItem_Fields(t *testing.T) {
	item := TodoItem{
		ID:          "todo-789",
		Content:     "Buy milk",
		Completed:   false,
		Priority:    1,
		SourceAgent: "test-agent",
		CreatedAt:   1700000000,
		UpdatedAt:   1700001000,
	}
	if item.ID != "todo-789" {
		t.Errorf("ID = %q, want %q", item.ID, "todo-789")
	}
	if item.Completed {
		t.Error("Completed should be false")
	}
}

// --- ConnectMnemo interface test ---

func TestConnectMnemoReturnsInterface(t *testing.T) {
	// Verify ConnectMnemo returns MnemoClient interface type
	// (compile-time check — this function can't connect without a server)
	var _ MnemoClient = (*Client)(nil)
	var _ MnemoClient = (*NullMnemo)(nil)
}