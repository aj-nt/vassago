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
	s.EndSession(sess.ID)

	err := s.EndSession(sess.ID)
	if err == nil {
		t.Error("expected error for already-ended session")
	}
}

func TestEnsureAgent(t *testing.T) {
	s := testStore(t)

	agent, err := s.EnsureAgent("agent-001", "dev-agent", "development")
	if err != nil {
		t.Fatalf("ensure agent: %v", err)
	}
	if agent.ID != "agent-001" {
		t.Errorf("ID = %q, want %q", agent.ID, "agent-001")
	}
	if agent.Name != "dev-agent" {
		t.Errorf("name = %q, want %q", agent.Name, "dev-agent")
	}

	// Ensure idempotent
	agent2, err := s.EnsureAgent("agent-001", "dev-agent", "development")
	if err != nil {
		t.Fatalf("ensure agent again: %v", err)
	}
	if agent2.ID != agent.ID {
		t.Error("idempotent ensure changed agent ID")
	}
}