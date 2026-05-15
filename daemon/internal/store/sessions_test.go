// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func TestCreateSession(t *testing.T) {
	s := testStore(t)

	sess, err := s.CreateSession("agent-001", "cli", "Test Session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if sess.ID == "" {
		t.Error("session ID is empty")
	}
	if sess.AgentID != "agent-001" {
		t.Errorf("agent_id = %q, want %q", sess.AgentID, "agent-001")
	}
	if sess.Source != "cli" {
		t.Errorf("source = %q, want %q", sess.Source, "cli")
	}
	if sess.Title != "Test Session" {
		t.Errorf("title = %q, want %q", sess.Title, "Test Session")
	}
}

func TestGetSession(t *testing.T) {
	s := testStore(t)

	created, _ := s.CreateSession("agent-001", "cli", "Test")
	got, err := s.GetSession(created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGetSession_notFound(t *testing.T) {
	s := testStore(t)

	_, err := s.GetSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestAddMessages(t *testing.T) {
	s := testStore(t)

	sess, _ := s.CreateSession("agent-001", "cli", "Chat")

	msgs := []Message{
		{SessionID: sess.ID, Role: "user", Content: "Hello", Timestamp: 1000},
		{SessionID: sess.ID, Role: "assistant", Content: "Hi there", Timestamp: 1001},
	}

	err := s.AddMessages(sess.ID, msgs)
	if err != nil {
		t.Fatalf("add messages: %v", err)
	}

	// Verify message count updated
	got, _ := s.GetSession(sess.ID)
	if got.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", got.MessageCount)
	}
}

func TestEndSession(t *testing.T) {
	s := testStore(t)

	sess, _ := s.CreateSession("agent-001", "cli", "Test")

	err := s.EndSession(sess.ID)
	if err != nil {
		t.Fatalf("end session: %v", err)
	}

	got, _ := s.GetSession(sess.ID)
	if got.EndedAt == 0 {
		t.Error("ended_at should be set")
	}
}

func TestEndSession_alreadyEnded(t *testing.T) {
	s := testStore(t)

	sess, _ := s.CreateSession("agent-001", "cli", "Test")
	s.EndSession(sess.ID) //nolint:errcheck

	err := s.EndSession(sess.ID)
	if err == nil {
		t.Error("expected error for already-ended session")
	}
}

func TestEnsureAgent(t *testing.T) {
	s := testStore(t)

	agent, isNew, err := s.EnsureAgent("agent-001", "dev-agent", "development")
	if err != nil {
		t.Fatalf("ensure agent: %v", err)
	}
	if agent.ID != "agent-001" {
		t.Errorf("ID = %q, want %q", agent.ID, "agent-001")
	}
	if agent.Name != "dev-agent" {
		t.Errorf("name = %q, want %q", agent.Name, "dev-agent")
	}
	if !isNew {
		t.Error("expected isNew=true for first registration")
	}

	// Ensure idempotent — second call should return isNew=false
	agent2, isNew2, err := s.EnsureAgent("agent-001", "dev-agent", "development")
	if err != nil {
		t.Fatalf("ensure agent again: %v", err)
	}
	if agent2.ID != agent.ID {
		t.Error("idempotent ensure changed agent ID")
	}
	if isNew2 {
		t.Error("expected isNew=false for re-registration")
	}
}
func TestSetAgentLastSession(t *testing.T) {
	s := testStore(t)

	// Create agent
	agent, _, err := s.EnsureAgent("agent-sess", "test", "test")
	if err != nil {
		t.Fatalf("ensure agent: %v", err)
	}

	// Set last session
	err = s.SetAgentLastSession(agent.ID, "session-123", "309 messages, 1h 55m")
	if err != nil {
		t.Fatalf("set agent last session: %v", err)
	}

	// Read back
	got, err := s.GetAgentByID(agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.LastSessionID != "session-123" {
		t.Errorf("LastSessionID = %q, want %q", got.LastSessionID, "session-123")
	}
	if got.LastSessionSummary != "309 messages, 1h 55m" {
		t.Errorf("LastSessionSummary = %q, want %q", got.LastSessionSummary, "309 messages, 1h 55m")
	}
}

func TestSetAgentLastSession_overwrite(t *testing.T) {
	s := testStore(t)

	agent, _, _ := s.EnsureAgent("agent-sess-2", "test", "test")

	// Set first session
	err := s.SetAgentLastSession(agent.ID, "old-id", "old summary")
	if err != nil {
		t.Fatalf("set agent last session (first): %v", err)
	}

	// Overwrite
	err = s.SetAgentLastSession(agent.ID, "new-id", "new summary")
	if err != nil {
		t.Fatalf("set agent last session (second): %v", err)
	}

	got, err := s.GetAgentByID(agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.LastSessionID != "new-id" {
		t.Errorf("LastSessionID = %q, want %q", got.LastSessionID, "new-id")
	}
	if got.LastSessionSummary != "new summary" {
		t.Errorf("LastSessionSummary = %q, want %q", got.LastSessionSummary, "new summary")
	}
}

func TestGetAgentByID_notFound(t *testing.T) {
	s := testStore(t)

	_, err := s.GetAgentByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestEnsureAgent_returnsLastSessionFields(t *testing.T) {
	s := testStore(t)

	// Create agent with EnsureAgent — should get empty session fields
	agent, isNew, err := s.EnsureAgent("agent-fresh", "fresh", "test")
	if err != nil {
		t.Fatalf("ensure agent: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true")
	}
	if agent.LastSessionID != "" {
		t.Errorf("new agent LastSessionID = %q, want empty", agent.LastSessionID)
	}
	if agent.LastSessionSummary != "" {
		t.Errorf("new agent LastSessionSummary = %q, want empty", agent.LastSessionSummary)
	}
}
