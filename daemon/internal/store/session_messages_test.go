// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func TestGetSessionMessages(t *testing.T) {
	s := testStore(t)

	// Create a session and add messages
	sess, err := s.CreateSession("test-agent", "cli", "test session")
	if err != nil {
		t.Fatal(err)
	}

	msgs := []Message{
		{SessionID: sess.ID, Role: "user", Content: "hello"},
		{SessionID: sess.ID, Role: "assistant", Content: "hi there"},
		{SessionID: sess.ID, Role: "user", Content: "goodbye"},
	}
	for _, m := range msgs {
		if err := s.AddMessage(m); err != nil {
			t.Fatal(err)
		}
	}

	// Retrieve messages
	got, err := s.GetSessionMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	if got[0].Role != "user" || got[0].Content != "hello" {
		t.Errorf("first message = %v, want user/hello", got[0])
	}
	if got[2].Role != "user" || got[2].Content != "goodbye" {
		t.Errorf("third message = %v, want user/goodbye", got[2])
	}
}

func TestGetSessionMessagesEmpty(t *testing.T) {
	s := testStore(t)

	sess, err := s.CreateSession("test-agent", "cli", "empty session")
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSessionMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 messages, got %d", len(got))
	}
}

func TestGetSessionMessagesNotFound(t *testing.T) {
	s := testStore(t)

	got, err := s.GetSessionMessages("nonexistent")
	// No error — just returns empty slice for unknown sessions
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 messages for nonexistent session, got %d", len(got))
	}
}