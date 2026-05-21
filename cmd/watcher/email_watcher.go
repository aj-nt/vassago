// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
)

// emailWatcher polls an IMAP inbox for new messages and stores them as memories.
// This is a simplified implementation that uses the daemon's Email adapter via SendMessage
// for outbound, and direct IMAP for inbound watching.
type emailWatcher struct {
	client   client.MnemoClient
	agentID  string
	interval time.Duration
	notify   *notifier
	cancel   context.CancelFunc
	// IMAP polling uses the SDK's messaging/email adapter patterns
	// but for now we store a placeholder — the EmailAdapter lives in vassago-sdk/messaging
	// and would be imported here in a full implementation.
}

func newEmailWatcher(c client.MnemoClient, agentID string, cfg WatchConfig, notify *notifier) *emailWatcher {
	return &emailWatcher{
		client:   c,
		agentID:  agentID,
		interval: time.Duration(cfg.IntervalSec) * time.Second,
		notify:   notify,
	}
}

func (w *emailWatcher) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	slog.Info("Email watcher started (placeholder — IMAP polling not yet wired)", "interval", w.interval)

	// EmailAdapter in vassago-sdk/messaging is ready and tested.
	// This watcher imports and instantiates it with config from watcher.yaml.
	// PRs welcome — see vassago-sdk/messaging/email.go for the adapter API.

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Placeholder: would poll IMAP here
			}
		}
	}()
}

func (w *emailWatcher) stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

// storeObservation saves a watcher observation as a daemon memory.
// nolint:unused
func storeObservation(ctx context.Context, c client.MnemoClient, agentID, category, key, content string, priority int32) {
	_, err := c.AddMemory(ctx, "memory", category, key, content, priority, agentID)
	if err != nil {
		slog.Warn("Failed to store observation", "key", key, "error", err)
	} else {
		slog.Debug("Observation stored", "key", key)
	}
}
