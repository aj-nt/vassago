// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"testing"

	pb "github.com/aj-nt/vassago/proto"
)

// --- Memory CRUD tests ---

func TestClientGRPC_AddMemory(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	entry, err := c.AddMemory(ctx, "memory", "environment", "test-key", "test content", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}
	if entry.Id == "" {
		t.Error("Expected entry ID to be set")
	}
	if entry.Content != "test content" {
		t.Errorf("Expected content 'test content', got %q", entry.Content)
	}
	if entry.Target != "memory" {
		t.Errorf("Expected target 'memory', got %q", entry.Target)
	}
	if entry.SourceAgent != "test-agent" {
		t.Errorf("Expected source_agent 'test-agent', got %q", entry.SourceAgent)
	}
}

func TestClientGRPC_AddMemory_Validation(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, err := c.AddMemory(ctx, "", "environment", "key", "content", 3, "agent")
	if err == nil {
		t.Error("Expected error for empty target")
	}

	_, err = c.AddMemory(ctx, "memory", "environment", "", "content", 3, "agent")
	if err == nil {
		t.Error("Expected error for empty key")
	}

	_, err = c.AddMemory(ctx, "memory", "environment", "key", "content", 3, "")
	if err == nil {
		t.Error("Expected error for empty source agent")
	}
}

func TestClientGRPC_GetMemory(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	// Add first, then get
	entry, err := c.AddMemory(ctx, "memory", "environment", "test-key", "hello world", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddMemory failed: %v", err)
	}

	got, err := c.GetMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("GetMemory failed: %v", err)
	}
	if got.Id != entry.Id {
		t.Errorf("Expected ID %s, got %s", entry.Id, got.Id)
	}
}

func TestClientGRPC_GetMemory_NotFound(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, err := c.GetMemory(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent memory")
	}
}

func TestClientGRPC_RemoveMemory(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	entry, _ := c.AddMemory(ctx, "memory", "environment", "test-key", "content", 3, "test-agent")

	err := c.RemoveMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("RemoveMemory failed: %v", err)
	}

	// Verify it's gone
	_, err = c.GetMemory(ctx, entry.Id)
	if err == nil {
		t.Error("Expected error after removing memory")
	}
}

func TestClientGRPC_SearchMemories(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, _ = c.AddMemory(ctx, "memory", "environment", "key1", "python is great", 3, "agent1")
	_, _ = c.AddMemory(ctx, "memory", "observation", "key2", "go is fast", 3, "agent1")

	results, err := c.SearchMemories(ctx, "python", "memory", 10)
	if err != nil {
		t.Fatalf("SearchMemories failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least one search result")
	}
}

func TestClientGRPC_ListMemories(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, _ = c.AddMemory(ctx, "memory", "environment", "key1", "content 1", 3, "agent1")
	_, _ = c.AddMemory(ctx, "memory", "observation", "key2", "content 2", 3, "agent1")

	entries, err := c.ListMemories(ctx, "memory", nil, nil)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("Expected at least 2 memories, got %d", len(entries))
	}
}

// --- Hot Block tests ---

func TestClientGRPC_GetHotBlock(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	block, err := c.GetHotBlock(ctx, "memory", "test-agent", 4000, nil, nil, 0)
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if block == "" {
		t.Error("Expected non-empty hot block")
	}
}

// --- Session tests ---

func TestClientGRPC_CreateSession(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	sessionID, err := c.CreateSession(ctx, "test-agent", "cli", "test session")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sessionID == "" {
		t.Error("Expected session ID to be set")
	}
}

func TestClientGRPC_AddMessages(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	sessionID, _ := c.CreateSession(ctx, "test-agent", "cli", "test session")

	msgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}
	err := c.AddMessages(ctx, sessionID, msgs)
	if err != nil {
		t.Fatalf("AddMessages failed: %v", err)
	}
}

func TestClientGRPC_EndSession(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	sessionID, _ := c.CreateSession(ctx, "test-agent", "cli", "test session")

	err := c.EndSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}
}

