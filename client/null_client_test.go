// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"io"
	"testing"

	pb "github.com/aj-nt/vassago/proto"
)

func TestNullMnemo_RegisterAgent(t *testing.T) {
	n := &NullMnemo{}
	id, registered, err := n.RegisterAgent(context.Background(), "test", "test", "assistant")
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
	if registered {
		t.Error("expected registered=false")
	}
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestNullMnemo_Heartbeat(t *testing.T) {
	n := &NullMnemo{}
	if err := n.Heartbeat(context.Background(), "test"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestNullMnemo_Subscribe(t *testing.T) {
	n := &NullMnemo{}
	stream, err := n.Subscribe(context.Background(), &pb.SubscribeRequest{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if stream == nil {
		t.Fatal("expected non-nil stream")
	}
	// Verify stream returns io.EOF on Recv
	_, recvErr := stream.Recv()
	if recvErr != io.EOF {
		t.Errorf("expected io.EOF from Recv, got %v", recvErr)
	}
}

func TestNullMnemo_WriteOperationsReturnNotConnected(t *testing.T) {
	n := &NullMnemo{}
	ctx := context.Background()

	_, err := n.AddMemory(ctx, "t", "c", "k", "v", 1, "a")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("AddMemory: expected 'vassago not connected', got %v", err)
	}

	_, err = n.GetMemory(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("GetMemory: expected 'vassago not connected', got %v", err)
	}

	err = n.RemoveMemory(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("RemoveMemory: expected 'vassago not connected', got %v", err)
	}

	err = n.HealthCheck(ctx)
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("HealthCheck: expected 'vassago not connected', got %v", err)
	}

	_, err = n.SendMessage(ctx, "telegram", "ch", "msg")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("SendMessage: expected 'vassago not connected', got %v", err)
	}

	_, err = n.AddTodo(ctx, "content", 1, "agent")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("AddTodo: expected 'vassago not connected', got %v", err)
	}

	_, err = n.CompleteTodo(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("CompleteTodo: expected 'vassago not connected', got %v", err)
	}

	err = n.RemoveTodo(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("RemoveTodo: expected 'vassago not connected', got %v", err)
	}

	_, err = n.CreateCronJob(ctx, "name", "* * * * *", "agent", "msg", nil)
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("CreateCronJob: expected 'vassago not connected', got %v", err)
	}

	_, err = n.UpdateCronJob(ctx, "id", nil, nil, nil, nil)
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("UpdateCronJob: expected 'vassago not connected', got %v", err)
	}

	err = n.DeleteCronJob(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("DeleteCronJob: expected 'vassago not connected', got %v", err)
	}
}

