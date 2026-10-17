// This file is part of Vassago.
// See LICENSE for license information.

package main

import (
	"testing"

	"github.com/aj-nt/vassago/internal/ollama"
)

func TestOllamaClient_Creation(t *testing.T) {
	cfg := ollama.Config{
		BaseURL:     "http://localhost:11434/v1",
		Model:       "test-model",
		MaxTokens:   512,
		Temperature: 0.2,
		TimeoutSec:  30,
	}

	client := ollama.NewClient(cfg)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.Model() != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", client.Model())
	}
}

func TestWorkerRunner_Creation(t *testing.T) {
	cfg := &Config{
		DaemonAddr: "localhost:50051",
		AgentID:    "test-agent",
		AgentName:  "Test Agent",
		Worker: WorkerConfig{
			Enabled:    true,
			AgentTypes: []string{"general", "static_analysis"},
			PollSec:    5,
			MaxTurns:   10,
			Ollama: OllamaConfig{
				BaseURL:     "http://localhost:11434/v1",
				Model:       "test-model",
				MaxTokens:   512,
				Temperature: 0.1,
				TimeoutSec:  30,
			},
		},
	}

	if !cfg.Worker.Enabled {
		t.Error("Worker should be enabled")
	}
	if len(cfg.Worker.AgentTypes) != 2 {
		t.Errorf("expected 2 agent types, got %d", len(cfg.Worker.AgentTypes))
	}
}

func TestWorkerRunner_Disabled(t *testing.T) {
	cfg := &Config{}

	// Workers should not be started when disabled
	if cfg.Worker.Enabled {
		t.Error("Worker should be disabled by default")
	}
}
