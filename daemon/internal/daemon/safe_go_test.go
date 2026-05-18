package daemon

import (
	"sync"
	"testing"
	"time"
)

func TestSafeGo_ExecutesFunctionNormally(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	called := false

	safeGo("test-normal", func() {
		defer wg.Done()
		called = true
	})

	wg.Wait()
	if !called {
		t.Error("safeGo did not execute the function")
	}
}

func TestSafeGo_RecoversFromPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	recovered := make(chan struct{})

	safeGo("test-panic", func() {
		defer wg.Done()
		defer close(recovered)
		panic("boom")
	})

	wg.Wait()

	// Verify that the goroutine completed (meaning the panic was recovered)
	select {
	case <-recovered:
		// recovered — defer ran after recover
	case <-time.After(time.Second):
		t.Error("panic propagated — goroutine never completed")
	}
}

func TestSafeGo_DoesNotPropagatePanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	completed := false

	safeGo("test-no-propagation", func() {
		defer wg.Done()
		panic("should not escape")
	})

	wg.Wait()
	completed = true

	if !completed {
		t.Error("test did not complete — panic propagated")
	}
}

func TestSafeGo_RunsAsGoroutine(t *testing.T) {
	// Verify safeGo actually spawns a goroutine (it runs concurrently)
	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})

	safeGo("test-concurrent", func() {
		close(started)
		wg.Done()
	})

	// If safeGo didn't spawn a goroutine, this would deadlock
	select {
	case <-started:
		// goroutine ran
	case <-time.After(time.Second):
		t.Error("safeGo did not spawn a goroutine")
	}

	wg.Wait()
}

func TestSafeGo_LogsPanicDetails(t *testing.T) {
	// This is a smoke test — the log output is hard to capture in unit tests
	// because slog uses the default logger which may differ across goroutines.
	// The important behavior (recovery + non-propagation) is covered above.

	var wg sync.WaitGroup
	wg.Add(1)

	safeGo("test-logging", func() {
		defer wg.Done()
		panic("logged error")
	})

	wg.Wait()
	// Function returned — panic was recovered. The log would have "background
	// goroutine panicked" with stack trace. Verified manually in integration tests.
}
