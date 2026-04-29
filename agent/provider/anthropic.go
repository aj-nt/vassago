package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// AnthropicProvider implements Provider for Anthropic Messages API.
type AnthropicProvider struct {
	apiKey     string
	model      string
	client     *http.Client
	capabilities agentctx.ProviderCapabilities
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{},
		capabilities: agentctx.ProviderCapabilities{
			SupportsStreaming:       true,
			SupportsFunctionCalling: true,
			SupportsPromptCaching:  true,
			SupportsVision:         false,
			MaxContextTokens:       200000,
		},
	}
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string { return "anthropic" }

// Capabilities returns what this provider supports.
func (p *AnthropicProvider) Capabilities() agentctx.ProviderCapabilities { return p.capabilities }

// Prepare builds the Anthropic Messages API request.
func (p *AnthropicProvider) Prepare(_ context.Context, convCtx *agentctx.ConversationContext, prepared agentctx.PreparedRequest) (interface{}, error) {
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
			content := msg.Content
			for _, tc := range msg.ToolCalls {
				content += fmt.Sprintf("\n<%s>%s</%s>", tc.Name, string(tc.Arguments), tc.Name)
			}
			m["content"] = content
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		messages = append(messages, m)
	}

	// Build request body
	body := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
		"max_tokens": 4096,
	}

	// Add tools (function calling)
	if len(prepared.Tools) > 0 && p.capabilities.SupportsFunctionCalling {
		tools := make([]interface{}, len(prepared.Tools))
		for i, t := range prepared.Tools {
			tools[i] = map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"input_schema": t.Parameters,
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

// Execute sends the request to the Anthropic API.
func (p *AnthropicProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	bodyJSON, err := json.Marshal(req)
	if err != nil {
		return agentctx.ProviderResult{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyJSON))
	if err != nil {
		return agentctx.ProviderResult{ShouldRetry: false, Error: err}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

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

// Parse converts the Anthropic response to a canonical ParsedResponse.
func (p *AnthropicProvider) Parse(_ context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	if result.Error != nil {
		return agentctx.ParsedResponse{}, result.Error
	}

	respBody, ok := result.RawResponse.([]byte)
	if !ok {
		return agentctx.ParsedResponse{}, fmt.Errorf("anthropic: unexpected raw response type %T", result.RawResponse)
	}

	var resp anthropicResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return agentctx.ParsedResponse{}, fmt.Errorf("parse response: %w", err)
	}

	if len(resp.Content) == 0 {
		return agentctx.ParsedResponse{Text: "", FinishReason: resp.StopReason}, nil
	}

	parsed := agentctx.ParsedResponse{
		FinishReason: resp.StopReason,
		Usage:        convertAnthropicUsage(resp.Usage),
	}

	// Extract text content and tool calls
	for _, content := range resp.Content {
		if content.Type == "text" {
			parsed.Text += content.Text
		} else if content.Type == "tool_use" {
			args, _ := json.Marshal(content.Input)
			parsed.ToolCalls = append(parsed.ToolCalls, agentctx.ToolCall{
				ID:        content.ID,
				Name:      content.Name,
				Arguments: args,
			})
		}
	}

	return parsed, nil
}

// Internal types for JSON unmarshaling
type anthropicResponse struct {
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type   string                 `json:"type"`
	Text   string                 `json:"text,omitempty"`
	ID     string                 `json:"id,omitempty"`
	Name   string                 `json:"name,omitempty"`
	Input  map[string]interface{} `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func convertAnthropicUsage(u anthropicUsage) *agentctx.UsageInfo {
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		return nil
	}
	return &agentctx.UsageInfo{
		PromptTokens:    u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
	}
}
