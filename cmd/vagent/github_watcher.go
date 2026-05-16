// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
)

// githubWatcher polls GitHub repositories for recent events and stores them as memories.
type githubWatcher struct {
	client     client.MnemoClient
	agentID    string
	repos      []string
	interval   time.Duration
	notify     *notifier
	cancel     context.CancelFunc
	lastSeen   map[string]int64 // repo -> last event timestamp seen
}

func newGitHubWatcher(c client.MnemoClient, agentID string, cfg WatchConfig, notify *notifier) *githubWatcher {
	return &githubWatcher{
		client:   c,
		agentID:  agentID,
		repos:    cfg.Repos,
		interval: time.Duration(cfg.IntervalSec) * time.Second,
		notify:   notify,
		lastSeen: make(map[string]int64),
	}
}

func (w *githubWatcher) start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	// Initial poll
	w.poll(ctx)

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.poll(ctx)
			}
		}
	}()

	slog.Info("GitHub watcher started", "repos", len(w.repos), "interval", w.interval)
}

func (w *githubWatcher) stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *githubWatcher) poll(ctx context.Context) {
	for _, repo := range w.repos {
		w.pollRepo(ctx, repo)
	}
}

type ghEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Repo      struct {
		Name string `json:"name"`
	} `json:"repo"`
	Payload json.RawMessage `json:"payload"`
}

func (w *githubWatcher) pollRepo(ctx context.Context, repo string) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/events?per_page=10", repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vagent/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Warn("GitHub poll failed", "repo", repo, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		slog.Warn("GitHub API non-200", "repo", repo, "status", resp.StatusCode)
		return
	}

	var events []ghEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		slog.Warn("GitHub JSON decode failed", "repo", repo, "error", err)
		return
	}

	lastSeen := w.lastSeen[repo]
	var newEvents int
	for _, e := range events {
		ts := e.CreatedAt.Unix()
		if ts <= lastSeen {
			break // events are returned newest-first
		}

		summary := w.summarizeEvent(e)
		key := fmt.Sprintf("gh-%s-%s", repo, e.ID)
		_, err := w.client.AddMemory(ctx, "memory", "observation", key, summary, 2, w.agentID)
		if err != nil {
			slog.Warn("Failed to store GitHub event", "repo", repo, "event", e.Type, "error", err)
			continue
		}
		newEvents++

		if w.notify != nil && e.Type == "IssuesEvent" || e.Type == "PullRequestEvent" {
			w.notify.send(fmt.Sprintf("GitHub [%s] %s: %s", repo, e.Type, summary))
		}
	}

	if len(events) > 0 {
		w.lastSeen[repo] = events[0].CreatedAt.Unix()
	}

	if newEvents > 0 {
		slog.Info("GitHub events stored", "repo", repo, "new", newEvents)
	}
}

func (w *githubWatcher) summarizeEvent(e ghEvent) string {
	switch e.Type {
	case "PushEvent":
		return w.summarizePush(e)
	case "IssuesEvent":
		return w.summarizeIssue(e)
	case "PullRequestEvent":
		return w.summarizePR(e)
	case "CreateEvent":
		return "Branch or tag created"
	case "DeleteEvent":
		return "Branch or tag deleted"
	case "ForkEvent":
		return "Repository was forked"
	case "WatchEvent":
		return "Repository was starred"
	case "IssueCommentEvent":
		return "Comment added to an issue"
	case "PullRequestReviewEvent":
		return "PR review submitted"
	default:
		return fmt.Sprintf("GitHub event: %s", e.Type)
	}
}

func (w *githubWatcher) summarizePush(e ghEvent) string {
	var payload struct {
		Ref    string `json:"ref"`
		Size   int    `json:"size"`
		Commits []struct {
			Message string `json:"message"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Sprintf("%d commit(s) pushed", 0)
	}

	branch := payload.Ref
	if len(branch) > 11 && branch[:11] == "refs/heads/" {
		branch = branch[11:]
	}

	if len(payload.Commits) == 1 {
		msg := payload.Commits[0].Message
		if idx := strings.Index(msg, "\n"); idx > 0 {
			msg = msg[:idx]
		}
		return fmt.Sprintf("Pushed to %s: %s", branch, msg)
	}
	return fmt.Sprintf("Pushed %d commits to %s", payload.Size, branch)
}

func (w *githubWatcher) summarizeIssue(e ghEvent) string {
	var payload struct {
		Action string `json:"action"`
		Issue  struct {
			Title  string `json:"title"`
			Number int    `json:"number"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return "Issue activity"
	}
	return fmt.Sprintf("Issue #%d %s: %s", payload.Issue.Number, payload.Action, payload.Issue.Title)
}

func (w *githubWatcher) summarizePR(e ghEvent) string {
	var payload struct {
		Action      string `json:"action"`
		PullRequest struct {
			Title  string `json:"title"`
			Number int    `json:"number"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return "PR activity"
	}
	return fmt.Sprintf("PR #%d %s: %s", payload.PullRequest.Number, payload.Action, payload.PullRequest.Title)
}
