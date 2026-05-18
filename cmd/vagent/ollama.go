package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaClient is a minimal OpenAI-compatible client for Ollama.
type OllamaClient struct {
	baseURL     string
	model       string
	maxTokens   int
	temperature float64
	timeout     time.Duration
	client      *http.Client
}

func newOllamaClient(cfg OllamaConfig) *OllamaClient {
	return &OllamaClient{
		baseURL:     cfg.BaseURL,
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		timeout:     time.Duration(cfg.TimeoutSec) * time.Second,
		client:      &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second},
	}
}

type ollamaChatRequest struct {
	Model       string           `json:"model"`
	Messages    []ollamaMessage  `json:"messages"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature float64          `json:"temperature"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Choices []ollamaChoice `json:"choices"`
}

type ollamaChoice struct {
	Message ollamaMessage `json:"message"`
}

// Chat sends a chat completion request and returns the response text.
func (c *OllamaClient) Chat(messages []ollamaMessage) (string, error) {
	req := ollamaChatRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		end := min(len(respBody), 500)
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody[:end]))
	}

	var cr ollamaChatResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := cr.Choices[0].Message.Content
	if content == "" {
		// GLM-5.1 puts reasoning in a separate field — try that
		var raw map[string]interface{}
		if json.Unmarshal(respBody, &raw) == nil {
			if choices, ok := raw["choices"].([]interface{}); ok && len(choices) > 0 {
				if ch, ok := choices[0].(map[string]interface{}); ok {
					if msg, ok := ch["message"].(map[string]interface{}); ok {
						for _, key := range []string{"reasoning", "reasoning_content"} {
							if r, ok := msg[key].(string); ok && r != "" {
								return r, nil
							}
						}
					}
				}
			}
		}
		return "", fmt.Errorf("empty response content")
	}

	return content, nil
}
