// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import "testing"

func TestPing(t *testing.T) {
	s := testStore(t)
	if err := s.Ping(); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestPingAfterClose(t *testing.T) {
	s := testStore(t)
	s.Close()
	if err := s.Ping(); err == nil {
		t.Error("expected Ping() to fail after Close()")
	}
}
