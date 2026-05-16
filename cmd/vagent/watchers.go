// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"log/slog"
	"os"

	"github.com/aj-nt/vassago-sdk/client"
)

// startWatchers starts all configured watchers.
func startWatchers(c client.MnemoClient, agentID string, cfg *Config) []Watcher {
	notify := newNotifier(c, cfg.Notify)
	var watchers []Watcher

	if cfg.GitHub.Enabled {
		w := newGitHubWatcher(c, agentID, cfg.GitHub, notify)
		w.start()
		watchers = append(watchers, w)
	}

	if cfg.Email.Enabled {
		w := newEmailWatcher(c, agentID, cfg.Email, notify)
		w.start()
		watchers = append(watchers, w)
	}

	if len(watchers) == 0 {
		slog.Warn("No watchers enabled — vagent has nothing to do")
		slog.Info("Create a config at ~/.vassago/vagent.yaml and enable github or email watchers")
		os.Exit(1)
	}

	return watchers
}

func slogFatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func slogInfo(msg string, args ...any) {
	args = append([]any{"msg", msg}, args...)
	// slog.Info already formats msg as first positional; just call directly
	slog.Info(msg, args...)
}
