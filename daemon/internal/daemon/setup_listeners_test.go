// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupListeners_unixSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cfg := DaemonConfig{
		Socket: socketPath,
	}

	listeners, err := setupListeners(cfg)
	require.NoError(t, err)
	require.Len(t, listeners, 1)

	assert.FileExists(t, socketPath, "socket file should be created")

	// Clean up
	listeners[0].Close()
	os.Remove(socketPath)
}

func TestSetupListeners_tcp(t *testing.T) {
	cfg := DaemonConfig{
		TCP: "localhost:0", // random port
	}

	listeners, err := setupListeners(cfg)
	require.NoError(t, err)
	require.Len(t, listeners, 1)
	defer listeners[0].Close()

	addr := listeners[0].Addr().String()
	// localhost:0 resolves to 127.0.0.1:PORT at the OS level
	assert.Regexp(t, `^(127\.0\.0\.1|\[::1\]):\d+$`, addr, "should listen on a localhost interface")
}

func TestSetupListeners_both(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	cfg := DaemonConfig{
		Socket: socketPath,
		TCP:    "localhost:0",
	}

	listeners, err := setupListeners(cfg)
	require.NoError(t, err)
	require.Len(t, listeners, 2)

	// Clean up
	for _, l := range listeners {
		l.Close()
	}
	os.Remove(socketPath)
}

func TestSetupListeners_neither(t *testing.T) {
	cfg := DaemonConfig{}

	_, err := setupListeners(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no listener configured")
}
