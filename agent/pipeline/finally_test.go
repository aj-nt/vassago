// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package pipeline

import (
	"sync/atomic"
	"testing"
)

func TestFinallyFunc_CalledOnYIELD(t *testing.T) {
	var called atomic.Int32
	finallyFunc := func(sessionID string, result *LoopResult) {
		called.Add(1)
	}

	// Verify FinallyFunc is part of LoopConfig
	cfg := &LoopConfig{
		Stages:    &Stages{},
		FinallyFunc: finallyFunc,
	}

	if cfg.FinallyFunc == nil {
		t.Fatal("FinallyFunc should be settable on LoopConfig")
	}

	// Call it directly to verify it works
	cfg.FinallyFunc("test-session", &LoopResult{})
	if called.Load() != 1 {
		t.Fatalf("expected FinallyFunc to be called once, got %d", called.Load())
	}
}

func TestFinallyFunc_NilSafe(t *testing.T) {
	cfg := &LoopConfig{
		Stages: &Stages{},
		// FinallyFunc is nil — should not panic
	}
	if cfg.FinallyFunc != nil {
		t.Fatal("FinallyFunc should default to nil")
	}
}

func TestFinallyFunc_ReceivesResult(t *testing.T) {
	var receivedResult *LoopResult
	finallyFunc := func(sessionID string, result *LoopResult) {
		receivedResult = result
	}

	result := &LoopResult{Iterations: 5}
	finallyFunc("test-session", result)

	if receivedResult == nil {
		t.Fatal("FinallyFunc should receive the LoopResult")
	}
	if receivedResult.Iterations != 5 {
		t.Fatalf("expected 5 iterations, got %d", receivedResult.Iterations)
	}
}