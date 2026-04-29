package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
)

// Stream sends the request with stream:true and returns a channel of streaming events.
func (p *AnthropicProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	// Add stream:true to the request
	reqMap, ok := req.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("anthropic stream: unexpected request type %T", req)
	}
	reqMap["stream"] = true

	bodyJSON, err := json.Marshal(reqMap)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("stream request failed with status %d", resp.StatusCode)
	}

	ch := make(chan agentctx.StreamEvent, 64)
	go p.readSSEStream(resp, ch)

	return ch, nil
}

// readSSEStream reads Server-Sent Events from the Anthropic streaming response.
// Anthropic SSE format:
//   event: message_start
//   data: {"type":"message_start","message":{...}}
//
//   event: content_block_start
//   data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
//   data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_...","name":"ToolName","input":{}}}
//
//   event: content_block_delta
//   data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}
//   data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"..."}}
//
//   event: content_block_stop
//   data: {"type":"content_block_stop","index":0}
//
//   event: message_delta
//   data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":24}}
//
//   event: message_stop
//   data: {"type":"message_stop"}
func (p *AnthropicProvider) readSSEStream(resp *http.Response, ch chan<- agentctx.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var currentToolID string
	var currentToolName string

	for scanner.Scan() {
		line := scanner.Text()

		// Only process data: lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}

		// Parse the outer event type
		var rawEvent struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(data), &rawEvent); err != nil {
			continue
		}

		switch rawEvent.Type {
		case "content_block_delta":
			var event struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text,omitempty"`
					PartialJSON string `json:"partial_json,omitempty"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			switch event.Delta.Type {
			case "text_delta":
				ch <- agentctx.StreamEvent{Type: agentctx.StreamText, Text: event.Delta.Text}
			case "input_json_delta":
				ch <- agentctx.StreamEvent{
					Type:           agentctx.StreamToolDelta,
					ToolCallID:     currentToolID,
					ToolName:       currentToolName,
					ToolInputDelta: event.Delta.PartialJSON,
				}
			}

		case "content_block_start":
			var event struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id,omitempty"`
					Name string `json:"name,omitempty"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			if event.ContentBlock.Type == "tool_use" {
				currentToolID = event.ContentBlock.ID
				currentToolName = event.ContentBlock.Name
				ch <- agentctx.StreamEvent{
					Type:       agentctx.StreamToolStart,
					ToolCallID: event.ContentBlock.ID,
					ToolName:   event.ContentBlock.Name,
				}
			}

		case "content_block_stop":
			if currentToolID != "" {
				ch <- agentctx.StreamEvent{
					Type:       agentctx.StreamToolEnd,
					ToolCallID: currentToolID,
					ToolName:   currentToolName,
				}
				currentToolID = ""
				currentToolName = ""
			}

		case "message_delta":
			var event struct {
				Delta struct {
					StopReason string `json:"stop_reason,omitempty"`
				} `json:"delta"`
				Usage *struct {
					OutputTokens int `json:"output_tokens,omitempty"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			doneEvent := agentctx.StreamEvent{Type: agentctx.StreamDone, Done: true}
			if event.Usage != nil {
				doneEvent.Usage = &agentctx.UsageInfo{
					CompletionTokens: event.Usage.OutputTokens,
				}
			}
			ch <- doneEvent
			return

		case "message_start", "message_stop", "ping":
			// Metadata events — no action needed
		}
	}
}