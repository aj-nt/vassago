package grpc

import (
	"context"

	pb "github.com/aj-nt/mnemosyne/proto"
)

// AddTodo creates a new todo item.
func (s *Service) AddTodo(ctx context.Context, req *pb.AddTodoRequest) (*pb.TodoItem, error) {
	priority := req.Priority
	if priority == 0 {
		priority = 3
	}
	return s.store.AddTodo(req.Content, priority, req.SourceAgent)
}

// ListTodos returns a stream of todos.
func (s *Service) ListTodos(req *pb.ListTodosRequest, stream pb.Mnemosyne_ListTodosServer) error {
	includeCompleted := req.IncludeCompleted
	sourceAgent := ""
	if req.SourceAgent != nil {
		sourceAgent = *req.SourceAgent
	}

	todos, err := s.store.ListTodos(includeCompleted, sourceAgent)
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

// CompleteTodo marks a todo as completed.
func (s *Service) CompleteTodo(ctx context.Context, req *pb.CompleteTodoRequest) (*pb.TodoItem, error) {
	return s.store.CompleteTodo(req.Id)
}

// RemoveTodo removes a todo item.
func (s *Service) RemoveTodo(ctx context.Context, req *pb.RemoveTodoRequest) (*pb.Empty, error) {
	if err := s.store.RemoveTodo(req.Id); err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}