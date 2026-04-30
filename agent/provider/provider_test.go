package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// Test OpenAI Parse functionality
func TestOpenAIParse(t *testing.T) {
	provider := NewOpenAIProvider("https://api.openai.test", "test-key", "gpt-test")

	// Also test basic accessor methods
	t.Run("Name", func(t *testing.T) {
		if provider.Name() != "openai" {
			t.Errorf("Expected Name() = 'openai', got '%s'", provider.Name())
		}
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := provider.Capabilities()
		if !caps.SupportsStreaming {
			t.Error("Expected SupportsStreaming = true")
		}
		if !caps.SupportsFunctionCalling {
			t.Error("Expected SupportsFunctionCalling = true")
		}
	})

	tests := []struct {
		name           string
		responseJSON    string
		expectedText   string
		expectedTools  int
		expectedReason string
		wantErr        bool
	}{
		{
			name: "Text response",
			responseJSON: `{
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello, world!"
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 20,
					"total_tokens": 30
				}
			}`,
			expectedText:   "Hello, world!",
			expectedTools:  0,
			expectedReason: "stop",
			wantErr:        false,
		},
		{
			name: "Tool call response",
			responseJSON: `{
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Let me help",
						"tool_calls": [{
							"id": "call_123",
							"type": "function",
							"function": {
								"name": "FileRead",
								"arguments": "{\"path\": \"/test/file.txt\"}"
							}
						}]
					},
					"finish_reason": "tool_calls"
				}]
			}`,
			expectedText:   "Let me help",
			expectedTools:  1,
			expectedReason: "tool_calls",
			wantErr:        false,
		},
		{
			name: "Empty response",
			responseJSON: `{
				"choices": []
			}`,
			expectedText:   "",
			expectedTools:  0,
			expectedReason: "stop",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agentctx.ProviderResult{RawResponse: []byte(tt.responseJSON)}
			parsed, err := provider.Parse(context.Background(), result)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if parsed.Text != tt.expectedText {
				t.Errorf("Expected text '%s', got '%s'", tt.expectedText, parsed.Text)
			}

			if len(parsed.ToolCalls) != tt.expectedTools {
				t.Errorf("Expected %d tool calls, got %d", tt.expectedTools, len(parsed.ToolCalls))
			}

			if parsed.FinishReason != tt.expectedReason {
				t.Errorf("Expected finish reason '%s', got '%s'", tt.expectedReason, parsed.FinishReason)
			}
		})
	}
}

// Test OpenAI Prepare functionality
func TestOpenAIPrepare(t *testing.T) {
	provider := NewOpenAIProvider("https://api.openai.test", "test-key", "gpt-test")

	tests := []struct {
		name     string
		prepared agentctx.PreparedRequest
		wantKeys []string // Keys that should be in the request body
	}{
		{
			name: "Simple text request",
			prepared: agentctx.PreparedRequest{
				SystemPrompt: "You are helpful",
				Messages: []agentctx.Message{
					{Role: "user", Content: "Hello"},
				},
				Tools:     []agentctx.ToolDefinition{},
				APIKwargs: map[string]interface{}{"temperature": 0.7},
			},
			wantKeys: []string{"model", "messages", "temperature"},
		},
		{
			name: "Request with tools",
			prepared: agentctx.PreparedRequest{
				Messages: []agentctx.Message{
					{Role: "user", Content: "Process this"},
				},
				Tools: []agentctx.ToolDefinition{
					{
						Name:        "FileRead",
						Description: "Read file contents",
						Parameters:  map[string]interface{}{"type": "object"},
					},
				},
				APIKwargs: map[string]interface{}{},
			},
			wantKeys: []string{"model", "messages", "tools"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := provider.Prepare(context.Background(), nil, tt.prepared)
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}

			reqMap, ok := req.(map[string]interface{})
			if !ok {
				t.Fatal("Prepare() should return a map")
			}

			// Check that expected keys are present
			for _, key := range tt.wantKeys {
				if _, exists := reqMap[key]; !exists {
					t.Errorf("Expected key '%s' in request body", key)
				}
			}
		})
	}
}

