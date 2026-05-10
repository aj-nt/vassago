// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/agent/eventbus"
	"github.com/aj-nt/mnemosyne/agent/pipeline"
	"github.com/aj-nt/mnemosyne/agent/provider"
	memoclient "github.com/aj-nt/mnemosyne/client"
	agentlog "github.com/aj-nt/mnemosyne/agent/log"
)

// ConversationCompletePayload is the data carried by conversation_complete events.
type ConversationCompletePayload struct {
	SessionID string
	Messages  []agentctx.Message
	AgentName string
	AgentID   string
}

// MemoryExtractionSubscriber listens for conversation_complete events and
// asynchronously extracts and persists memories using the cheapest model.
// Each extraction runs in its own goroutine with timeout and retry.
type MemoryExtractionSubscriber struct {
	registry  *provider.ProviderRegistry
	router    *provider.ModelRouter
	mnemo     memoclient.MnemoClient
	maxRetry  int
	timeout   time.Duration
}

// NewMemoryExtractionSubscriber creates a subscriber that uses the cheapest
// available model for memory extraction (structured JSON task, not complex reasoning).
func NewMemoryExtractionSubscriber(
	registry *provider.ProviderRegistry,
	router *provider.ModelRouter,
	mnemo memoclient.MnemoClient,
) *MemoryExtractionSubscriber {
	return &MemoryExtractionSubscriber{
		registry: registry,
		router:   router,
		mnemo:    mnemo,
		maxRetry: 3,
		timeout:  30 * time.Second,
	}
}

// Subscribe registers this subscriber on the event bus. Returns the channel
// so the caller can unsubscribe later. Non-blocking — extraction runs in
// a background goroutine.
func (s *MemoryExtractionSubscriber) Subscribe(bus *eventbus.EventBus) <-chan eventbus.Event {
	ch := bus.Subscribe("conversation_complete")
	go s.listen(ch)
	return ch
}

func (s *MemoryExtractionSubscriber) listen(ch <-chan eventbus.Event) {
	for ev := range ch {
		payload, ok := ev.Data.(ConversationCompletePayload)
		if !ok {
			continue
		}
		go s.extract(payload)
	}
}

func (s *MemoryExtractionSubscriber) extract(payload ConversationCompletePayload) {
	if len(payload.Messages) == 0 || s.mnemo == nil {
		return
	}

	// Pick the cheapest provider available for extraction.
	// Structured JSON extraction doesn't need strong reasoning —
	// the cheapest model that can do text completion is fine.
	prov := s.cheapestProvider()
	if prov == nil {
		agentlog.Printf("[memory-extract] session %s: no provider available, skipping extraction", payload.SessionID)
		return
	}

	// Retry with backoff
	var lastErr error
	for attempt := 0; attempt < s.maxRetry; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			agentlog.Printf("[memory-extract] session %s: retry %d/%d after %v", payload.SessionID, attempt+1, s.maxRetry, backoff)
			time.Sleep(backoff)
		}

		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		err := ExtractMemories(ctx, payload.Messages, prov, s.mnemo, payload.SessionID, payload.AgentName)
		cancel()

		if err == nil {
			agentlog.Printf("[memory-extract] session %s: extraction complete", payload.SessionID)
			return
		}

		lastErr = err
		agentlog.Printf("[memory-extract] session %s: attempt %d failed: %v", payload.SessionID, attempt+1, err)
	}

	agentlog.Printf("[memory-extract] session %s: all %d attempts failed, last error: %v", payload.SessionID, s.maxRetry, lastErr)
}

// cheapestProvider returns the cheapest available provider.
// Uses the model router's cost data if available; falls back to the primary.
func (s *MemoryExtractionSubscriber) cheapestProvider() provider.Provider {
	if s.router != nil {
		// Find the cheapest model registered in the router, then resolve
		// its provider through the registry (models are keyed by name,
		// providers by provider key like "openai"/"anthropic")
		cheapest := s.findCheapestModel()
		if cheapest != nil {
			if prov, err := s.registry.Get(cheapest.Provider); err == nil {
				return prov
			}
		}
	}

	if s.registry == nil {
		return nil
	}
	p, err := s.registry.GetPrimary()
	if err != nil {
		return nil
	}
	return p
}

func (s *MemoryExtractionSubscriber) findCheapestModel() *provider.ModelConfig {
	if s.router == nil {
		return nil
	}

	models := s.router.List()
	var cheapest *provider.ModelConfig
	var cheapestCost float64 = -1

	for _, name := range models {
		m, err := s.router.Get(name)
		if err != nil {
			continue
		}
		cost := m.InputCost + m.OutputCost
		if cheapestCost < 0 || cost < cheapestCost {
			cheapestCost = cost
			cheapest = m
		}
	}
	return cheapest
}

// --- Wire into Agent ---

// emitConversationComplete sends a conversation_complete event to the event bus.
// This replaces the synchronous extractAndPersistMemories call in FinallyFunc.
func (a *Agent) emitConversationComplete(sessionID string, convCtx *agentctx.ConversationContext) {
	if a.EventBus == nil {
		return
	}

	// Skip if conversation ended in error — nothing useful to extract
	// (checked in the old path; we skip emitting entirely to avoid noise)
	if convCtx == nil {
		return
	}

	a.EventBus.Emit(eventbus.Event{
		Kind: "conversation_complete",
		Data: ConversationCompletePayload{
			SessionID: sessionID,
			Messages:  convCtx.Messages,
			AgentName: a.Config.AgentName,
			AgentID:   a.Config.AgentID,
		},
	})
}

// skipMemoryExtraction returns true if we should skip extraction for this result.
func skipMemoryExtraction(result *pipeline.LoopResult) bool {
	if result == nil {
		return true
	}
	return result.ContextOverflow || result.StoppedOnError
}
