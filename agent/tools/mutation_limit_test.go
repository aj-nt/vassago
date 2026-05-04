// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"sync"
	"testing"
)

func TestMutationRateLimit_allowsUpToMax(t *testing.T) {
	rl := NewMutationRateLimit(5)
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() call %d: expected true, got false", i+1)
		}
	}
	if rl.Allow() {
		t.Error("Allow() beyond max: expected false, got true")
	}
}

func TestMutationRateLimit_disabled(t *testing.T) {
	rl := NewMutationRateLimit(-1)
	for i := 0; i < 100; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() call %d with disabled limit: expected true", i+1)
		}
	}
}

func TestMutationRateLimit_defaultLimit(t *testing.T) {
	rl := NewMutationRateLimit(0)
	// Should use default of 10
	for i := 0; i < 10; i++ {
		if !rl.Allow() {
			t.Errorf("Allow() call %d with default limit: expected true", i+1)
		}
	}
	if rl.Allow() {
		t.Error("Allow() beyond default limit: expected false, got true")
	}
}

func TestMutationRateLimit_reset(t *testing.T) {
	rl := NewMutationRateLimit(3)
	for i := 0; i < 3; i++ {
		rl.Allow()
	}
	rl.Reset()
	if !rl.Allow() {
		t.Error("Allow() after Reset: expected true")
	}
}

func TestMutationRateLimit_concurrentAccess(t *testing.T) {
	rl := NewMutationRateLimit(100)
	var wg sync.WaitGroup

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				rl.Allow()
			}
		}()
	}

	wg.Wait()

	if rl.Count() != 100 {
		t.Errorf("after 200 concurrent calls with limit 100: count = %d, want 100", rl.Count())
	}
}

func TestMutationRateLimit_string(t *testing.T) {
	rl := NewMutationRateLimit(5)
	rl.Allow()
	s := rl.String()
	if s != "mutation rate limit: 1/5" {
		t.Errorf("String() = %q, want %q", s, "mutation rate limit: 1/5")
	}

	disabled := NewMutationRateLimit(-1)
	s2 := disabled.String()
	if s2 != "mutation rate limit: disabled" {
		t.Errorf("String() disabled = %q, want %q", s2, "mutation rate limit: disabled")
	}
}