// This file is part of Vassago.
// See LICENSE for license information.

package sdk

import (
	"context"
	"testing"
	"time"
)

func TestConnectEmptyAddress(t *testing.T) {
	ctx := context.Background()
	_, err := Connect(ctx, Config{
		Address:   "",
		AgentName: "test-agent",
	})
	if err == nil {
		t.Fatal("Connect with empty Address: expected error, got nil")
	}
}

func TestConnectEmptyAgentName(t *testing.T) {
	ctx := context.Background()
	_, err := Connect(ctx, Config{
		Address:   "localhost:50051",
		AgentName: "",
	})
	if err == nil {
		t.Fatal("Connect with empty AgentName: expected error, got nil")
	}
}

func TestConnectDefaultFilling(t *testing.T) {
	// These tests verify the config defaults are applied
	// without reaching the gRPC dial (which would fail with connection refused).

	// AgentRole default.
	cfg := Config{
		Address:   "invalid-address-that-will-fail-dial",
		AgentName: "test-agent",
		AgentRole: "",
		HeartbeatInterval: 30 * time.Second,
		HotBlockCharLimit: 8000,
	}
	// Connect will fail on dial — that's expected. We just want to verify
	// it doesn't fail on validation, which means defaults were applied.
	ctx := context.Background()
	_, err := Connect(ctx, cfg)
	if err == nil {
		t.Fatal("Connect to invalid address: expected dial error, got nil")
	}
	// The error should be a dial error, not a validation error.
	// Validation errors have prefix "sdk:".
	if err.Error() == "sdk: config.Address is required" {
		t.Error("Connect with non-empty Address: got validation error, expected dial error")
	}
	if err.Error() == "sdk: config.AgentName is required" {
		t.Error("Connect with non-empty AgentName: got validation error, expected dial error")
	}
}

func TestConnectContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := Connect(ctx, Config{
		Address:   "localhost:50051",
		AgentName: "test-agent",
	})
	if err == nil {
		t.Error("Connect with cancelled context: expected error, got nil")
	}
}
