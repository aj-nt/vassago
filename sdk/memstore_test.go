package sdk

import (
	"context"
	"testing"
)

func TestMemStoreRemember(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	entry, err := store.Remember(ctx, "AJ uses neovim", "project", 3)
	if err != nil {
		t.Fatalf("Remember: unexpected error: %v", err)
	}
	if entry == nil {
		t.Fatal("Remember: returned nil entry")
	}
	if entry.Content != "AJ uses neovim" {
		t.Errorf("Remember: content = %q, want %q", entry.Content, "AJ uses neovim")
	}
	if entry.Category != "project" {
		t.Errorf("Remember: category = %q, want %q", entry.Category, "project")
	}
	if entry.Priority != 3 {
		t.Errorf("Remember: priority = %d, want 3", entry.Priority)
	}
	if entry.SourceAgent != "test-agent" {
		t.Errorf("Remember: SourceAgent = %q, want %q", entry.SourceAgent, "test-agent")
	}
	if entry.Id == "" {
		t.Error("Remember: Id is empty")
	}
	if len(entry.Id) != 8 {
		t.Errorf("Remember: Id length = %d, want 8", len(entry.Id))
	}
	if entry.CreatedAt == 0 {
		t.Error("Remember: CreatedAt is zero")
	}
	if entry.UpdatedAt == 0 {
		t.Error("Remember: UpdatedAt is zero")
	}
}

func TestMemStoreRememberWithKey(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	entry, err := store.RememberWithKey(ctx, "editor", "AJ uses neovim", "project", 3)
	if err != nil {
		t.Fatalf("RememberWithKey: unexpected error: %v", err)
	}
	if entry.Key != "editor" {
		t.Errorf("RememberWithKey: key = %q, want %q", entry.Key, "editor")
	}
}

func TestMemStoreForget(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	entry, err := store.Remember(ctx, "temporary note", "scratch", 1)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}

	// Verify it exists.
	got, err := store.GetMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("GetMemory before Forget: %v", err)
	}
	if got.Id != entry.Id {
		t.Fatalf("GetMemory: wrong entry returned")
	}

	// Forget it.
	if err := store.Forget(ctx, entry.Id); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// Verify it's gone.
	_, err = store.GetMemory(ctx, entry.Id)
	if err == nil {
		t.Error("Forget: entry still exists after Forget")
	}
}

func TestMemStoreForgetNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	// Forgetting a nonexistent ID should not error (delete is idempotent).
	err := store.Forget(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Forget(nonexistent): unexpected error: %v", err)
	}
}

func TestMemStoreSearch(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.Remember(ctx, "AJ uses neovim with lazy.nvim", "project", 3)
	store.Remember(ctx, "AJ prefers Go over Python for daemons", "preference", 2)
	store.Remember(ctx, "Studio server runs Debian", "environment", 1)

	// Search for "neovim" — should return exactly 1 result.
	results, err := store.Search(ctx, "neovim", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search('neovim'): got %d results, want 1", len(results))
	}
	if results[0].Content != "AJ uses neovim with lazy.nvim" {
		t.Errorf("Search: wrong result: %q", results[0].Content)
	}

	// Search for "AJ" — should match the two AJ entries.
	results, err = store.Search(ctx, "AJ", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Search('AJ'): got %d results, want 2", len(results))
	}

	// Case-insensitive search.
	results, err = store.Search(ctx, "neovim", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search('neovim') case-insensitive: got %d, want 1", len(results))
	}

	// Search limit.
	results, err = store.Search(ctx, "AJ", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Search('AJ', limit=2): got %d results, want 2", len(results))
	}

	// No-match search.
	results, err = store.Search(ctx, "zzz_nonexistent", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search('zzz_nonexistent'): got %d results, want 0", len(results))
	}
}

func TestMemStoreList(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.Remember(ctx, "m1", "cat-a", 3)
	store.Remember(ctx, "m2", "cat-a", 1)
	store.Remember(ctx, "m3", "cat-b", 2)

	// List all.
	all, err := store.List(ctx, nil, nil)
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List(all): got %d, want 3", len(all))
	}

	// Filter by category.
	catA := "cat-a"
	results, err := store.List(ctx, &catA, nil)
	if err != nil {
		t.Fatalf("List(cat-a): %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List(cat-a): got %d, want 2", len(results))
	}

	// Filter by priority.
	minPri := int32(2)
	results, err = store.List(ctx, nil, &minPri)
	if err != nil {
		t.Fatalf("List(minPri=2): %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List(minPri=2): got %d, want 2 (priorities 3 and 2)", len(results))
	}

	// Filter by both.
	results, err = store.List(ctx, &catA, &minPri)
	if err != nil {
		t.Fatalf("List(cat-a, minPri=2): %v", err)
	}
	if len(results) != 1 {
		t.Errorf("List(cat-a, minPri=2): got %d, want 1", len(results))
	}
}

func TestMemStoreGetMemory(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	entry, _ := store.Remember(ctx, "test memory", "test", 1)

	got, err := store.GetMemory(ctx, entry.Id)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Id != entry.Id {
		t.Errorf("GetMemory: got ID %q, want %q", got.Id, entry.Id)
	}
	if got.Content != "test memory" {
		t.Errorf("GetMemory: content = %q", got.Content)
	}
}

func TestMemStoreGetMemoryNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	_, err := store.GetMemory(ctx, "nonexistent")
	if err == nil {
		t.Error("GetMemory(nonexistent): expected error, got nil")
	}
}

func TestMemStoreHotBlock(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	// Empty store.
	hot, err := store.HotBlock(ctx)
	if err != nil {
		t.Fatalf("HotBlock(empty): %v", err)
	}
	if hot == "" {
		t.Error("HotBlock(empty): returned empty string — should have header")
	}

	// Add a memory and verify it shows up.
	store.Remember(ctx, "AJ uses neovim", "project", 3)
	hot, err = store.HotBlock(ctx)
	if err != nil {
		t.Fatalf("HotBlock: %v", err)
	}
	if hot == "" {
		t.Error("HotBlock: returned empty string")
	}
}

func TestMemStoreAgentName(t *testing.T) {
	store := NewMemStore("hermes")
	if store.AgentName() != "hermes" {
		t.Errorf("AgentName: got %q, want %q", store.AgentName(), "hermes")
	}
}

func TestMemStoreIsConnected(t *testing.T) {
	store := NewMemStore("test-agent")
	if !store.IsConnected() {
		t.Error("MemStore.IsConnected() should always return true")
	}
}

func TestMemStoreClose(t *testing.T) {
	store := NewMemStore("test-agent")
	if err := store.Close(); err != nil {
		t.Errorf("Close: unexpected error: %v", err)
	}
}
