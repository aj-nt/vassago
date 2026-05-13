// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/google/uuid"
)

// MemStore is an in-memory MemoryStore for development and testing.
// It implements the same interface as the real daemon-backed store,
// but stores everything in Go maps and slices. No daemon required.
//
// Use MemStore when:
//   - Writing tests for agent memory integration
//   - Developing locally without the daemon running
//   - Running in environments where SQLite/gRPC aren't available
type MemStore struct {
	mu sync.RWMutex

	agentName string

	// Memory entries keyed by ID.
	memories map[string]*pb.MemoryEntry

	// Sessions keyed by session ID.
	sessions map[string]*client.SessionDetail

	// Todos keyed by ID.
	todos map[string]*client.TodoItem

	// Skills keyed by ID.
	skills map[string]*pb.SkillEntry

	// Cron jobs keyed by ID.
	jobs map[string]*client.CronJobInfo

	// Messages map[platform]map[channel] -> channel for testing.
	messageCh chan testMessage
}

type testMessage struct {
	platform  string
	target    string
	message   string
	resultCh  chan sendResult
}

type sendResult struct {
	msgID string
	err   error
}

// NewMemStore creates an in-memory store for testing.
func NewMemStore(agentName string) *MemStore {
	return &MemStore{
		agentName: agentName,
		memories:  make(map[string]*pb.MemoryEntry),
		sessions:  make(map[string]*client.SessionDetail),
		todos:     make(map[string]*client.TodoItem),
		skills:    make(map[string]*pb.SkillEntry),
		jobs:      make(map[string]*client.CronJobInfo),
		messageCh: make(chan testMessage, 100),
	}
}

// --- Hot Block ---

func (m *MemStore) HotBlock(ctx context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var b strings.Builder
	b.WriteString("## Memories\n")
	count := 0
	for _, mem := range m.memories {
		if count >= 20 {
			break
		}
		b.WriteString(fmt.Sprintf("- [%s] %s\n", mem.Category, mem.Content))
		count++
	}
	if count == 0 {
		b.WriteString("(no memories)\n")
	}
	return b.String(), nil
}

func (m *MemStore) CachedHotBlock() string {
	hot, _ := m.HotBlock(context.Background())
	return hot
}

// --- Memory CRUD ---

func (m *MemStore) Remember(ctx context.Context, content, category string, priority int32) (*pb.MemoryEntry, error) {
	return m.RememberWithKey(ctx, "", content, category, priority)
}

func (m *MemStore) RememberWithKey(ctx context.Context, key, content, category string, priority int32) (*pb.MemoryEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()[:8]
	now := time.Now().Unix()
	entry := &pb.MemoryEntry{
		Id:          id,
		Target:      "memory",
		Category:    category,
		Key:         key,
		Content:     content,
		Priority:    priority,
		SourceAgent: m.agentName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.memories[id] = entry
	return entry, nil
}

func (m *MemStore) Forget(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.memories, id)
	return nil
}

func (m *MemStore) Search(ctx context.Context, query string, limit int32) ([]*pb.MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query = strings.ToLower(query)
	var results []*pb.MemoryEntry
	for _, mem := range m.memories {
		if strings.Contains(strings.ToLower(mem.Content), query) {
			results = append(results, mem)
			if len(results) >= int(limit) {
				break
			}
		}
	}
	return results, nil
}

func (m *MemStore) List(ctx context.Context, category *string, minPriority *int32) ([]*pb.MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*pb.MemoryEntry
	for _, mem := range m.memories {
		if category != nil && *category != "" && mem.Category != *category {
			continue
		}
		if minPriority != nil && mem.Priority < *minPriority {
			continue
		}
		results = append(results, mem)
	}
	return results, nil
}

func (m *MemStore) Consolidate(ctx context.Context, scope pb.ConsolidateScope, batchSize int32) (*pb.ConsolidateResponse, error) {
	return &pb.ConsolidateResponse{Removed: 0}, nil
}

func (m *MemStore) GetMemory(ctx context.Context, id string) (*pb.MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mem, ok := m.memories[id]
	if !ok {
		return nil, fmt.Errorf("memory %s not found", id)
	}
	return mem, nil
}

// --- Session Management ---

func (m *MemStore) CreateSession(ctx context.Context, source, title string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := "mem-" + uuid.New().String()[:8]
	now := time.Now().Unix()
	m.sessions[id] = &client.SessionDetail{
		ID:        id,
		Title:     title,
		Source:    source,
		CreatedAt: now,
	}
	return id, nil
}

func (m *MemStore) AppendMessages(ctx context.Context, sessionID string, messages []client.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	sess.Messages = append(sess.Messages, messages...)
	sess.MessageCount = int32(len(sess.Messages))
	return nil
}

func (m *MemStore) EndSession(ctx context.Context, sessionID string) error { return nil }

func (m *MemStore) GetSession(ctx context.Context, sessionID string) (*client.SessionDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}
	return sess, nil
}

func (m *MemStore) SearchSessions(ctx context.Context, query string, limit int32) ([]client.SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []client.SessionInfo
	for _, s := range m.sessions {
		if strings.Contains(strings.ToLower(s.Title), strings.ToLower(query)) {
			results = append(results, client.SessionInfo{
				ID:           s.ID,
				Title:        s.Title,
				Source:       s.Source,
				MessageCount: s.MessageCount,
			})
			if len(results) >= int(limit) {
				break
			}
		}
	}
	return results, nil
}

