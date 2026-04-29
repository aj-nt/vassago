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
func (p *OpenAIProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	reqMap, ok := req.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("openai stream: unexpected request type %T", req)
	}
	reqMap["stream"] = true

	bodyJSON, err := json.Marshal(reqMap)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		body, _ := json.Marshal(map[string]interface{}{"status": resp.StatusCode})
		resp.Body.Close()
		return nil, fmt.Errorf("stream request failed with status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan agentctx.StreamEvent, 64)
	go p.readSSEStream(resp, ch)

	return ch, nil
}

// readSSEStream reads Server-Sent Events from the OpenAI streaming response.
func (p *OpenAIProvider) readSSEStream(resp *http.Response, ch chan<- agentctx.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- agentctx.StreamEvent{Type: agentctx.StreamDone, Done: true}
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				TotalTokens int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Text delta
		if choice.Delta.Content != "" {
			ch <- agentctx.StreamEvent{Type: agentctx.StreamText, Text: choice.Delta.Content}
		}

		// Tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			if tc.ID != "" {
				ch <- agentctx.StreamEvent{
					Type:      agentctx.StreamToolStart,
					ToolCallID: tc.ID,
					ToolName:  tc.Function.Name,
				}
			}
			if tc.Function.Arguments != "" {
				ch <- agentctx.StreamEvent{
					Type:           agentctx.StreamToolDelta,
					ToolCallID:     tc.ID,
					ToolInputDelta: tc.Function.Arguments,
				}
			}
		}

		// Finish
		if choice.FinishReason != nil {
			doneEvent := agentctx.StreamEvent{Type: agentctx.StreamDone, Done: true}
			if chunk.Usage != nil {
				doneEvent.Usage = &agentctx.UsageInfo{
					TotalTokens: chunk.Usage.TotalTokens,
				}
			}
			ch <- doneEvent
			return
		}
	}
}