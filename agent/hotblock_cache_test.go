// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"sync/atomic"
	"testing"

	memoclient "github.com/aj-nt/mnemosyne/client"
)

func TestAgent_HotBlockCache_ReturnsCached(t *testing.T) {
	cfg := Config{
		AgentID:   "test-agent",
		AgentName: "test",
		AgentRole: "dev",
	}
	a := NewAgent(cfg, nil, nil, &memoclient.NullMnemo{})
	defer a.Close()

	// Set up cache with initial content
	cache := memoclient.NewHotBlockCache("memory", "test-agent", "[env] python: 3.11", 20, 1)
	a.SetHotBlockCache(cache)

	// With NullMnemo and fresh cache, GetHotBlock should return cached content
	hb, err := a.GetHotBlock(context.Background())
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if hb != "[env] python: 3.11" {
		t.Errorf("Expected cached content, got %q", hb)
	}
}

func TestAgent_HotBlockCache_StaleFallsThrough(t *testing.T) {
	cfg := Config{
		AgentID:   "test-agent",
		AgentName: "test",
		AgentRole: "dev",
	}
	a := NewAgent(cfg, nil, nil, &memoclient.NullMnemo{})
	defer a.Close()

	// Set up cache with initial content
	cache := memoclient.NewHotBlockCache("memory", "test-agent", "[env] python: 3.11", 20, 1)
	a.SetHotBlockCache(cache)

	// Invalidate the cache
	a.InvalidateHotBlock()

	// With NullMnemo and stale cache, GetHotBlock should return empty (no daemon to refetch)
	hb, err := a.GetHotBlock(context.Background())
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	// Since Mnemo is NullMnemo (not connected), stale path falls through to GetHotBlock → returns ""
	if hb != "" {
		t.Errorf("Expected empty result with NullMnemo and stale cache, got %q", hb)
	}
}

func TestAgent_InvalidateHotBlock_SetsStaleFlag(t *testing.T) {
	cfg := Config{
		AgentID:   "test-agent",
		AgentName: "test",
		AgentRole: "dev",
	}
	a := NewAgent(cfg, nil, nil, &memoclient.NullMnemo{})
	defer a.Close()

	// Initially not stale
	if atomic.LoadInt32(&a.hotBlockStale) != 0 {
		t.Error("Expected hotBlockStale to be 0 initially")
	}

	// After invalidation
	a.InvalidateHotBlock()
	if atomic.LoadInt32(&a.hotBlockStale) != 1 {
		t.Error("Expected hotBlockStale to be 1 after InvalidateHotBlock")
	}
}

func TestAgent_HotBlockCache_NoCache_NilMnemo(t *testing.T) {
	cfg := Config{
		AgentID:   "test-agent",
		AgentName: "test",
		AgentRole: "dev",
	}
	a := NewAgent(cfg, nil, nil, &memoclient.NullMnemo{})
	defer a.Close()

	// No cache, NullMnemo — should return empty string
	hb, err := a.GetHotBlock(context.Background())
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if hb != "" {
		t.Errorf("Expected empty result with no cache and NullMnemo, got %q", hb)
	}
}

func TestAgent_HotBlockCache_EmptyCache_FallsThrough(t *testing.T) {
	cfg := Config{
		AgentID:   "test-agent",
		AgentName: "test",
		AgentRole: "dev",
	}
	a := NewAgent(cfg, nil, nil, &memoclient.NullMnemo{})
	defer a.Close()

	// Set up cache with empty content
	cache := memoclient.NewHotBlockCache("memory", "test-agent", "", 0, 0)
	a.SetHotBlockCache(cache)

	// Empty content in cache should fall through to Mnemo (not connected) → returns ""
	hb, err := a.GetHotBlock(context.Background())
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if hb != "" {
		t.Errorf("Expected empty result, got %q", hb)
	}
}

func TestAgent_HotBlockCache_MultipleInvalidations(t *testing.T) {
	cfg := Config{
		AgentID:   "test-agent",
		AgentName: "test",
		AgentRole: "dev",
	}
	a := NewAgent(cfg, nil, nil, &memoclient.NullMnemo{})
	defer a.Close()

	cache := memoclient.NewHotBlockCache("memory", "test-agent", "initial content", 16, 1)
	a.SetHotBlockCache(cache)

	// First call returns cached content
	hb, err := a.GetHotBlock(context.Background())
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if hb != "initial content" {
		t.Errorf("Expected cached content, got %q", hb)
	}

	// Invalidate once
	a.InvalidateHotBlock()

	// After invalidation with NullMnemo: falls through to GetHotBlock on NullMnemo → ""
	hb, err = a.GetHotBlock(context.Background())
	if err != nil {
		t.Fatalf("GetHotBlock failed: %v", err)
	}
	if hb != "" {
		t.Errorf("Expected empty after stale+NullMnemo, got %q", hb)
	}

	// Invalidate again — still stale
	a.InvalidateHotBlock()
	if atomic.LoadInt32(&a.hotBlockStale) != 1 {
		t.Error("Expected hotBlockStale to still be 1")
	}
}