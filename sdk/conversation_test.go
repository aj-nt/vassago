// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

func TestPlatformKey(t *testing.T) {
	got := platformKey("discord", "channel-123")
	want := "discord:channel-123"
	if got != want {
		t.Errorf("platformKey: got %q, want %q", got, want)
	}
}

func TestNewConversationManagerDefaults(t *testing.T) {
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		return "ok", nil
	}

	store := NewMemStore("test-agent")
	cm := NewConversationManager(store, handler, ConversationManagerConfig{})
	if cm.responseTimeout != 120*time.Second {
		t.Errorf("default ResponseTimeout: got %v, want 120s", cm.responseTimeout)
	}
	if cm.maxMsgChars != 0 {
		t.Errorf("default MaxMessageChars: got %d, want 0", cm.maxMsgChars)
	}
	if cm.handler == nil {
		t.Error("handler is nil")
	}
	if cm.store == nil {
		t.Error("store is nil")
	}
}

func TestNewConversationManagerExplicit(t *testing.T) {
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		return "ok", nil
	}

	store := NewMemStore("test-agent")
	cm := NewConversationManager(store, handler, ConversationManagerConfig{
		ResponseTimeout: 30 * time.Second,
		MaxMessageChars: 500,
	})
	if cm.responseTimeout != 30*time.Second {
		t.Errorf("ResponseTimeout: got %v, want 30s", cm.responseTimeout)
	}
	if cm.maxMsgChars != 500 {
		t.Errorf("MaxMessageChars: got %d, want 500", cm.maxMsgChars)
	}
}

func TestGetChannelMutex(t *testing.T) {
	cm := &ConversationManager{}

	key1 := "discord:chan1"
	key2 := "discord:chan2"

	mu1 := cm.getChannelMutex(key1)
	mu2 := cm.getChannelMutex(key1) // same key, should return same mutex
	mu3 := cm.getChannelMutex(key2) // different key, different mutex

	if mu1 != mu2 {
		t.Error("same key returned different mutex")
	}
	if mu1 == mu3 {
		t.Error("different keys returned same mutex")
	}
}

// makeTestCM creates a ConversationManager backed by a MemStore for testing.
func makeTestCM(handler MessageHandler, cfg ConversationManagerConfig) (*MemStore, *ConversationManager) {
	store := NewMemStore("test-agent")
	cm := &ConversationManager{
		store:           store,
		handler:         handler,
		responseTimeout: cfg.ResponseTimeout,
		maxMsgChars:     cfg.MaxMessageChars,
	}
	if cm.responseTimeout <= 0 {
		cm.responseTimeout = 120 * time.Second
	}
	return store, cm
}

// startTestMessageConsumer reads from the MemStore test message channel
// and responds to each message. Returns a done channel and the captured messages.
func startTestMessageConsumer(store *MemStore) (chan struct{}, *[]string, *sync.Mutex) {
	msgCh := store.TestMessages()
	done := make(chan struct{})
	var msgs []string
	var mu sync.Mutex
	go func() {
		for {
			select {
			case msg := <-msgCh:
				mu.Lock()
				msgs = append(msgs, msg.message)
				mu.Unlock()
				msg.resultCh <- sendResult{msgID: "msg-" + msg.message[:min(5, len(msg.message))], err: nil}
			case <-done:
				return
			}
		}
	}()
	return done, &msgs, &mu
}

func TestHandleMessageEmptyMessage(t *testing.T) {
	called := false
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		called = true
		return "called", nil
	}

	store, cm := makeTestCM(handler, ConversationManagerConfig{})
	done, _, _ := startTestMessageConsumer(store)
	defer close(done)

	cm.handleMessage(&pb.MessageEvent{
		Platform:  "telegram",
		ChannelId: "chan1",
		Message:   "",
	})

	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("handler was called for empty message")
	}
}

func TestHandleMessageTruncation(t *testing.T) {
	handlerCalled := make(chan *pb.MessageEvent, 1)
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		handlerCalled <- event
		return "ok", nil
	}

	store, cm := makeTestCM(handler, ConversationManagerConfig{MaxMessageChars: 10})
	done, _, _ := startTestMessageConsumer(store)
	defer close(done)

	cm.handleMessage(&pb.MessageEvent{
		Platform:    "discord",
		ChannelId:   "chan1",
		ChannelName: "#general",
		SenderName:  "test-user",
		Message:     "this is a very long message that should be truncated",
	})

	select {
	case event := <-handlerCalled:
		if len(event.Message) > 10 {
			t.Errorf("message not truncated: len=%d, content=%q", len(event.Message), event.Message)
		}
		if event.Message != "this is a " {
			t.Errorf("truncated message = %q, want %q", event.Message, "this is a ")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler was never called")
	}
}

