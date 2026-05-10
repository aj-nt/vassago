// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

// Package agentlog provides structured logging via log/slog for the agent.
// Replaces ad-hoc log.Printf with structured key-value logging.
//
// Phase 33: all agent code routes through this package instead of log.Printf.
//
// Two styles:
//   - Structured: agentlog.Info("agent started", "model", "gpt-4o")
//   - Bridge: agentlog.Printf("agent started with model %s", name)
//     (routes through slog.Info with formatted message)
package agentlog

import (
	"fmt"
	"log/slog"
	"sync"
)

var (
	mu     sync.RWMutex
	logger *slog.Logger
)

// Init sets the global logger. Call once at startup.
// If never called, slog.Default() is used (writes to stderr).
func Init(l *slog.Logger) {
	mu.Lock()
	defer mu.Unlock()
	logger = l
}

func get() *slog.Logger {
	mu.RLock()
	l := logger
	mu.RUnlock()
	if l != nil {
		return l
	}
	return slog.Default()
}

// Info logs at INFO level with structured key-value pairs.
func Info(msg string, args ...any) {
	get().Info(msg, args...)
}

// Error logs at ERROR level.
func Error(msg string, args ...any) {
	get().Error(msg, args...)
}

// Warn logs at WARN level.
func Warn(msg string, args ...any) {
	get().Warn(msg, args...)
}

// Debug logs at DEBUG level.
func Debug(msg string, args ...any) {
	get().Debug(msg, args...)
}

// Printf is a bridge from printf-style logging to structured slog.
// It formats the message with fmt.Sprintf and logs at INFO level.
// This makes migration incremental — existing log.Printf calls become
// agentlog.Printf without changing semantics.
//
// For new code, prefer agentlog.Info(key=value pairs).
func Printf(format string, args ...any) {
	get().Info(fmt.Sprintf(format, args...))
}

// Fatalf logs at ERROR level, then panics. Use for unrecoverable startup errors.
func Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	get().Error(msg)
	panic(msg)
}