func TestClientGRPC_SearchSessions(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, _ = c.CreateSession(ctx, "test-agent", "cli", "test session about python")

	sessions, err := c.SearchSessions(ctx, "python", 10)
	if err != nil {
		t.Fatalf("SearchSessions failed: %v", err)
	}
	if len(sessions) == 0 {
		t.Error("Expected at least one session in search results")
	}
}

func TestClientGRPC_ListRecentSessions(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, _ = c.CreateSession(ctx, "test-agent", "cli", "session 1")

	sessions, err := c.ListRecentSessions(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecentSessions failed: %v", err)
	}
	// Mock returns all sessions
	t.Logf("Got %d sessions", len(sessions))
}

func TestClientGRPC_GetSession(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	sessionID, _ := c.CreateSession(ctx, "test-agent", "cli", "test session")

	detail, err := c.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if detail.ID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, detail.ID)
	}
}

// --- Agent tests ---

func TestClientGRPC_RegisterAgent(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	agentID, isNew, err := c.RegisterAgent(ctx, "test-agent", "TestAgent", "dev")
	if err != nil {
		t.Fatalf("RegisterAgent failed: %v", err)
	}
	if agentID != "test-agent" {
		t.Errorf("Expected agent ID 'test-agent', got %q", agentID)
	}
	if !isNew {
		t.Error("Expected new agent registration")
	}

	// Register again — should not be new
	_, isNew, err = c.RegisterAgent(ctx, "test-agent", "TestAgent", "dev")
	if err != nil {
		t.Fatalf("Second RegisterAgent failed: %v", err)
	}
	if isNew {
		t.Error("Expected existing agent, not new")
	}
}

func TestClientGRPC_Heartbeat(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	err := c.Heartbeat(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

// --- Consolidation tests ---

func TestClientGRPC_Consolidate(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	resp, err := c.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL, 100)
	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}
	if resp == nil {
		t.Error("Expected non-nil ConsolidateResponse")
	}
}

// --- Health check tests ---

func TestClientGRPC_HealthCheck(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	err := c.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestClientGRPC_IsConnected(t *testing.T) {
	c, _, _ := setupMockServer(t)

	if !c.IsConnected() {
		t.Error("Expected Client.IsConnected() to be true")
	}

	// NullMnemo should return false
	var vassago MnemoClient = &NullMnemo{}
	if vassago.IsConnected() {
		t.Error("Expected NullMnemo.IsConnected() to be false")
	}
}

// --- Todo tests ---

func TestClientGRPC_TodoCRUD(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	// AddTodo
	todo, err := c.AddTodo(ctx, "Buy groceries", 3, "test-agent")
	if err != nil {
		t.Fatalf("AddTodo failed: %v", err)
	}
	if todo.ID == "" {
		t.Error("Expected todo ID to be set")
	}
	if todo.Content != "Buy groceries" {
		t.Errorf("Expected content 'Buy groceries', got %q", todo.Content)
	}

	// ListTodos
	todos, err := c.ListTodos(ctx, true, "")
	if err != nil {
		t.Fatalf("ListTodos failed: %v", err)
	}
	if len(todos) == 0 {
		t.Error("Expected at least one todo")
	}

	// CompleteTodo
	completed, err := c.CompleteTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("CompleteTodo failed: %v", err)
	}
	if !completed.Completed {
		t.Error("Expected todo to be completed")
	}

	// RemoveTodo
	err = c.RemoveTodo(ctx, todo.ID)
	if err != nil {
		t.Fatalf("RemoveTodo failed: %v", err)
	}
}

// --- Messaging tests ---

func TestClientGRPC_SendMessage(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	msgID, err := c.SendMessage(ctx, "test-platform", "test-channel", "hello world")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if msgID == "" {
		t.Error("Expected message ID to be set")
	}
}