func TestHandleMessageNoTruncation(t *testing.T) {
	handlerCalled := make(chan *pb.MessageEvent, 1)
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		handlerCalled <- event
		return "ok", nil
	}

	store, cm := makeTestCM(handler, ConversationManagerConfig{MaxMessageChars: 0})
	done, _, _ := startTestMessageConsumer(store)
	defer close(done)

	original := "short message"
	cm.handleMessage(&pb.MessageEvent{
		Platform:    "telegram",
		ChannelId:   "chan2",
		ChannelName: "chat",
		SenderName:  "user",
		Message:     original,
	})

	select {
	case event := <-handlerCalled:
		if event.Message != original {
			t.Errorf("message was truncated when it shouldn't have been: %q", event.Message)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler was never called")
	}
}

func TestHandleMessageHandlerError(t *testing.T) {
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		return "", fmt.Errorf("test error")
	}

	store, cm := makeTestCM(handler, ConversationManagerConfig{})
	done, msgs, mu := startTestMessageConsumer(store)
	defer close(done)

	cm.handleMessage(&pb.MessageEvent{
		Platform:    "discord",
		ChannelId:   "chan-err",
		ChannelName: "errors",
		SenderName:  "user",
		Message:     "trigger error",
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	found := false
	for _, r := range *msgs {
		if strings.Contains(r, "Sorry, I encountered an error") {
			found = true
			break
		}
	}
	mu.Unlock()

	if !found {
		t.Error("handler error did not produce error response message")
	}
}

func TestHandleMessageEmptyResponse(t *testing.T) {
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		return "", nil // empty response = don't send
	}

	store, cm := makeTestCM(handler, ConversationManagerConfig{})
	done, msgs, mu := startTestMessageConsumer(store)
	defer close(done)

	cm.handleMessage(&pb.MessageEvent{
		Platform:    "telegram",
		ChannelId:   "chan-empty",
		ChannelName: "chat",
		SenderName:  "user",
		Message:     "hello",
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	sentCount := len(*msgs)
	mu.Unlock()

	if sentCount > 0 {
		t.Errorf("empty response triggered SendMessage (count=%d)", sentCount)
	}
}

func TestHandleMessageConversationCreated(t *testing.T) {
	handlerCalled := make(chan *Conversation, 1)
	handler := func(ctx context.Context, event *pb.MessageEvent, conv *Conversation) (string, error) {
		handlerCalled <- conv
		return "ok", nil
	}

	store, cm := makeTestCM(handler, ConversationManagerConfig{})
	done, _, _ := startTestMessageConsumer(store)
	defer close(done)

	cm.handleMessage(&pb.MessageEvent{
		Platform:    "telegram",
		ChannelId:   "conv-test",
		ChannelName: "Test Chat",
		SenderName:  "user",
		Message:     "hello world",
	})

	select {
	case conv := <-handlerCalled:
		if conv.SessionID == "" {
			t.Error("conversation has no SessionID")
		}
		// Verify conversation was stored in sessions map.
		key := platformKey("telegram", "conv-test")
		if cached, ok := cm.sessions.Load(key); !ok {
			t.Error("conversation not stored in sessions map")
		} else if cached.(*Conversation) != conv {
			t.Error("stored conversation pointer doesn't match handler's conversation")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler was never called")
	}
}

func TestGetOrCreateConversationCached(t *testing.T) {
	store := NewMemStore("test-agent")
	cm := &ConversationManager{
		store:           store,
		responseTimeout: 120 * time.Second,
	}

	event := &pb.MessageEvent{Platform: "telegram", ChannelId: "tg-chan", ChannelName: "TG"}
	key := platformKey(event.Platform, event.ChannelId)

	// First call creates.
	conv1 := cm.getOrCreateConversation(key, event)
	if conv1 == nil {
		t.Fatal("first getOrCreateConversation returned nil")
	}
	if conv1.SessionID == "" {
		t.Error("SessionID is empty")
	}

	// Second call should return the same pointer.
	conv2 := cm.getOrCreateConversation(key, event)
	if conv1 != conv2 {
		t.Error("second call returned different conversation pointer")
	}
}
