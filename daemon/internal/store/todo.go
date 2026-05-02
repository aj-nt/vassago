package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
)

// AddTodo creates a new todo item.
func (s *Store) AddTodo(content string, priority int32, sourceAgent string) (*pb.TodoItem, error) {
	id := generateID()
	now := time.Now().Unix()

	_, err := s.db.Exec(
		`INSERT INTO todos (id, content, completed, priority, source_agent, created_at, updated_at, completed_at)
		 VALUES (?, ?, 0, ?, ?, ?, ?, 0)`,
		id, content, priority, sourceAgent, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("AddTodo: %w", err)
	}

	return &pb.TodoItem{
		Id:          id,
		Content:     content,
		Completed:   false,
		Priority:    priority,
		SourceAgent: sourceAgent,
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: 0,
	}, nil
}

// ListTodos returns todos, optionally filtering by completion status and agent.
func (s *Store) ListTodos(includeCompleted bool, sourceAgent string) ([]*pb.TodoItem, error) {
	query := `SELECT id, content, completed, priority, source_agent, created_at, updated_at, completed_at FROM todos`
	args := []interface{}{}

	where := []string{}
	if !includeCompleted {
		where = append(where, "completed = 0")
	}
	if sourceAgent != "" {
		where = append(where, "source_agent = ?")
		args = append(args, sourceAgent)
	}
	if len(where) > 0 {
		query += " WHERE " + joinWhere(where)
	}
	query += " ORDER BY priority DESC, created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListTodos: %w", err)
	}
	defer rows.Close()

	var todos []*pb.TodoItem
	for rows.Next() {
		var item pb.TodoItem
		var completed bool
		var completedAt sql.NullInt64
		if err := rows.Scan(&item.Id, &item.Content, &completed, &item.Priority, &item.SourceAgent, &item.CreatedAt, &item.UpdatedAt, &completedAt); err != nil {
			return nil, fmt.Errorf("ListTodos scan: %w", err)
		}
		item.Completed = completed
		if completedAt.Valid {
			item.CompletedAt = completedAt.Int64
		}
		todos = append(todos, &item)
	}
	return todos, rows.Err()
}

// CompleteTodo marks a todo as completed.
func (s *Store) CompleteTodo(id string) (*pb.TodoItem, error) {
	now := time.Now().Unix()

	result, err := s.db.Exec(
		`UPDATE todos SET completed = 1, updated_at = ?, completed_at = ? WHERE id = ?`,
		now, now, id,
	)
	if err != nil {
		return nil, fmt.Errorf("CompleteTodo: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return nil, fmt.Errorf("CompleteTodo: todo %s not found", id)
	}

	return s.GetTodo(id)
}

// RemoveTodo removes a todo item.
func (s *Store) RemoveTodo(id string) error {
	result, err := s.db.Exec(`DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("RemoveTodo: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("RemoveTodo: todo %s not found", id)
	}
	return nil
}

// GetTodo returns a single todo by ID.
func (s *Store) GetTodo(id string) (*pb.TodoItem, error) {
	var item pb.TodoItem
	var completed bool
	var completedAt sql.NullInt64

	err := s.db.QueryRow(
		`SELECT id, content, completed, priority, source_agent, created_at, updated_at, completed_at FROM todos WHERE id = ?`,
		id,
	).Scan(&item.Id, &item.Content, &completed, &item.Priority, &item.SourceAgent, &item.CreatedAt, &item.UpdatedAt, &completedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("GetTodo: todo %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("GetTodo: %w", err)
	}

	item.Completed = completed
	if completedAt.Valid {
		item.CompletedAt = completedAt.Int64
	}
	return &item, nil
}

// generateID creates a random 16-character hex ID.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func joinWhere(clauses []string) string {
	result := clauses[0]
	for _, c := range clauses[1:] {
		result += " AND " + c
	}
	return result
}