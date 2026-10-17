// This file is part of Vassago.
// See LICENSE for license information.

package extraction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPClientConfig configures an HTTP-based LLM client for extraction.
type HTTPClientConfig struct {
	Provider string // "openai", "ollama", or custom provider name
	Model    string
	BaseURL  string // e.g., "http://localhost:11434/v1" for Ollama
	APIKey   string
	Timeout  time.Duration // HTTP request timeout (default: 120s)
}

// NewHTTPClient creates a new HTTP-based LLM client for the extraction pipeline.
func NewHTTPClient(cfg HTTPClientConfig) *HTTPClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = 120 * time.Second
	}
	return &HTTPClient{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

// HTTPClient implements LLMClient via OpenAI-compatible chat completions API.
type HTTPClient struct {
	cfg    HTTPClientConfig
	client *http.Client
}

// Complete sends a prompt to the LLM and returns the response text.
func (c *HTTPClient) Complete(prompt string) (string, error) {
	url := c.cfg.BaseURL + "/chat/completions"

	body := map[string]interface{}{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  1024,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM request returned status %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode LLM response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty choices in LLM response")
	}

	return result.Choices[0].Message.Content, nil
}
