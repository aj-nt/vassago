package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
// Covers OpenAI, Ollama, OpenRouter, and any /v1/chat/completions endpoint.
type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	model      string
	client     *http.Client
	capabilities agentctx.ProviderCapabilities
}

// NewOpenAIProvider creates a new OpenAI-compatible provider.
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
		capabilities: agentctx.ProviderCapabilities{
			SupportsStreaming:       true,
			SupportsFunctionCalling: true,
			SupportsPromptCaching:  strings.Contains(baseURL, "openai"),
			SupportsVision:         true,
			MaxContextTokens:       128000,
		},
	}
}

// Name returns the provider name.
func (p *OpenAIProvider) Name() string { return "openai" }

// Capabilities returns what this provider supports.
func (p *OpenAIProvider) Capabilities() agentctx.ProviderCapabilities { return p.capabilities }

// Prepare builds the OpenAI chat completion request.
func (p *OpenAIProvider) Prepare(_ context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
	messages := make([]map[string]interface{}, 0, len(prepared.Messages)+1)

	// System prompt
	if prepared.SystemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": prepared.SystemPrompt,
		})
	}

	// Conversation messages
	for _, msg := range prepared.Messages {
		m := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if len(msg.ToolCalls) > 0 {
			tcs := make([]interface{}, len(msg.ToolCalls))
			for i, tc := range msg.ToolCalls {
				tcs[i] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": string(tc.Arguments),
					},
				}
			}
			m["tool_calls"] = tcs
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		if msg.Name != "" {
			m["name"] = msg.Name
		}
		messages = append(messages, m)
	}

	// Build request body
	body := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
	}

	// Add tools (function calling)
	if len(prepared.Tools) > 0 && p.capabilities.SupportsFunctionCalling {
		tools := make([]interface{}, len(prepared.Tools))
		for i, t := range prepared.Tools {
			tools[i] = map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			}
		}
		body["tools"] = tools
	}

	// Merge API kwargs (temperature, etc.)
	for k, v := range prepared.APIKwargs {
		body[k] = v
	}

	return body, nil
}

// Execute sends the request to the OpenAI-compatible API.
func (p *OpenAIProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	bodyJSON, err := json.Marshal(req)
	if err != nil {
		return agentctx.ProviderResult{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return agentctx.ProviderResult{ShouldRetry: false, Error: err}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return agentctx.ProviderResult{ShouldRetry: true, Error: err}, nil
	}
	defer resp.Body.Close()

	// Retry on server errors
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(resp.Body)
		return agentctx.ProviderResult{
			ShouldRetry:   resp.StatusCode == 503 || resp.StatusCode == 429,
			ShouldFallback: resp.StatusCode == 429,
			Error:         fmt.Errorf("server error %d: %s", resp.StatusCode, string(body)),
		}, nil
	}
	if resp.StatusCode == 429 {
		body, _ := io.ReadAll(resp.Body)
		return agentctx.ProviderResult{
			ShouldRetry:    true,
			ShouldFallback: true,
			Error:          fmt.Errorf("rate limited: %s", string(body)),
		}, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return agentctx.ProviderResult{
			ShouldRetry: false,
			Error:       fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	// Read and return the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return agentctx.ProviderResult{Error: err}, nil
	}

	return agentctx.ProviderResult{
		RawResponse: respBody,
	}, nil
}

// Parse converts the OpenAI response to a canonical ParsedResponse.
func (p *OpenAIProvider) Parse(_ context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	if result.Error != nil {
		return agentctx.ParsedResponse{}, result.Error
	}

	respBody, ok := result.RawResponse.([]byte)
	if !ok {
		return agentctx.ParsedResponse{}, fmt.Errorf("openai: unexpected raw response type %T", result.RawResponse)
	}

	var resp openaiResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return agentctx.ParsedResponse{}, fmt.Errorf("parse response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return agentctx.ParsedResponse{Text: "", FinishReason: "stop"}, nil
	}

	choice := resp.Choices[0]
	parsed := agentctx.ParsedResponse{
		FinishReason: choice.FinishReason,
		Usage:        convertUsage(resp.Usage),
	}

	// Extract text content
	if choice.Message != nil {
		parsed.Text = choice.Message.Content
		parsed.Reasoning = choice.Message.Reasoning

		// Extract tool calls
		if len(choice.Message.ToolCalls) > 0 {
			parsed.ToolCalls = make([]agentctx.ToolCall, len(choice.Message.ToolCalls))
			for i, tc := range choice.Message.ToolCalls {
				args, _ := json.Marshal(tc.Function.Arguments)
				parsed.ToolCalls[i] = agentctx.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				}
			}
		}
	}

	return parsed, nil
}

// ParseStream reads SSE events from a streaming response.
func (p *OpenAIProvider) ParseStream(reader io.Reader) ([]agentctx.ParsedResponse, error) {
	var responses []agentctx.ParsedResponse
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // Skip malformed chunks
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		resp := agentctx.ParsedResponse{
			Text:         delta.Content,
			FinishReason: chunk.Choices[0].FinishReason,
		}
		if delta.ToolCalls != nil {
			resp.ToolCalls = make([]agentctx.ToolCall, len(delta.ToolCalls))
			for i, tc := range delta.ToolCalls {
				args, _ := json.Marshal(tc.Function.Arguments)
				resp.ToolCalls[i] = agentctx.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				}
			}
		}
		responses = append(responses, resp)
	}

	return responses, scanner.Err()
}

// Internal types for JSON unmarshaling

type openaiResponse struct {
	Choices []openaiChoice    `json:"choices"`
	Usage   openaiUsage       `json:"usage"`
}

type openaiChoice struct {
	Index        int               `json:"index"`
	Message      *openaiMessage    `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type openaiMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall  `json:"tool_calls,omitempty"`
	Reasoning string           `json:"reasoning,omitempty"` // o1/o3 models
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiUsage struct {
	PromptTokens    int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openaiStreamChunk struct {
	Choices []struct {
		Index        int `json:"index"`
		Delta        struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func convertUsage(u openaiUsage) *agentctx.UsageInfo {
	if u.TotalTokens == 0 {
		return nil
	}
	return &agentctx.UsageInfo{
		PromptTokens:    u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}