// Test OpenAI Stream parsing
func TestOpenAIParseStream(t *testing.T) {
	provider := NewOpenAIProvider("https://api.openai.test", "test-key", "gpt-test")

	tests := []struct {
		name           string
		streamData     string
		expectedChunks int
		expectedText   string
	}{
		{
			name: "Simple text stream",
			streamData: "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n" +
				"data: {\"choices\":[{\"delta\":{\"content\":\" world!\"},\"index\":0}]}\n" +
				"data: [DONE]\n",
			expectedChunks: 2,
			expectedText:   "Hello world!",
		},
		{
			name: "Stream with tool calls",
			streamData: "data: {\"choices\":[{\"delta\":{\"content\":\"Processing\"},\"index\":0}]}\n" +
				"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_123\",\"type\":\"function\",\"function\":{\"name\":\"FileRead\",\"arguments\":\"{\\\"path\\\":\\\"test.txt\\\"}\"}}]},\"index\":0}]}\n" +
				"data: [DONE]\n",
			expectedChunks: 2,
			expectedText:   "Processing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.streamData))
			responses, err := provider.ParseStream(reader)
			if err != nil {
				t.Fatalf("ParseStream() error = %v", err)
			}

			if len(responses) != tt.expectedChunks {
				t.Errorf("Expected %d chunks, got %d", tt.expectedChunks, len(responses))
			}

			// Concatenate all text chunks
			var accumulatedText string
			for _, resp := range responses {
				accumulatedText += resp.Text
			}

			if accumulatedText != tt.expectedText {
				t.Errorf("Expected text '%s', got '%s'", tt.expectedText, accumulatedText)
			}
		})
	}
}

// Test Anthropic Prepare functionality
func TestAnthropicPrepare(t *testing.T) {
	provider := NewAnthropicProvider("test-key", "claude-test")

	tests := []struct {
		name     string
		prepared agentctx.PreparedRequest
		wantKeys []string // Keys that should be in the request body
	}{
		{
			name: "Simple text request",
			prepared: agentctx.PreparedRequest{
				SystemPrompt: "You are helpful",
				Messages: []agentctx.Message{
					{Role: "user", Content: "Hello"},
				},
				Tools:     []agentctx.ToolDefinition{},
				APIKwargs: map[string]interface{}{"temperature": 0.7},
			},
			wantKeys: []string{"model", "messages", "system", "temperature"},
		},
		{
			name: "Request with tools",
			prepared: agentctx.PreparedRequest{
				Messages: []agentctx.Message{
					{Role: "user", Content: "Process this"},
				},
				Tools: []agentctx.ToolDefinition{
					{
						Name:        "FileRead",
						Description: "Read file contents",
						Parameters:  map[string]interface{}{"type": "object"},
					},
				},
				APIKwargs: map[string]interface{}{},
			},
			wantKeys: []string{"model", "messages", "tools"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := provider.Prepare(context.Background(), nil, tt.prepared)
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}

			reqMap, ok := req.(map[string]interface{})
			if !ok {
				t.Fatal("Prepare() should return a map")
			}

			// Check that expected keys are present
			for _, key := range tt.wantKeys {
				if _, exists := reqMap[key]; !exists {
					t.Errorf("Expected key '%s' in request body", key)
				}
			}
		})
	}
}

