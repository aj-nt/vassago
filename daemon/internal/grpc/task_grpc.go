// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago-sdk/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AddTask creates a new orchestration task.
func (s *Service) AddTask(ctx context.Context, req *pb.AddTaskRequest) (*pb.TaskEntry, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.AgentType == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_type is required")
	}
	if req.Goal == "" {
		return nil, status.Error(codes.InvalidArgument, "goal is required")
	}

	task := &store.TaskEntry{
		ID:         req.Id,
		Owner:      ownerFromCtx(ctx),
		AgentType:  req.AgentType,
		Goal:       req.Goal,
		Context:    req.Context,
		Priority:   req.Priority,
		TTLSeconds: req.TtlSeconds,
		MaxRetries: req.MaxRetries,
		Status:     store.TaskReady,
	}
	if task.Context == "" {
		task.Context = "{}"
	}

	entry, err := s.store.AddTask(task)
	if err != nil {
		slog.Error("AddTask failed", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "add task: %v", err)
	}

	return taskEntryToProto(entry), nil
}

// GetTask retrieves a task by ID.
func (s *Service) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.TaskEntry, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	entry, err := s.store.GetTask(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get task: %v", err)
	}
	if entry == nil {
		return nil, status.Errorf(codes.NotFound, "task not found: %s", req.Id)
	}

	return taskEntryToProto(entry), nil
}

// ClaimTask atomically claims a ready task. Returns nil with OK status if
// already claimed (the caller checks assigned_agent in the response).
func (s *Service) ClaimTask(ctx context.Context, req *pb.ClaimTaskRequest) (*pb.TaskEntry, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	entry, err := s.store.ClaimTask(req.TaskId, req.AgentId)
	if err != nil {
		slog.Error("ClaimTask failed", "error", err, "task_id", req.TaskId, "agent", req.AgentId)
		return nil, status.Errorf(codes.Internal, "claim task: %v", err)
	}
	if entry == nil {
		// Task not found or already claimed — return an empty entry with status "claimed"
		// so the client can distinguish "not found" from "already claimed"
		return &pb.TaskEntry{Id: req.TaskId, Status: "claimed", AssignedAgent: "other"}, nil
	}

	return taskEntryToProto(entry), nil
}

// CompleteTask marks a task as done and records the result key.
func (s *Service) CompleteTask(ctx context.Context, req *pb.CompleteTaskRequest) (*pb.TaskEntry, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}
	if req.ResultKey == "" {
		return nil, status.Error(codes.InvalidArgument, "result_key is required")
	}

	entry, err := s.store.CompleteTask(req.TaskId, req.ResultKey)
	if err != nil {
		slog.Error("CompleteTask failed", "error", err, "task_id", req.TaskId)
		return nil, status.Errorf(codes.Internal, "complete task: %v", err)
	}
	if entry == nil {
		return nil, status.Errorf(codes.NotFound, "task not found: %s", req.TaskId)
	}

	return taskEntryToProto(entry), nil
}

// FailTask marks a claimed task as failed, triggering retry or dead-letter.
func (s *Service) FailTask(ctx context.Context, req *pb.FailTaskRequest) (*pb.TaskEntry, error) {
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}

	entry, err := s.store.FailTask(req.TaskId)
	if err != nil {
		slog.Error("FailTask failed", "error", err, "task_id", req.TaskId)
		return nil, status.Errorf(codes.Internal, "fail task: %v", err)
	}
	if entry == nil {
		return nil, status.Errorf(codes.NotFound, "task not found: %s", req.TaskId)
	}

	return taskEntryToProto(entry), nil
}

// FindReadyTasks returns tasks with status=ready, optionally filtered by agent_type.
func (s *Service) FindReadyTasks(ctx context.Context, req *pb.FindReadyTasksRequest) (*pb.TaskList, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	tasks, err := s.store.FindReadyTasks(req.AgentType, limit)
	if err != nil {
		slog.Error("FindReadyTasks failed", "error", err)
		return nil, status.Errorf(codes.Internal, "find ready tasks: %v", err)
	}

	pbTasks := make([]*pb.TaskEntry, len(tasks))
	for i, t := range tasks {
		pbTasks[i] = taskEntryToProto(t)
	}
	return &pb.TaskList{Tasks: pbTasks}, nil
}

// ListTasksByStatus returns tasks filtered by optional status.
func (s *Service) ListTasksByStatus(ctx context.Context, req *pb.ListTasksByStatusRequest) (*pb.TaskList, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	var statusFilter store.TaskStatus
	if req.Status != "" {
		statusFilter = store.TaskStatus(req.Status)
	}

	tasks, err := s.store.ListTasksByStatus(statusFilter, limit)
	if err != nil {
		slog.Error("ListTasksByStatus failed", "error", err)
		return nil, status.Errorf(codes.Internal, "list tasks: %v", err)
	}

	pbTasks := make([]*pb.TaskEntry, len(tasks))
	for i, t := range tasks {
		pbTasks[i] = taskEntryToProto(t)
	}
	return &pb.TaskList{Tasks: pbTasks}, nil
}

// ResetTimedOutTasks resets claimed tasks past their TTL back to ready.
func (s *Service) ResetTimedOutTasks(ctx context.Context, req *pb.ResetTimedOutTasksRequest) (*pb.ResetTimedOutTasksResponse, error) {
	ids, err := s.store.ResetTimedOutTasks()
	if err != nil {
		slog.Error("ResetTimedOutTasks failed", "error", err)
		return nil, status.Errorf(codes.Internal, "reset timed-out tasks: %v", err)
	}

	return &pb.ResetTimedOutTasksResponse{
		ResetIds: ids,
		Count:    int32(len(ids)),
	}, nil
}

// DeleteTask removes a task by ID.
func (s *Service) DeleteTask(ctx context.Context, req *pb.DeleteTaskRequest) (*pb.DeleteTaskResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	deleted, err := s.store.DeleteTask(req.Id)
	if err != nil {
		slog.Error("DeleteTask failed", "error", err, "id", req.Id)
		return nil, status.Errorf(codes.Internal, "delete task: %v", err)
	}

	return &pb.DeleteTaskResponse{Deleted: deleted}, nil
}

// taskEntryToProto converts a store task to a proto TaskEntry.
func taskEntryToProto(t *store.TaskEntry) *pb.TaskEntry {
	parentJSON := "[]"
	if len(t.ParentIDs) > 0 {
		b, _ := json.Marshal(t.ParentIDs)
		parentJSON = string(b)
	}
	return &pb.TaskEntry{
		Id:            t.ID,
		ParentIds:     parentJSON,
		Status:        string(t.Status),
		AgentType:     t.AgentType,
		AssignedAgent: t.AssignedAgent,
		Goal:          t.Goal,
		Context:       t.Context,
		ResultKey:     t.ResultKey,
		Priority:      t.Priority,
		CreatedAt:     t.CreatedAt,
		ClaimedAt:     t.ClaimedAt,
		CompletedAt:   t.CompletedAt,
		TtlSeconds:    t.TTLSeconds,
		RetryCount:    t.RetryCount,
		MaxRetries:    t.MaxRetries,
	}
}
