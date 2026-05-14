// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package extraction

import (
	"context"
	"testing"
	"time"
)

func TestScheduler_NewDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DebounceInterval = 0 // disabled

	s := NewScheduler(cfg, nil)
	if s == nil {
		t.Fatal("expected non-nil scheduler, nil means disabled")
	}
	if s.Enabled() {
		t.Error("scheduler should be disabled when DebounceInterval=0")
	}
}

func TestScheduler_NewEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = "test-model" // required for validation

	s := NewScheduler(cfg, nil)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if !s.Enabled() {
		t.Error("scheduler should be enabled with defaults")
	}
}

func TestScheduler_NotifyTriggersDebounce(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	cfg.DebounceInterval = 1 // 1 second for testing

	triggered := make(chan string, 1)
	onTrigger := func(sessionID string) {
		triggered <- sessionID
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewScheduler(cfg, onTrigger)
	s.Start(ctx)
	defer s.Stop()

	// First notify starts the debounce timer
	s.Notify("session-abc")

	// Should trigger after DebounceInterval
	select {
	case sid := <-triggered:
		if sid != "session-abc" {
			t.Errorf("triggered session = %q, want session-abc", sid)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for debounce trigger")
	}
}

func TestScheduler_NotifyResetsTimer(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	cfg.DebounceInterval = 2 // seconds

	triggered := make(chan string, 1)
	onTrigger := func(sessionID string) {
		select {
		case triggered <- sessionID:
		default:
			// Don't block if channel full
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewScheduler(cfg, onTrigger)
	s.Start(ctx)
	defer s.Stop()

	// Fire first notify
	s.Notify("session-X")
	time.Sleep(500 * time.Millisecond)

	// Fire second notify — should reset the 2s timer
	s.Notify("session-X")

	start := time.Now()

	select {
	case <-triggered:
		elapsed := time.Since(start)
		// Should be ~2s from the SECOND notify, not the first
		if elapsed < 1500*time.Millisecond {
			t.Errorf("triggered too early: %v (should be ~2s from second notify)", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for debounce trigger")
	}
}

func TestScheduler_HardCapFires(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	cfg.DebounceInterval = 10  // long debounce
	cfg.HardCapInterval = 1     // 1 second hard cap for testing

	triggered := make(chan string, 1)
	onTrigger := func(sessionID string) {
		select {
		case triggered <- sessionID:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewScheduler(cfg, onTrigger)
	s.Start(ctx)
	defer s.Stop()

	s.Notify("session-hardcap")

	// Hard cap should fire before debounce (1s vs 10s)
	select {
	case <-triggered:
		// Good — hard cap fired
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for hard cap trigger")
	}
}

func TestScheduler_StopPreventsFiring(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = "test-model"
	cfg.DebounceInterval = 1

	fired := false
	onTrigger := func(string) { fired = true }

	ctx, cancel := context.WithCancel(context.Background())

	s := NewScheduler(cfg, onTrigger)
	s.Start(ctx)
	s.Notify("session-z")
	s.Stop()
	cancel()

	time.Sleep(1500 * time.Millisecond)

	if fired {
		t.Error("trigger should NOT fire after Stop()")
	}
}
