package eventbus

import (
	"sync"
	"testing"
)

func TestNewEventBus(t *testing.T) {
	bus := NewEventBus()
	if bus == nil {
		t.Error("NewEventBus should return a non-nil bus")
	}
	if bus.closed {
		t.Error("New event bus should not be closed")
	}
}

func TestEventBus_SubscribeAndEmit(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("test_event")

	bus.Emit(Event{Kind: "test_event", Data: "hello"})

	select {
	case evt := <-ch:
		if evt.Kind != "test_event" {
			t.Errorf("Expected kind 'test_event', got %q", evt.Kind)
		}
		if evt.Data != "hello" {
			t.Errorf("Expected data 'hello', got %v", evt.Data)
		}
	default:
		t.Error("Expected to receive event from subscription")
	}
}

func TestEventBus_WildcardSubscriber(t *testing.T) {
	bus := NewEventBus()
	allCh := bus.SubscribeAll()
	specificCh := bus.Subscribe("tool_call")

	bus.Emit(Event{Kind: "tool_call", Data: "SearchMemories"})
	bus.Emit(Event{Kind: "iteration_start", Data: 1})

	// Wildcard should get both
	 count := 0
	for {
		select {
		case <-allCh:
			count++
		default:
			goto done
		}
	}
	done:
	if count != 2 {
		t.Errorf("Expected 2 events on wildcard, got %d", count)
	}

	// Specific should only get tool_call
	select {
	case <-specificCh:
		// good
	default:
		t.Error("Expected specific subscriber to get tool_call event")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("test")

	bus.Unsubscribe("test", ch)

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("Unsubscribed channel should be closed")
	}
}

func TestEventBus_Close(t *testing.T) {
	bus := NewEventBus()
	ch1 := bus.Subscribe("a")
	ch2 := bus.Subscribe("b")

	bus.Close()

	// Both channels should be closed
	_, ok1 := <-ch1
	_, ok2 := <-ch2
	if ok1 || ok2 {
		t.Error("All channels should be closed after Close()")
	}

	// Emit after close should not panic
	bus.Emit(Event{Kind: "a", Data: "after close"})
}

func TestEventBus_DropsOnFullBuffer(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("flood")

	// Fill the buffer (64 events)
	for i := 0; i < 100; i++ {
		bus.Emit(Event{Kind: "flood", Data: i})
	}

	// Should have ~64 events (buffer size), not 100
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
	done:
	if count > 64 {
		t.Errorf("Expected at most 64 buffered events, got %d", count)
	}
}

func TestEventBus_Concurrent(t *testing.T) {
	bus := NewEventBus()
	ch := bus.SubscribeAll()

	var wg sync.WaitGroup
	// Emit from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bus.Emit(Event{Kind: "concurrent", Data: n})
		}(i)
	}
	wg.Wait()

	// Should receive all events
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
	done:
	if count != 10 {
		t.Errorf("Expected 10 concurrent events, got %d", count)
	}
}

func TestEventBus_NoSubscribers(t *testing.T) {
	bus := NewEventBus()
	// Should not panic
	bus.Emit(Event{Kind: "nobody_home", Data: nil})
}