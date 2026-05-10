package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

func TestReadSSEStreamWithReasoningContent(t *testing.T) {
	// SSE data with reasoning_content (DeepSeek format)
	sseData := `data: {"choices":[{"delta":{"reasoning_content":"Let me think"},"finish_reason":null}]}

data: {"choices":[{"delta":{"reasoning_content":" about this"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"The answer"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":" is 42"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseData))
	}))
	defer ts.Close()

	p := NewOpenAIProvider(ts.URL, "test-key", "test-model")
	p.streamClient = &http.Client{}

	ch, err := p.Stream(context.Background(), map[string]interface{}{"messages": []interface{}{}})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var reasoningParts []string
	var textParts []string
	for ev := range ch {
		switch ev.Type {
		case agentctx.StreamReasoning:
			reasoningParts = append(reasoningParts, ev.Reasoning)
		case agentctx.StreamText:
			textParts = append(textParts, ev.Text)
		}
	}

	if len(reasoningParts) < 2 {
		t.Errorf("expected 2+ reasoning events, got %d", len(reasoningParts))
	}
	if len(textParts) < 2 {
		t.Errorf("expected 2+ text events, got %d", len(textParts))
	}
}

func TestReadSSEStreamWithReasoning(t *testing.T) {
	// SSE data with reasoning (OpenAI o-series format)
	sseData := `data: {"choices":[{"delta":{"reasoning":"Let me think"},"finish_reason":null}]}

data: {"choices":[{"delta":{"reasoning":" about this"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"The answer"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":" is 42"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseData))
	}))
	defer ts.Close()

	p := NewOpenAIProvider(ts.URL, "test-key", "test-model")
	p.streamClient = &http.Client{}

	ch, err := p.Stream(context.Background(), map[string]interface{}{"messages": []interface{}{}})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var reasoningParts []string
	var textParts []string
	for ev := range ch {
		switch ev.Type {
		case agentctx.StreamReasoning:
			reasoningParts = append(reasoningParts, ev.Reasoning)
		case agentctx.StreamText:
			textParts = append(textParts, ev.Text)
		}
	}

	if len(reasoningParts) < 2 {
		t.Errorf("expected 2+ reasoning events, got %d", len(reasoningParts))
	}
	if len(textParts) < 2 {
		t.Errorf("expected 2+ text events, got %d", len(textParts))
	}
}