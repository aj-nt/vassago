// This file is part of Vassago.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"testing"
	"time"
)

func TestHotBlockCache_Content(t *testing.T) {
	cache := NewHotBlockCache("memory", "agent-1", "[env] python: 3.11", 20, 1)

	if cache.Content() != "[env] python: 3.11" {
		t.Errorf("expected initial content, got %q", cache.Content())
	}
}

func TestHotBlockCache_Update(t *testing.T) {
	cache := NewHotBlockCache("memory", "agent-1", "old content", 12, 1)

	cache.Update("new content", 11, 2)

	if cache.Content() != "new content" {
		t.Errorf("expected updated content, got %q", cache.Content())
	}
	content, _, entryCount, _ := cache.Stat()
	if content != "new content" {
		t.Errorf("Stat: expected new content, got %q", content)
	}
	if entryCount != 2 {
		t.Errorf("Stat: expected entryCount=2, got %d", entryCount)
	}
}

func TestHotBlockCache_Age(t *testing.T) {
	cache := NewHotBlockCache("memory", "agent-1", "content", 7, 1)

	age := cache.Age()
	if age > time.Second {
		t.Errorf("cache should be very fresh, age=%v", age)
	}

	time.Sleep(50 * time.Millisecond)

	age2 := cache.Age()
	if age2 < 40*time.Millisecond {
		t.Errorf("cache should have aged, age=%v", age2)
	}
}

func TestHotBlockCache_concurrentAccess(t *testing.T) {
	cache := NewHotBlockCache("memory", "agent-1", "initial", 7, 1)

	// Simulate concurrent reads and writes
	done := make(chan struct{})

	// Writer goroutine
	go func() {
		defer func() { done <- struct{}{} }()
		for i := 0; i < 100; i++ {
			cache.Update("updated-content", 16, 2)
		}
	}()

	// Reader goroutine
	go func() {
		defer func() { done <- struct{}{} }()
		for i := 0; i < 100; i++ {
			_ = cache.Content()
			_, _, _, _ = cache.Stat()
		}
	}()

	<-done
	<-done
}
