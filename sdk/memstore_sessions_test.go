// This file is part of Vassago.
// See LICENSE for license information.

package sdk

import (
	"context"
	"testing"

	"github.com/aj-nt/vassago-sdk/client"
)

// --- Sessions ---

func TestMemStoreCreateSession(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	id, err := store.CreateSession(ctx, "cli", "Test chat")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id == "" {
		t.Fatal("CreateSession: empty ID")
	}
}

func TestMemStoreAppendAndGetSession(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	sid, _ := store.CreateSession(ctx, "cli", "Test")
	err := store.AppendMessages(ctx, sid, []client.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	})
	if err != nil {
		t.Fatalf("AppendMessages: %v", err)
	}

	detail, err := store.GetSession(ctx, sid)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if detail.ID != sid {
		t.Errorf("GetSession: ID = %q, want %q", detail.ID, sid)
	}
	if len(detail.Messages) != 2 {
		t.Errorf("GetSession: %d messages, want 2", len(detail.Messages))
	}
	if detail.MessageCount != 2 {
		t.Errorf("GetSession: MessageCount = %d, want 2", detail.MessageCount)
	}
}

func TestMemStoreAppendMessagesNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	err := store.AppendMessages(context.Background(), "nonexistent", []client.Message{})
	if err == nil {
		t.Error("AppendMessages on nonexistent session: expected error")
	}
}

func TestMemStoreEndSession(t *testing.T) {
	store := NewMemStore("test-agent")
	if err := store.EndSession(context.Background(), "some-id"); err != nil {
		t.Errorf("EndSession: unexpected error: %v", err)
	}
}

func TestMemStoreSearchSessions(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.CreateSession(ctx, "telegram", "Morning briefing")
	store.CreateSession(ctx, "discord", "Debug session")
	store.CreateSession(ctx, "cli", "Afternoon planning")

	results, err := store.SearchSessions(ctx, "debug", 10)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchSessions('debug'): got %d, want 1", len(results))
	}
	if results[0].Title != "Debug session" {
		t.Errorf("SearchSessions: wrong title: %q", results[0].Title)
	}

	// No match.
	results, err = store.SearchSessions(ctx, "zzz_nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchSessions('nonexistent'): got %d, want 0", len(results))
	}
}

func TestMemStoreRecentSessions(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.CreateSession(ctx, "telegram", "Session 1")
	store.CreateSession(ctx, "discord", "Session 2")
	store.CreateSession(ctx, "cli", "Session 3")

	results, err := store.RecentSessions(ctx, 2)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("RecentSessions(limit=2): got %d, want 2", len(results))
	}
}

// --- Messaging ---

func TestMemStoreSendMessage(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	// SendMessage writes to an internal channel. Start a goroutine to consume it.
	msgCh := store.TestMessages()
	done := make(chan struct{})
	var captured testMessage
	go func() {
		captured = <-msgCh
		// Reply so SendMessage unblocks.
		captured.resultCh <- sendResult{msgID: "msg-123", err: nil}
		close(done)
	}()

	msgID, err := store.SendMessage(ctx, "telegram", "channel-1", "hello world")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	<-done

	if msgID != "msg-123" {
		t.Errorf("SendMessage: msgID = %q, want %q", msgID, "msg-123")
	}
	if captured.platform != "telegram" {
		t.Errorf("SendMessage: platform = %q", captured.platform)
	}
	if captured.message != "hello world" {
		t.Errorf("SendMessage: message = %q", captured.message)
	}
}

func TestMemStoreListChannels(t *testing.T) {
	store := NewMemStore("test-agent")
	channels, err := store.ListChannels(context.Background(), "telegram")
	if err != nil {
		t.Errorf("ListChannels: %v", err)
	}
	if channels != nil {
		t.Errorf("ListChannels: expected nil, got %d channels", len(channels))
	}
}
