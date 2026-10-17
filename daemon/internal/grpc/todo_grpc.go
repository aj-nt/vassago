// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

// AddTodo creates a new todo item, scoped to owner (Phase 28).
func (s *Service) AddTodo(ctx context.Context, req *pb.AddTodoRequest) (*pb.TodoItem, error) {
	priority := req.Priority
	if priority == 0 {
		priority = 3
	}
	return s.store.AddTodo(ownerFromCtx(ctx), req.Content, priority, req.SourceAgent)
}

// ListTodos returns a stream of todos, scoped to owner (Phase 28).
func (s *Service) ListTodos(req *pb.ListTodosRequest, stream pb.Vassago_ListTodosServer) error {
	includeCompleted := req.IncludeCompleted
	sourceAgent := ""
	if req.SourceAgent != nil {
		sourceAgent = *req.SourceAgent
	}

	// Extract owner from gRPC metadata context
	todos, err := s.store.ListTodos(ownerFromCtx(stream.Context()), includeCompleted, sourceAgent)
	if err != nil {
		return err
	}

	for _, todo := range todos {
		if err := stream.Send(todo); err != nil {
			return err
		}
	}
	return nil
}

// CompleteTodo marks a todo as completed, scoped to owner (Phase 28).
func (s *Service) CompleteTodo(ctx context.Context, req *pb.CompleteTodoRequest) (*pb.TodoItem, error) {
	return s.store.CompleteTodo(ownerFromCtx(ctx), req.Id)
}

// RemoveTodo removes a todo item, scoped to owner (Phase 28).
func (s *Service) RemoveTodo(ctx context.Context, req *pb.RemoveTodoRequest) (*pb.Empty, error) {
	if err := s.store.RemoveTodo(ownerFromCtx(ctx), req.Id); err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}