func TestNullMnemo_ReadOperationsReturnEmpty(t *testing.T) {
	n := &NullMnemo{}
	ctx := context.Background()

	t.Run("SearchMemories", func(t *testing.T) {
		results, err := n.SearchMemories(ctx, "query", "target", 10)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("ListMemories", func(t *testing.T) {
		results, err := n.ListMemories(ctx, "target", nil, nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("GetHotBlock", func(t *testing.T) {
		content, err := n.GetHotBlock(ctx, "target", "agent", 4000, nil, nil, 2)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if content != "" {
			t.Errorf("expected empty content, got %q", content)
		}
	})

	t.Run("SearchSessions", func(t *testing.T) {
		results, err := n.SearchSessions(ctx, "query", 10)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("ListRecentSessions", func(t *testing.T) {
		results, err := n.ListRecentSessions(ctx, 10)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("GetSession", func(t *testing.T) {
		result, err := n.GetSession(ctx, "id")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("ListChannels", func(t *testing.T) {
		results, err := n.ListChannels(ctx, "telegram")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("ListTodos", func(t *testing.T) {
		results, err := n.ListTodos(ctx, false, "agent")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("ListCronJobs", func(t *testing.T) {
		results, err := n.ListCronJobs(ctx, "agent", false)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})
}

func TestNullMnemo_FireAndForgetService(t *testing.T) {
	n := &NullMnemo{}
	ctx := context.Background()

	t.Run("CreateSession", func(t *testing.T) {
		id, err := n.CreateSession(ctx, "agent", "cli", "title")
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if id != "" {
			t.Errorf("expected empty id, got %q", id)
		}
	})

	t.Run("AddMessages", func(t *testing.T) {
		if err := n.AddMessages(ctx, "session", nil); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("EndSession", func(t *testing.T) {
		if err := n.EndSession(ctx, "session"); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("Consolidate", func(t *testing.T) {
		resp, err := n.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL, 100)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if resp == nil {
			t.Error("expected non-nil response")
		}
	})

	t.Run("Close", func(t *testing.T) {
		if err := n.Close(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("IsConnected", func(t *testing.T) {
		if n.IsConnected() {
			t.Error("NullMnemo should report IsConnected=false")
		}
	})
}

func TestNullSubscribeStream(t *testing.T) {
	stream := &nullSubscribeStream{}

	t.Run("Header", func(t *testing.T) {
		md, err := stream.Header()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if md != nil {
			t.Errorf("expected nil metadata, got %v", md)
		}
	})

	t.Run("Trailer", func(t *testing.T) {
		trailer := stream.Trailer()
		if trailer != nil {
			t.Errorf("expected nil trailer, got %v", trailer)
		}
	})

	t.Run("CloseSend", func(t *testing.T) {
		if err := stream.CloseSend(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("Context", func(t *testing.T) {
		ctx := stream.Context()
		if ctx == nil {
			t.Error("expected non-nil context")
		}
	})

	t.Run("SendMsg", func(t *testing.T) {
		if err := stream.SendMsg(nil); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("RecvMsg", func(t *testing.T) {
		if err := stream.RecvMsg(nil); err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
	})
}

// --- Pure helper function tests ---

func TestOptionalString(t *testing.T) {
	t.Run("Empty string", func(t *testing.T) {
		result := optionalString("")
		if result != nil {
			t.Errorf("expected nil for empty string, got %v", result)
		}
	})

	t.Run("Non-empty string", func(t *testing.T) {
		result := optionalString("hello")
		if result == nil {
			t.Error("expected non-nil for non-empty string")
		} else if *result != "hello" {
			t.Errorf("expected 'hello', got %q", *result)
		}
	})
}

func TestOptionalBool(t *testing.T) {
	t.Run("True", func(t *testing.T) {
		result := optionalBool(true)
		if result == nil || !*result {
			t.Error("expected *true")
		}
	})

	t.Run("False", func(t *testing.T) {
		result := optionalBool(false)
		if result == nil {
			t.Error("expected non-nil pointer")
		} else if *result != false {
			t.Errorf("expected false, got true")
		}
	})
}

func TestProtoTodoToItem(t *testing.T) {
	pbItem := &pb.TodoItem{
		Id:          "test-id",
		Content:     "test content",
		Completed:   true,
		Priority:    3,
		SourceAgent: "test-agent",
		CreatedAt:   1000,
		UpdatedAt:   2000,
		CompletedAt: 3000,
	}

	item := protoTodoToItem(pbItem)
	if item == nil {
		t.Fatal("expected non-nil item")
	}
	if item.ID != "test-id" {
		t.Errorf("ID = %q, want %q", item.ID, "test-id")
	}
	if item.Content != "test content" {
		t.Errorf("Content = %q, want %q", item.Content, "test content")
	}
	if !item.Completed {
		t.Error("Completed should be true")
	}
	if item.Priority != 3 {
		t.Errorf("Priority = %d, want 3", item.Priority)
	}
	if item.SourceAgent != "test-agent" {
		t.Errorf("SourceAgent = %q, want %q", item.SourceAgent, "test-agent")
	}
	if item.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d, want 1000", item.CreatedAt)
	}
	if item.UpdatedAt != 2000 {
		t.Errorf("UpdatedAt = %d, want 2000", item.UpdatedAt)
	}
	if item.CompletedAt != 3000 {
		t.Errorf("CompletedAt = %d, want 3000", item.CompletedAt)
	}
}

func TestProtoTodoToItem_Nil(t *testing.T) {
	// protoTodoToItem doesn't handle nil input — callers always pass non-nil.
	// This test just documents that behavior.
	// If nil-safety were needed, we'd add a nil check to the function.
}
