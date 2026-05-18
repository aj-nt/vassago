// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
)

// notifier sends notifications through the daemon's messaging service.
type notifier struct {
	client   client.MnemoClient
	platform string
	target   string
}

func newNotifier(c client.MnemoClient, cfg NotifyConfig) *notifier {
	if cfg.Platform == "" || cfg.Target == "" {
		return nil
	}
	return &notifier{
		client:   c,
		platform: cfg.Platform,
		target:   cfg.Target,
	}
}

func (n *notifier) send(message string) {
	if n == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := n.client.SendMessage(ctx, n.platform, n.target, message)
	if err != nil {
		slog.Warn("Notification failed", "platform", n.platform, "target", n.target, "error", err)
	}
}
