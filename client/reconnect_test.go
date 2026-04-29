package client

import (
	"context"
	"testing"
	"time"
)

func TestDefaultReconnectPolicy(t *testing.T) {
	policy := DefaultReconnectPolicy()
	if policy.MaxRetries != 10 {
		t.Errorf("expected MaxRetries=10, got %d", policy.MaxRetries)
	}
	if policy.InitialBackoff != 500*time.Millisecond {
		t.Errorf("expected InitialBackoff=500ms, got %v", policy.InitialBackoff)
	}
	if policy.MaxBackoff != 30*time.Second {
		t.Errorf("expected MaxBackoff=30s, got %v", policy.MaxBackoff)
	}
	if policy.Multiplier != 2.0 {
		t.Errorf("expected Multiplier=2.0, got %f", policy.Multiplier)
	}
}

func TestResilientClient_Close(t *testing.T) {
	// Verify that closing without connection doesn't panic
	rc := &ResilientClient{closed: false}
	if err := rc.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
	if !rc.closed {
		t.Error("expected closed=true after Close()")
	}
}

func TestResilientClient_OperationsAfterClose(t *testing.T) {
	rc := &ResilientClient{closed: true}
	ctx := context.Background()

	_, err := rc.AddMemory(ctx, "memory", "environment", "test", "content", 3, "test")
	if err == nil {
		t.Error("expected error after Close()")
	}

	_, err = rc.GetMemory(ctx, "test-id")
	if err == nil {
		t.Error("expected error after Close()")
	}

	err = rc.RemoveMemory(ctx, "test-id")
	if err == nil {
		t.Error("expected error after Close()")
	}

	_, err = rc.SearchMemories(ctx, "test", "memory", 10)
	if err == nil {
		t.Error("expected error after Close()")
	}

	err = rc.HealthCheck(ctx)
	if err == nil {
		t.Error("expected error after Close()")
	}
}

func TestRawConnection(t *testing.T) {
	// Test with invalid address to verify error handling
	conn, _, err := RawConnection("invalid-address:99999")
	if err != nil {
		t.Fatalf("RawConnection should not fail on dial: %v", err)
	}
	conn.Close()
}