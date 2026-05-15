// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"os"
	"path/filepath"
	"testing"
)

// testStore creates an in-memory SQLite store for testing.
func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNew(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	// Verify schema_version table exists
	var version int
	err := s.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != SchemaVersion {
		t.Errorf("schema version = %d, want %d", version, SchemaVersion)
	}
}

func TestNew_createsDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mnemosyne.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNew_enablesWAL(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	var mode string
	err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("check journal mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal mode = %q, want %q", mode, "wal")
	}
}

func TestNew_enablesForeignKeys(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	var fk int
	err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatalf("check foreign keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign keys = %d, want 1", fk)
	}
}
