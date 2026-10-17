// This file is part of Vassago.
// See LICENSE for license information.

package replication

import (
	"testing"
)

func TestNewPeer(t *testing.T) {
	cfg := PeerConfig{
		ID:         "test-peer",
		Addr:       "localhost:9999",
		Namespaces: []string{"default"},
	}
	p := NewPeer(cfg)
	if p.ID != "test-peer" {
		t.Errorf("ID = %q, want %q", p.ID, "test-peer")
	}
	if p.Addr != "localhost:9999" {
		t.Errorf("Addr = %q, want %q", p.Addr, "localhost:9999")
	}
	if len(p.Namespaces) != 1 || p.Namespaces[0] != "default" {
		t.Errorf("Namespaces = %v, want [default]", p.Namespaces)
	}
}