func TestClientGRPC_ListChannels(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	channels, err := c.ListChannels(ctx, "test-platform")
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}
	if len(channels) == 0 {
		t.Error("Expected at least one channel")
	}
}

// --- Cron tests ---

func TestClientGRPC_CronCRUD(t *testing.T) {
	c, mockSrv, _ := setupMockServer(t)
	ctx := context.Background()
	_ = mockSrv // Use mock server for cron job storage

	// Register agent first (cron jobs reference agent IDs)
	c.RegisterAgent(ctx, "cron-agent", "CronAgent", "test")

	// CreateCronJob
	enabled := true
	job, err := c.CreateCronJob(ctx, "test-cron", "*/5 * * * *", "cron-agent", "hello", &enabled)
	if err != nil {
		t.Fatalf("CreateCronJob failed: %v", err)
	}
	if job.ID == "" {
		t.Error("Expected job ID to be set")
	}

	// ListCronJobs
	jobs, err := c.ListCronJobs(ctx, "cron-agent", false)
	if err != nil {
		t.Fatalf("ListCronJobs failed: %v", err)
	}
	if len(jobs) == 0 {
		t.Error("Expected at least one cron job")
	}

	// UpdateCronJob
	newName := "updated-name"
	updated, err := c.UpdateCronJob(ctx, job.ID, &newName, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateCronJob failed: %v", err)
	}
	if updated.Name != "updated-name" {
		t.Errorf("Expected name 'updated-name', got %q", updated.Name)
	}

	// DeleteCronJob
	err = c.DeleteCronJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("DeleteCronJob failed: %v", err)
	}
}

// --- ProtoCronJob conversion test (covers protoCronJobToInfo) ---

func TestProtoCronJobToInfo(t *testing.T) {
	pbJob := &pb.CronJob{
		Id:         "cron-1",
		Name:       "test-job",
		Schedule:   "*/5 * * * *",
		AgentId:    "agent-1",
		Message:    "hello",
		Enabled:    true,
		CreatedAt:  1700000000,
		UpdatedAt:  1700001000,
		LastFireAt: 1700002000,
		NextFireAt: 1700003000,
	}
	info := protoCronJobToInfo(pbJob)
	if info.ID != "cron-1" {
		t.Errorf("Expected ID 'cron-1', got %q", info.ID)
	}
	if info.Name != "test-job" {
		t.Errorf("Expected name 'test-job', got %q", info.Name)
	}
	if !info.Enabled {
		t.Error("Expected Enabled=true")
	}
}

// --- Error path tests ---

func TestClientGRPC_GetMemoryNonexistent(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, err := c.GetMemory(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent memory ID")
	}
}

func TestClientGRPC_EndSessionNonexistent(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	err := c.EndSession(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent session ID")
	}
}

func TestClientGRPC_AddMessagesToNonexistentSession(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	msgs := []Message{{Role: "user", Content: "hello"}}
	err := c.AddMessages(ctx, "nonexistent-id", msgs)
	if err == nil {
		t.Error("Expected error when adding messages to nonexistent session")
	}
}

func TestClientGRPC_GetSessionNonexistent(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, err := c.GetSession(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent session ID")
	}
}

func TestClientGRPC_CompleteTodoNonexistent(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	_, err := c.CompleteTodo(ctx, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for nonexistent todo ID")
	}
}

func TestClientGRPC_UpdateCronJobNonexistent(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	name := "new-name"
	_, err := c.UpdateCronJob(ctx, "nonexistent-id", &name, nil, nil, nil)
	if err == nil {
		t.Error("Expected error for nonexistent cron job ID")
	}
}

// --- ConnectMnemo test ---

func TestConnectMnemoType(t *testing.T) {
	// Verify ConnectMnemo returns MnemoClient interface type
	// Can't test actual connection without a running daemon, but we verify
	// that Client implements MnemoClient
	var _ MnemoClient = (*Client)(nil)
	var _ MnemoClient = (*NullMnemo)(nil)
}

// --- NullMnemo comprehensive tests ---

