package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// AnthropicProvider implements Provider for Anthropic Messages API.
type AnthropicProvider struct {
	apiKey       string
	model        string
	client       *http.Client
	capabilities agentctx.ProviderCapabilities
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	return &AnthropicProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
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
	// Build Anthropic-format messages (no system role — system goes in top-level field)
	messages := make([]map[string]interface{}, 0)

	for _, msg := range prepared.Messages {
		// Skip system messages — they go in the top-level system field
		if msg.Role == "system" {
			continue
		}

		// Handle tool results: Anthropic expects role="user" with tool_result content blocks
		if msg.Role == "tool" && msg.ToolCallID != "" {
			resultContent := msg.Content
			if resultContent == "" {
				resultContent = "(empty result)"
			}
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":       "tool_result",
						"tool_use_id": msg.ToolCallID,
						"content":     resultContent,
					},
				},
			})
			continue
		}

		// Handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			content := make([]map[string]interface{}, 0)
			// Add text content if present
			if msg.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			// Add tool_use blocks
			for _, tc := range msg.ToolCalls {
				var input interface{}
				if err := json.Unmarshal(tc.Arguments, &input); err != nil {
					input = map[string]interface{}{}
				}
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			messages = append(messages, map[string]interface{}{
				"role":    "assistant",
				"content": content,
			})
			continue
		}

		// Regular text messages
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	// Build request body
	body := map[string]interface{}{
		"model":      p.model,
		"messages":   messages,
		"max_tokens": 4096,
	}

	// System prompt goes in the top-level system field (not as a message)
	if prepared.SystemPrompt != "" {
		body["system"] = prepared.SystemPrompt
	}

	// Add tools (Anthropic format)
	if len(prepared.Tools) > 0 && p.capabilities.SupportsFunctionCalling {
		tools := make([]map[string]interface{}, 0, len(prepared.Tools))
		for _, t := range prepared.Tools {
			tool := map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"input_schema": t.Parameters,
			}
			tools = append(tools, tool)
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

	return classifyHTTPResponse(resp), nil
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
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
	}
}