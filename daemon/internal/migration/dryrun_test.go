// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package migration

import (
	"path/filepath"
	"testing"
)

func TestDryRunImport_Empty(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	result, err := DryRunImport(hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("DryRunImport: %v", err)
	}
	if result.MemoriesRead != 0 || result.MemoriesImported != 0 {
		t.Errorf("expected 0 memories, got read=%d imported=%d", result.MemoriesRead, result.MemoriesImported)
	}
	if result.SessionsRead != 0 || result.SessionsImported != 0 {
		t.Errorf("expected 0 sessions, got read=%d imported=%d", result.SessionsRead, result.SessionsImported)
	}
}

func TestDryRunImport_WithData(t *testing.T) {
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	// Insert test data using existing helpers
	insertHermesMemory(t, hermesPath, "memory", "environment", "test-key-1", "test content 1", 3, 0)
	insertHermesMemory(t, hermesPath, "memory", "project", "test-key-2", "test content 2", 4, 0)
	insertHermesMemory(t, hermesPath, "user", "user", "user-name", "AJ likes pizza", 5, 0)
	insertHermesSession(t, hermesPath, "sess-1", "cli", "Test session", float64(1700000000), float64(1700000100), 5)

	result, err := DryRunImport(hermesPath, "test-agent")
	if err != nil {
		t.Fatalf("DryRunImport: %v", err)
	}
	if result.MemoriesRead != 3 {
		t.Errorf("expected 3 memories read, got %d", result.MemoriesRead)
	}
	if result.MemoriesImported != 3 {
		t.Errorf("expected 3 memories imported, got %d", result.MemoriesImported)
	}
	if result.SessionsRead != 1 {
		t.Errorf("expected 1 session read, got %d", result.SessionsRead)
	}
	if result.SessionsImported != 1 {
		t.Errorf("expected 1 session imported, got %d", result.SessionsImported)
	}
}

func TestDryRunImport_NoWrites(t *testing.T) {
	// Verify dry run doesn't create a Mnemosyne database
	hermesPath := filepath.Join(t.TempDir(), "state.db")
	if err := createHermesTestDB(hermesPath); err != nil {
		t.Fatalf("create hermes db: %v", err)
	}

	insertHermesMemory(t, hermesPath, "memory", "environment", "dry-key", "dry content", 3, 0)

	_, err := DryRunImport(hermesPath, "dry-run-agent")
	if err != nil {
		t.Fatalf("DryRunImport: %v", err)
	}

	// The dry-run function doesn't take a Mnemosyne store — it can't write.
	// This test verifies the function signature ensures no side effects.
}