// Test Provider Registry functionality
func TestProviderRegistry(t *testing.T) {
	registry := NewProviderRegistry()
	
	mock1 := NewMockProvider("mock1", nil)
	mock2 := NewMockProvider("mock2", nil)
	mock3 := NewMockProvider("mock3", nil)

	tests := []struct {
		name      string
		setup     func()
		expectErr bool
		checkFunc func(*testing.T)
	}{
		{
			name: "Register and Get",
			setup: func() {
				registry.Register("mock1", mock1)
			},
			checkFunc: func(t *testing.T) {
				p, err := registry.Get("mock1")
				if err != nil {
					t.Errorf("Get() error = %v", err)
				}
				if p.Name() != "mock1" {
					t.Errorf("Expected provider name 'mock1', got '%s'", p.Name())
				}
			},
		},
		{
			name: "SetPrimary and GetPrimary",
			setup: func() {
				registry.Register("mock1", mock1)
				registry.Register("mock2", mock2)
				registry.SetPrimary("mock2")
			},
			checkFunc: func(t *testing.T) {
				p, err := registry.GetPrimary()
				if err != nil {
					t.Errorf("GetPrimary() error = %v", err)
				}
				if p.Name() != "mock2" {
					t.Errorf("Expected primary provider 'mock2', got '%s'", p.Name())
				}
			},
		},
		{
			name: "Fallback chain",
			setup: func() {
				registry.Register("mock1", mock1)
				registry.Register("mock2", mock2)
				registry.Register("mock3", mock3)
				registry.SetFallbackChain([]string{"mock2", "mock3"})
				// Remove primary to test fallback
				delete(registry.providers, "mock1")
			},
			checkFunc: func(t *testing.T) {
				p, err := registry.GetWithFallback("mock1")
				if err != nil {
					t.Errorf("GetWithFallback() error = %v", err)
				}
				if p.Name() != "mock2" {
					t.Errorf("Expected fallback provider 'mock2', got '%s'", p.Name())
				}
			},
		},
		{
			name: "List providers",
			setup: func() {
				registry.Register("mock1", mock1)
				registry.Register("mock2", mock2)
			},
			checkFunc: func(t *testing.T) {
				providers := registry.List()
				if len(providers) != 2 {
					t.Errorf("Expected 2 providers, got %d", len(providers))
				}
			},
		},
		{
			name: "CapabilitiesOf",
			setup: func() {
				registry.Register("mock1", mock1)
			},
			checkFunc: func(t *testing.T) {
				caps, err := registry.CapabilitiesOf("mock1")
				if err != nil {
					t.Errorf("CapabilitiesOf() error = %v", err)
				}
				if !caps.SupportsFunctionCalling {
					t.Error("Expected SupportsFunctionCalling to be true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset registry for each test
			registry = NewProviderRegistry()
			mock1 = NewMockProvider("mock1", nil)
			mock2 = NewMockProvider("mock2", nil)
			mock3 = NewMockProvider("mock3", nil)

			if tt.setup != nil {
				tt.setup()
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t)
			}
		})
	}
}

// Test classifyHTTPResponse functionality
func TestClassifyHTTPResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		body           string
		expectRetry    bool
		expectFallback bool
		expectError    bool
	}{
		{
			name:           "200 OK",
			statusCode:     200,
			body:           `{"result": "success"}`,
			expectRetry:    false,
			expectFallback: false,
			expectError:    false,
		},
		{
			name:           "400 Bad Request",
			statusCode:     400,
			body:           `{"error": "bad request"}`,
			expectRetry:    false,
			expectFallback: false,
			expectError:    true,
		},
		{
			name:           "401 Unauthorized",
			statusCode:     401,
			body:           `{"error": "unauthorized"}`,
			expectRetry:    false,
			expectFallback: false,
			expectError:    true,
		},
		{
			name:           "403 Forbidden",
			statusCode:     403,
			body:           `{"error": "forbidden"}`,
			expectRetry:    false,
			expectFallback: false,
			expectError:    true,
		},
		{
			name:           "429 Rate Limited",
			statusCode:     429,
			body:           `{"error": "rate limited"}`,
			expectRetry:    true,
			expectFallback: true,
			expectError:    true,
		},
		{
			name:           "500 Server Error",
			statusCode:     500,
			body:           `{"error": "server error"}`,
			expectRetry:    false,
			expectFallback: true, // 5xx should fallback to next provider
			expectError:    true,
		},
		{
			name:           "502 Bad Gateway",
			statusCode:     502,
			body:           `{"error": "bad gateway"}`,
			expectRetry:    false,
			expectFallback: true, // 5xx should fallback to next provider
			expectError:    true,
		},
		{
			name:           "503 Service Unavailable",
			statusCode:     503,
			body:           `{"error": "service unavailable"}`,
			expectRetry:    true,
			expectFallback: true, // 5xx should fallback to next provider
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			
			result := classifyHTTPResponse(resp)

			if result.ShouldRetry != tt.expectRetry {
				t.Errorf("Expected ShouldRetry = %v, got %v", tt.expectRetry, result.ShouldRetry)
			}

			if result.ShouldFallback != tt.expectFallback {
				t.Errorf("Expected ShouldFallback = %v, got %v", tt.expectFallback, result.ShouldFallback)
			}

			if tt.expectError != (result.Error != nil) {
				t.Errorf("Expected error = %v, got %v", tt.expectError, result.Error != nil)
			}
		})
	}
}

