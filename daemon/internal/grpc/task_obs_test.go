// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/aj-nt/vassago/daemon/internal/pubsub"
	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// newTestServiceWithHub creates a service with a real pubsub hub for testing event publishing.
func newTestServiceWithHub(t *testing.T) (*Service, *pubsub.Hub, func()) {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	hub := pubsub.NewHub()
	svc := NewService(s, hub, nil)
	return svc, hub, func() { s.Close() }
}

func TestPublishTaskEvent_CompleteTaskPublishes(t *testing.T) {
	svc, hub, cleanup := newTestServiceWithHub(t)
	defer cleanup()

	// Subscribe to task events
	sub := &pubsub.Subscription{
		ID:      "gaap-observer",
		AgentID: "gaap",
		Targets: map[string]bool{"task": true},
		Ch:      make(chan pubsub.Event, 10),
	}
	hub.Subscribe(sub)

	// Add a task and complete it — verify event is published
	_, err := svc.AddTask(context.Background(), &pb.AddTaskRequest{
		Id:        "task-obs-1",
		AgentType: "static_analysis",
		Goal:      "review code",
		TtlSeconds: 60,
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Claim the task
	_, err = svc.ClaimTask(context.Background(), &pb.ClaimTaskRequest{
		TaskId:  "task-obs-1",
		AgentId: "worker-1",
	})
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// Complete the task — should publish TASK_STATUS_CHANGED
	_, err = svc.CompleteTask(context.Background(), &pb.CompleteTaskRequest{
		TaskId:    "task-obs-1",
		ResultKey: "mem-uuid-1234",
	})
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Verify the event was published
	select {
	case evt := <-sub.Ch:
		if evt.Type != pb.UpdateEvent_TASK_STATUS_CHANGED {
			t.Errorf("expected TASK_STATUS_CHANGED, got %v", evt.Type)
		}
		if evt.Target != "task" {
			t.Errorf("expected target='task', got %q", evt.Target)
		}
		if evt.Task == nil {
			t.Fatal("expected Task field to be populated")
		}
		if evt.Task.Id != "task-obs-1" {
			t.Errorf("expected task id='task-obs-1', got %q", evt.Task.Id)
		}
		if evt.Task.Status != "done" {
			t.Errorf("expected status='done', got %q", evt.Task.Status)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for TASK_STATUS_CHANGED event")
	}
}

func TestPublishTaskEvent_FailTaskPublishes(t *testing.T) {
	svc, hub, cleanup := newTestServiceWithHub(t)
	defer cleanup()

	sub := &pubsub.Subscription{
		ID:      "gaap-observer",
		AgentID: "gaap",
		Targets: map[string]bool{"task": true},
		Ch:      make(chan pubsub.Event, 10),
	}
	hub.Subscribe(sub)

	// Add a task
	_, err := svc.AddTask(context.Background(), &pb.AddTaskRequest{
		Id:        "task-obs-2",
		AgentType: "quality_scan",
		Goal:      "scan quality",
		TtlSeconds: 60,
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Claim the task
	_, err = svc.ClaimTask(context.Background(), &pb.ClaimTaskRequest{
		TaskId:  "task-obs-2",
		AgentId: "worker-2",
	})
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// Fail the task — should publish TASK_STATUS_CHANGED
	_, err = svc.FailTask(context.Background(), &pb.FailTaskRequest{
		TaskId: "task-obs-2",
	})
	if err != nil {
		t.Fatalf("FailTask: %v", err)
	}

	// Verify the event was published
	select {
	case evt := <-sub.Ch:
		if evt.Type != pb.UpdateEvent_TASK_STATUS_CHANGED {
			t.Errorf("expected TASK_STATUS_CHANGED, got %v", evt.Type)
		}
		if evt.Target != "task" {
			t.Errorf("expected target='task', got %q", evt.Target)
		}
		if evt.Task == nil {
			t.Fatal("expected Task field to be populated")
		}
		if evt.Task.Id != "task-obs-2" {
			t.Errorf("expected task id='task-obs-2', got %q", evt.Task.Id)
		}
		// Failed tasks either retry (status=ready) or dead-letter depending on retry count
		status := evt.Task.Status
		if status != "ready" && status != "dead_letter" {
			t.Errorf("expected status 'ready' or 'dead_letter', got %q", status)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for TASK_STATUS_CHANGED event")
	}
}

func TestPublishTaskEvent_NilHubDoesNotPanic(t *testing.T) {
	// Create a service with nil hub (like existing tests)
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	defer s.Close()
	svc := NewService(s, nil, nil)

	// Add and complete a task — must not panic
	_, err = svc.AddTask(context.Background(), &pb.AddTaskRequest{
		Id:        "task-nilhub",
		AgentType: "worker",
		Goal:      "do thing",
		TtlSeconds: 60,
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	_, err = svc.ClaimTask(context.Background(), &pb.ClaimTaskRequest{
		TaskId:  "task-nilhub",
		AgentId: "w",
	})
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// These must not panic
	_, err = svc.CompleteTask(context.Background(), &pb.CompleteTaskRequest{
		TaskId:    "task-nilhub",
		ResultKey: "rk",
	})
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// FailTask should be the same — it has not been wired yet, so test expects no-panic
	// after we wire it. For now it will pass because FailTask doesn't call publishTaskEvent.
	// This test will remain valid after wiring.
}