func TestNullMnemo_AllWriteMethods(t *testing.T) {
	n := &NullMnemo{}
	ctx := context.Background()

	// Write methods should return "vassago not connected" error
	_, err := n.AddMemory(ctx, "memory", "env", "key", "content", 3, "agent")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("AddMemory: expected 'vassago not connected' error, got %v", err)
	}

	_, err = n.GetMemory(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("GetMemory: expected 'vassago not connected' error, got %v", err)
	}

	err = n.RemoveMemory(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("RemoveMemory: expected 'vassago not connected' error, got %v", err)
	}

	_, err = n.Consolidate(ctx, pb.ConsolidateScope_CONSOLIDATE_SCOPE_ALL, 100)
	if err != nil {
		t.Errorf("Consolidate: expected nil (fire-and-forget), got %v", err)
	}

	_, err = n.SendMessage(ctx, "platform", "target", "message")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("SendMessage: expected 'vassago not connected' error, got %v", err)
	}

	_, err = n.AddTodo(ctx, "content", 3, "agent")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("AddTodo: expected 'vassago not connected' error, got %v", err)
	}

	_, err = n.CompleteTodo(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("CompleteTodo: expected 'vassago not connected' error, got %v", err)
	}

	err = n.RemoveTodo(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("RemoveTodo: expected 'vassago not connected' error, got %v", err)
	}

	_, err = n.CreateCronJob(ctx, "name", "schedule", "agent", "msg", nil)
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("CreateCronJob: expected 'vassago not connected' error, got %v", err)
	}

	_, err = n.UpdateCronJob(ctx, "id", nil, nil, nil, nil)
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("UpdateCronJob: expected 'vassago not connected' error, got %v", err)
	}

	err = n.DeleteCronJob(ctx, "id")
	if err == nil || err.Error() != "vassago not connected" {
		t.Errorf("DeleteCronJob: expected 'vassago not connected' error, got %v", err)
	}
}

func TestNullMnemo_AllReadMethods(t *testing.T) {
	n := &NullMnemo{}
	ctx := context.Background()

	// Read methods should return nil/empty, no error
	entries, err := n.SearchMemories(ctx, "query", "memory", 10)
	if err != nil {
		t.Errorf("SearchMemories: unexpected error %v", err)
	}
	if entries != nil {
		t.Errorf("SearchMemories: expected nil, got %v", entries)
	}

	memories, err := n.ListMemories(ctx, "memory", nil, nil)
	if err != nil {
		t.Errorf("ListMemories: unexpected error %v", err)
	}
	if memories != nil {
		t.Errorf("ListMemories: expected nil, got %v", memories)
	}

	block, err := n.GetHotBlock(ctx, "memory", "agent", 4000, nil, nil, 0)
	if err != nil {
		t.Errorf("GetHotBlock: unexpected error %v", err)
	}
	if block != "" {
		t.Errorf("GetHotBlock: expected empty string, got %q", block)
	}

	sessionID, err := n.CreateSession(ctx, "agent", "cli", "title")
	if err != nil {
		t.Errorf("CreateSession: unexpected error %v", err)
	}
	if sessionID != "" {
		t.Errorf("CreateSession: expected empty string, got %q", sessionID)
	}

	err = n.AddMessages(ctx, "session-id", nil)
	if err != nil {
		t.Errorf("AddMessages: unexpected error %v", err)
	}

	err = n.EndSession(ctx, "session-id")
	if err != nil {
		t.Errorf("EndSession: unexpected error %v", err)
	}

	sessions, err := n.ListRecentSessions(ctx, 10)
	if err != nil {
		t.Errorf("ListRecentSessions: unexpected error %v", err)
	}
	if sessions != nil {
		t.Errorf("ListRecentSessions: expected nil, got %v", sessions)
	}

	detail, err := n.GetSession(ctx, "session-id")
	if err != nil {
		t.Errorf("GetSession: unexpected error %v", err)
	}
	if detail != nil {
		t.Errorf("GetSession: expected nil, got %v", detail)
	}

	todos, err := n.ListTodos(ctx, true, "")
	if err != nil {
		t.Errorf("ListTodos: unexpected error %v", err)
	}
	if todos != nil {
		t.Errorf("ListTodos: expected nil, got %v", todos)
	}

	cronJobs, err := n.ListCronJobs(ctx, "agent", false)
	if err != nil {
		t.Errorf("ListCronJobs: unexpected error %v", err)
	}
	if cronJobs != nil {
		t.Errorf("ListCronJobs: expected nil, got %v", cronJobs)
	}

	channels, err := n.ListChannels(ctx, "platform")
	if err != nil {
		t.Errorf("ListChannels: unexpected error %v", err)
	}
	if channels != nil {
		t.Errorf("ListChannels: expected nil, got %v", channels)
	}

	// RegisterAgent returns empty values
	agentID, isNew, err := n.RegisterAgent(ctx, "agent", "name", "role")
	if err != nil {
		t.Errorf("RegisterAgent: unexpected error %v", err)
	}
	if agentID != "" || isNew {
		t.Errorf("RegisterAgent: expected empty/false, got %q/%v", agentID, isNew)
	}

	// Heartbeat succeeds silently
	err = n.Heartbeat(ctx, "agent")
	if err != nil {
		t.Errorf("Heartbeat: unexpected error %v", err)
	}
}

