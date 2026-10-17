// This file is part of Vassago.
// See LICENSE for license information.

package daemon

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
)

// ListenUnix creates a Unix domain socket listener.
// It removes any stale socket file first and sets permissions to owner-only.
func ListenUnix(socketPath string) (net.Listener, error) {
	// Remove stale socket
	os.Remove(socketPath)

	// Ensure directory exists
	sockDir := dirOfFile(socketPath)
	if err := os.MkdirAll(sockDir, 0700); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on unix socket %s: %w", socketPath, err)
	}

	// Set socket permissions — owner only
	if err := os.Chmod(socketPath, 0600); err != nil {
		l.Close()
		return nil, fmt.Errorf("chmod socket %s: %w", socketPath, err)
	}

	slog.Info("listening on unix socket", "path", socketPath)
	return l, nil
}

// ListenTCP creates a TCP listener on the given address.
func ListenTCP(addr string) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on TCP %s: %w", addr, err)
	}
	slog.Info("listening on TCP", "address", addr)
	return l, nil
}

// dirOfFile returns the directory portion of a file path.
func dirOfFile(path string) string {
	return filepath.Dir(path)
}
