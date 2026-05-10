// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"errors"
	"testing"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/pipeline"
	"github.com/aj-nt/mnemosyne/agent/provider"
)

var errSentinel = errors.New("sentinel error for testing")

func TestSkipMemoryExtraction(t *testing.T) {
	tests := []struct {
		name   string
		result *pipeline.LoopResult
		want   bool
	}{
		{"nil result", nil, true},
		{"normal", &pipeline.LoopResult{}, false},
		{"context overflow", &pipeline.LoopResult{ContextOverflow: true}, true},
		{"stopped on error", &pipeline.LoopResult{StoppedOnError: true}, true},
		{"both", &pipeline.LoopResult{ContextOverflow: true, StoppedOnError: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipMemoryExtraction(tt.result); got != tt.want {
				t.Errorf("skipMemoryExtraction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmitConversationComplete(t *testing.T) {
	bus := eventbus.NewEventBus()
	defer bus.Close()

	a := &Agent{
		Config:   Config{AgentName: "test-agent", AgentID: "agent-1"},
		EventBus: bus,
	}

	ch := bus.Subscribe("conversation_complete")
	convCtx := &agentctx.ConversationContext{
		SessionID: "session-1",
		Messages: []agentctx.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}

	a.emitConversationComplete("session-1", convCtx)

	select {
	case ev := <-ch:
		if ev.Kind != "conversation_complete" {
			t.Errorf("expected conversation_complete, got %s", ev.Kind)
		}
		payload, ok := ev.Data.(ConversationCompletePayload)
		if !ok {
			t.Fatalf("expected ConversationCompletePayload, got %T", ev.Data)
		}
		if payload.SessionID != "session-1" {
			t.Errorf("sessionID = %s, want session-1", payload.SessionID)
		}
		if len(payload.Messages) != 2 {
			t.Errorf("message count = %d, want 2", len(payload.Messages))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestEmitConversationComplete_NilBus(t *testing.T) {
	a := &Agent{EventBus: nil}
	a.emitConversationComplete("s1", &agentctx.ConversationContext{SessionID: "s1"})
}

func TestEmitConversationComplete_NilConvCtx(t *testing.T) {
	bus := eventbus.NewEventBus()
	defer bus.Close()
	a := &Agent{EventBus: bus}
	a.emitConversationComplete("s1", nil)
}

func TestMemoryExtractionSubscriber_Subscribe(t *testing.T) {
	bus := eventbus.NewEventBus()
	defer bus.Close()

	reg := provider.NewProviderRegistry()
	reg.Register("mock", &mockExtractProvider{
		response: agentctx.ParsedResponse{Text: `[]`},
	})
	reg.SetPrimary("mock")

	sub := NewMemoryExtractionSubscriber(reg, nil, &recordingClient{})
	if ch := sub.Subscribe(bus); ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
}

func TestMemoryExtractionSubscriber_ReceivesEvent(t *testing.T) {
	bus := eventbus.NewEventBus()
	defer bus.Close()

	mockClient := &recordingClient{}

	reg := provider.NewProviderRegistry()
	reg.Register("mock", &mockExtractProvider{
		response: agentctx.ParsedResponse{
			Text: `[{"key":"test_key","content":"test content","category":"observation","priority":2}]`,
		},
	})
	reg.SetPrimary("mock")

	sub := NewMemoryExtractionSubscriber(reg, nil, mockClient)
	sub.Subscribe(bus)

	bus.Emit(eventbus.Event{
		Kind: "conversation_complete",
		Data: ConversationCompletePayload{
			SessionID: "session-1",
			Messages: []agentctx.Message{
				{Role: "user", Content: "This is a test"},
				{Role: "assistant", Content: "Got it"},
			},
			AgentName: "test-agent",
		},
	})

	time.Sleep(300 * time.Millisecond)

	saved := mockClient.SavedMemories()
	if len(saved) != 1 {
		t.Fatalf("expected 1 saved memory, got %d", len(saved))
	}
	if saved[0].Key != "test_key" {
		t.Errorf("expected key 'test_key', got %q", saved[0].Key)
	}
}

func TestMemoryExtractionSubscriber_SkipsOnEmptyMessages(t *testing.T) {
	bus := eventbus.NewEventBus()
	defer bus.Close()

	mockClient := &recordingClient{}
	reg := provider.NewProviderRegistry()
	reg.Register("mock", &mockExtractProvider{err: errSentinel})
	reg.SetPrimary("mock")

	sub := NewMemoryExtractionSubscriber(reg, nil, mockClient)
	sub.Subscribe(bus)

	bus.Emit(eventbus.Event{
		Kind: "conversation_complete",
		Data: ConversationCompletePayload{
			SessionID: "session-1",
			Messages:  []agentctx.Message{},
			AgentName: "test-agent",
		},
	})

	time.Sleep(100 * time.Millisecond)
	if len(mockClient.SavedMemories()) != 0 {
		t.Errorf("expected 0 memories, got %d", len(mockClient.SavedMemories()))
	}
}

func TestMemoryExtractionSubscriber_NilMnemo(t *testing.T) {
	bus := eventbus.NewEventBus()
	defer bus.Close()

	reg := provider.NewProviderRegistry()
	reg.Register("mock", &mockExtractProvider{})
	reg.SetPrimary("mock")

	sub := NewMemoryExtractionSubscriber(reg, nil, nil)
	sub.Subscribe(bus)

	bus.Emit(eventbus.Event{
		Kind: "conversation_complete",
		Data: ConversationCompletePayload{
			SessionID: "session-1",
			Messages:  []agentctx.Message{{Role: "user", Content: "test"}},
			AgentName: "test-agent",
		},
	})

	time.Sleep(100 * time.Millisecond)
}

func TestMemoryExtractionSubscriber_FindCheapestModel_Empty(t *testing.T) {
	sub := &MemoryExtractionSubscriber{router: nil}
	if got := sub.findCheapestModel(); got != nil {
		t.Errorf("expected nil with no router, got %v", got)
	}
}