func (m *MemStore) RecentSessions(ctx context.Context, limit int32) ([]client.SessionInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []client.SessionInfo
	for _, s := range m.sessions {
		results = append(results, client.SessionInfo{
			ID:           s.ID,
			Title:        s.Title,
			Source:       s.Source,
			MessageCount: s.MessageCount,
		})
		if len(results) >= int(limit) {
			break
		}
	}
	return results, nil
}

// --- Messaging ---

func (m *MemStore) SendMessage(ctx context.Context, platform, target, message string) (string, error) {
	resultCh := make(chan sendResult, 1)
	m.messageCh <- testMessage{platform: platform, target: target, message: message, resultCh: resultCh}
	result := <-resultCh
	return result.msgID, result.err
}

func (m *MemStore) ListChannels(ctx context.Context, platform string) ([]*pb.Channel, error) {
	return nil, nil
}

// TestMessages returns a channel of messages "sent" via SendMessage.
// Tests can read from this to verify what would have been sent to platforms.
func (m *MemStore) TestMessages() <-chan testMessage {
	return m.messageCh
}

// --- Todos ---

func (m *MemStore) AddTodo(ctx context.Context, content string, priority int32) (*client.TodoItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := "todo-" + uuid.New().String()[:8]
	now := time.Now().Unix()
	todo := &client.TodoItem{
		ID:          id,
		Content:     content,
		Priority:    priority,
		SourceAgent: m.agentName,
		CreatedAt:   now,
	}
	m.todos[id] = todo
	return todo, nil
}

func (m *MemStore) ListTodos(ctx context.Context, includeCompleted bool) ([]client.TodoItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []client.TodoItem
	for _, t := range m.todos {
		if !includeCompleted && t.Completed {
			continue
		}
		results = append(results, *t)
	}
	return results, nil
}

func (m *MemStore) CompleteTodo(ctx context.Context, id string) (*client.TodoItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	todo, ok := m.todos[id]
	if !ok {
		return nil, fmt.Errorf("todo %s not found", id)
	}
	todo.Completed = true
	todo.CompletedAt = time.Now().Unix()
	return todo, nil
}

func (m *MemStore) RemoveTodo(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.todos, id)
	return nil
}

// --- Skills ---

func (m *MemStore) AddSkill(ctx context.Context, name, description string, triggers []string, category, content string, priority int32) (*pb.SkillEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := "skill-" + uuid.New().String()[:8]
	now := time.Now().Unix()
	skill := &pb.SkillEntry{
		Id:          id,
		Name:        name,
		Description: description,
		Triggers:    triggers,
		Category:    category,
		Content:     content,
		SourceAgent: m.agentName,
		Priority:    priority,
		CreatedAt:   now,
	}
	m.skills[id] = skill
	return skill, nil
}

func (m *MemStore) GetSkill(ctx context.Context, id string) (*pb.SkillEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.skills[id]
	if !ok {
		return nil, fmt.Errorf("skill %s not found", id)
	}
	return s, nil
}

func (m *MemStore) ListSkills(ctx context.Context, category string, limit int32) ([]*pb.SkillEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*pb.SkillEntry
	for _, s := range m.skills {
		if category != "" && s.Category != category {
			continue
		}
		results = append(results, s)
		if len(results) >= int(limit) {
			break
		}
	}
	return results, nil
}

func (m *MemStore) SearchSkills(ctx context.Context, query, category string, limit int32) ([]*pb.SkillEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query = strings.ToLower(query)
	var results []*pb.SkillEntry
	for _, s := range m.skills {
		if category != "" && s.Category != category {
			continue
		}
		if strings.Contains(strings.ToLower(s.Name), query) ||
			strings.Contains(strings.ToLower(s.Description), query) {
			results = append(results, s)
			if len(results) >= int(limit) {
				break
			}
		}
	}
	return results, nil
}

func (m *MemStore) RemoveSkill(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.skills[id]
	delete(m.skills, id)
	return ok, nil
}

// --- Cron ---

func (m *MemStore) ScheduleJob(ctx context.Context, name, schedule, message string, enabled bool) (*client.CronJobInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := "job-" + uuid.New().String()[:8]
	now := time.Now().Unix()
	job := &client.CronJobInfo{
		ID:        id,
		Name:      name,
		Schedule:  schedule,
		AgentID:   m.agentName,
		Message:   message,
		Enabled:   enabled,
		CreatedAt: now,
	}
	m.jobs[id] = job
	return job, nil
}

func (m *MemStore) ListJobs(ctx context.Context, includeDisabled bool) ([]client.CronJobInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []client.CronJobInfo
	for _, j := range m.jobs {
		if !includeDisabled && !j.Enabled {
			continue
		}
		results = append(results, *j)
	}
	return results, nil
}

func (m *MemStore) UpdateJob(ctx context.Context, id string, name, schedule, message *string, enabled *bool) (*client.CronJobInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}
	if name != nil {
		j.Name = *name
	}
	if schedule != nil {
		j.Schedule = *schedule
	}
	if message != nil {
		j.Message = *message
	}
	if enabled != nil {
		j.Enabled = *enabled
	}
	return j, nil
}

func (m *MemStore) DeleteJob(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, id)
	return nil
}

// Close is a no-op for MemStore.
func (m *MemStore) Close() error { return nil }

// AgentName returns the configured agent name.
func (m *MemStore) AgentName() string { return m.agentName }

// IsConnected always returns true for MemStore.
func (m *MemStore) IsConnected() bool { return true }