// --- Skill tests ---

func TestClientGRPC_AddSkill(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	skill, err := c.AddSkill(ctx, "Test Skill", "A test skill", []string{"test", "demo"}, "devops", "Test content", "test-agent", 3)
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}
	if skill.Id == "" {
		t.Error("Expected skill ID to be set")
	}
	if skill.Name != "Test Skill" {
		t.Errorf("Expected name 'Test Skill', got %q", skill.Name)
	}
	if skill.Category != "devops" {
		t.Errorf("Expected category 'devops', got %q", skill.Category)
	}
	if skill.Priority != 3 {
		t.Errorf("Expected priority 3, got %d", skill.Priority)
	}
}

func TestClientGRPC_GetSkill(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	created, err := c.AddSkill(ctx, "Get Skill", "A skill to test get", []string{"get"}, "test", "Get content", "test-agent", 2)
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}

	got, err := c.GetSkill(ctx, created.Id)
	if err != nil {
		t.Fatalf("GetSkill failed: %v", err)
	}
	if got.Name != "Get Skill" {
		t.Errorf("Expected name 'Get Skill', got %q", got.Name)
	}
}

func TestClientGRPC_ListSkills(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	c.AddSkill(ctx, "List Skill 1", "First", []string{"l1"}, "cat1", "C1", "agent1", 1)
	c.AddSkill(ctx, "List Skill 2", "Second", []string{"l2"}, "cat2", "C2", "agent2", 2)

	skills, err := c.ListSkills(ctx, "", "", 20)
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(skills))
	}
}

func TestClientGRPC_SearchSkills(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	c.AddSkill(ctx, "DB Debug", "Debug database issues", []string{"db"}, "devops", "Database debugging steps", "ops-agent", 3)

	skills, err := c.SearchSkills(ctx, "database", "", 10)
	if err != nil {
		t.Fatalf("SearchSkills failed: %v", err)
	}
	if len(skills) == 0 {
		t.Error("Expected at least one result for 'database'")
	}
}

func TestClientGRPC_RemoveSkill(t *testing.T) {
	c, _, _ := setupMockServer(t)
	ctx := context.Background()

	created, err := c.AddSkill(ctx, "ToRemove", "Will be removed", []string{"rm"}, "test", "x", "test-agent", 1)
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}

	success, err := c.RemoveSkill(ctx, created.Id)
	if err != nil {
		t.Fatalf("RemoveSkill failed: %v", err)
	}
	if !success {
		t.Error("Expected success=true for RemoveSkill")
	}
}