// Test OpenAI Stream with mock HTTP server
func TestOpenAIStream(t *testing.T) {
	provider := NewOpenAIProvider("http://localhost:0", "test-key", "gpt-test")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"Hello"},"index":0}]}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":" world!"},"index":0}]}`)
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":""},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	provider.baseURL = server.URL
	provider.client = &http.Client{}

	req := map[string]interface{}{
		"model":    "gpt-test",
		"messages":  []map[string]interface{}{{"role": "user", "content": "Hello"}},
		"stream":    true,
	}

	streamCh, err := provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var texts []string
	var gotDone bool
	var gotUsage *agentctx.UsageInfo
	for event := range streamCh {
		if event.Error != nil {
			t.Errorf("Stream event error: %v", event.Error)
			continue
		}
		if event.Type == agentctx.StreamText {
			texts = append(texts, event.Text)
		}
		if event.Type == agentctx.StreamDone {
			gotDone = true
			gotUsage = event.Usage
		}
	}

	combined := strings.Join(texts, "")
	if combined != "Hello world!" {
		t.Errorf("Expected stream text 'Hello world!', got '%s'", combined)
	}
	if !gotDone {
		t.Error("Expected StreamDone event")
	}
	if gotUsage == nil || gotUsage.TotalTokens != 15 {
		t.Errorf("Expected usage TotalTokens=15, got %v", gotUsage)
	}
}

// Test Anthropic Stream with mock HTTP server
func TestAnthropicStream(t *testing.T) {
	provider := NewAnthropicProvider("test-key", "claude-test")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-test","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":0}}`)
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world!"}}`)
		fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", `{"type":"content_block_stop","index":0}`)
		fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
		fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", `{"type":"message_stop"}`)
	}))
	defer server.Close()

	provider.baseURL = server.URL
	provider.client = &http.Client{}

	req := map[string]interface{}{
		"model":    "claude-test",
		"messages":  []map[string]interface{}{{"role": "user", "content": "Hello"}},
		"stream":    true,
		"max_tokens": float64(100),
	}

	streamCh, err := provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var texts []string
	var gotDone bool
	for event := range streamCh {
		if event.Error != nil {
			t.Errorf("Stream event error: %v", event.Error)
			continue
		}
		if event.Type == agentctx.StreamText {
			texts = append(texts, event.Text)
		}
		if event.Type == agentctx.StreamDone {
			gotDone = true
		}
	}

	combined := strings.Join(texts, "")
	if combined != "Hello world!" {
		t.Errorf("Expected stream text 'Hello world!', got '%s'", combined)
	}
	if !gotDone {
		t.Error("Expected StreamDone event")
	}
}