// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package cron

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CronRunner evaluates cron expressions and fires due jobs by calling
// the store to find due jobs, compute next fire times, and update timestamps.
// It triggers events via a callback function (which the daemon wires to
// the pub/sub system or agent invocation).
type CronRunner struct {
	store    CronStore
	onFire   FireCallback
	interval time.Duration
	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	maxJobs  int
}

// CronStore is the interface the runner needs from the store layer.
type CronStore interface {
	GetDueCronJobs(now int64) ([]*CronJob, error)
	UpdateCronJobFireTimestamps(id string, lastFireAt, nextFireAt int64) error
	ListCronJobs(agentID string, includeDisabled bool) ([]*CronJob, error)
}

// CronJob represents a scheduled job (mirrors proto but avoids proto dependency).
type CronJob struct {
	ID         string
	Name       string
	Schedule   string
	AgentID    string
	Message    string
	Enabled    bool
	CreatedAt  int64
	UpdatedAt  int64
	LastFireAt int64
	NextFireAt int64
}

// FireCallback is called when a cron job fires. The callback receives
// the job that fired and should handle the actual notification logic
// (e.g., publishing a pub/sub event or invoking an agent).
type FireCallback func(job *CronJob) error

// Config holds cron runner configuration.
type Config struct {
	Enabled  bool // Whether the cron runner is enabled
	Interval int  // Check interval in seconds (default: 60)
	MaxJobs  int  // Maximum number of cron jobs per agent (default: 100)
}

// DefaultConfig returns sensible defaults for the cron runner.
func DefaultConfig() Config {
	return Config{
		Enabled:  false,
		Interval: 60,
		MaxJobs:  100,
	}
}

// NewCronRunner creates a new cron runner.
func NewCronRunner(store CronStore, onFire FireCallback, cfg Config) *CronRunner {
	if cfg.Interval <= 0 {
		cfg.Interval = 60
	}
	if cfg.MaxJobs <= 0 {
		cfg.MaxJobs = 100
	}
	return &CronRunner{
		store:    store,
		onFire:   onFire,
		interval: time.Duration(cfg.Interval) * time.Second,
		maxJobs:  cfg.MaxJobs,
	}
}

// Start begins the cron runner evaluation loop.
func (r *CronRunner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	ctx, r.cancel = context.WithCancel(ctx)
	r.mu.Unlock()

	go r.run(ctx)
	slog.Info("Cron runner started", "interval", r.interval, "max_jobs", r.maxJobs)
}

// Stop cancels the cron runner evaluation loop.
func (r *CronRunner) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
	}
	r.running = false
	slog.Info("Cron runner stopped")
}

// Running returns whether the cron runner is currently running.
func (r *CronRunner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// run is the main evaluation loop.
func (r *CronRunner) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.tick(ctx); err != nil {
				slog.Error("Cron runner tick failed", "error", err)
			}
		}
	}
}

// tick evaluates due jobs and fires them.
func (r *CronRunner) tick(ctx context.Context) error {
	now := time.Now().Unix()

	dueJobs, err := r.store.GetDueCronJobs(now)
	if err != nil {
		return fmt.Errorf("get due cron jobs: %w", err)
	}

	for _, job := range dueJobs {
		// Compute next fire time from the schedule
		nextFire, err := NextFireTime(job.Schedule, now)
		if err != nil {
			slog.Error("Invalid cron schedule, disabling job",
				"job_id", job.ID, "schedule", job.Schedule, "error", err)
			// Disable invalid schedule — but we don't update via runner, just log
			continue
		}

		// Fire the job
		if r.onFire != nil {
			if err := r.onFire(job); err != nil {
				slog.Error("Cron job fire callback failed",
					"job_id", job.ID, "job_name", job.Name, "error", err)
			} else {
				slog.Info("Cron job fired",
					"job_id", job.ID, "job_name", job.Name, "agent_id", job.AgentID)
			}
		}

		// Update timestamps: last_fire = now, next_fire = computed
		if err := r.store.UpdateCronJobFireTimestamps(job.ID, now, nextFire); err != nil {
			slog.Error("Failed to update cron job timestamps",
				"job_id", job.ID, "error", err)
		}
	}

	return nil
}

// RecomputeNextFireTimes scans all enabled jobs and sets next_fire_at based on
// their schedule. This should be called on startup to initialize next_fire_at
// for newly created or migrated jobs that haven't been computed yet.
func (r *CronRunner) RecomputeNextFireTimes(ctx context.Context) error {
	jobs, err := r.store.ListCronJobs("", true)
	if err != nil {
		return fmt.Errorf("list cron jobs: %w", err)
	}

	for _, job := range jobs {
		if !job.Enabled {
			continue
		}

		// Only recompute if next_fire_at is 0 (uninitialized)
		if job.NextFireAt > 0 {
			continue
		}

		now := time.Now().Unix()
		nextFire, err := NextFireTime(job.Schedule, now)
		if err != nil {
			slog.Error("Invalid cron schedule during recompute",
				"job_id", job.ID, "schedule", job.Schedule, "error", err)
			continue
		}

		if err := r.store.UpdateCronJobFireTimestamps(job.ID, job.LastFireAt, nextFire); err != nil {
			slog.Error("Failed to recompute next_fire_at",
				"job_id", job.ID, "error", err)
		} else {
			slog.Info("Computed next fire time for job",
				"job_id", job.ID, "next_fire_at", time.Unix(nextFire, 0).Format(time.RFC3339))
		}
	}

	return nil
}

// NextFireTime computes the next fire time from a cron expression.
// It returns the Unix timestamp of the next occurrence after `from`.
// Uses UTC for consistent scheduling across deployments.
func NextFireTime(expr string, from int64) (int64, error) {
	fields, err := parseCronFields(expr)
	if err != nil {
		return 0, err
	}

	// Start from the next minute (in UTC)
	t := time.Unix(from, 0).In(time.UTC).Truncate(time.Minute).Add(time.Minute)

	// Limit search to prevent infinite loops (1 year max)
	deadline := t.Add(366 * 24 * time.Hour)

	for t.Before(deadline) {
		if matchesCron(t, fields) {
			return t.Unix(), nil
		}
		// Move forward by one minute
		t = t.Add(time.Minute)
	}

	return 0, fmt.Errorf("no matching time found within 1 year for schedule %q", expr)
}

// parseCronFields parses a 5-field cron expression into field specs.
func parseCronFields(expr string) ([]cronField, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(parts))
	}

	fieldDefs := []struct {
		name string
		min  int
		max  int
	}{
		{"minute", 0, 59},
		{"hour", 0, 23},
		{"day", 1, 31},
		{"month", 1, 12},
		{"weekday", 0, 6},
	}

	fields := make([]cronField, 5)
	for i, part := range parts {
		spec, err := parseFieldSpec(part, fieldDefs[i].min, fieldDefs[i].max)
		if err != nil {
			return nil, fmt.Errorf("invalid %s field %q: %w", fieldDefs[i].name, part, err)
		}
		fields[i] = spec
	}

	return fields, nil
}

// cronField represents a parsed cron field as a set of matching values.
type cronField struct {
	values map[int]bool
}

// parseFieldSpec converts a single cron field (e.g., "*/5", "1,3,5", "1-5") into a set of values.
func parseFieldSpec(field string, min, max int) (cronField, error) {
	cf := cronField{values: make(map[int]bool)}

	// Handle comma-separated values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle step values (e.g., */5, 1-10/2)
		step := 1
		stepIdx := strings.IndexByte(part, '/')
		if stepIdx >= 0 {
			s, err := strconv.Atoi(part[stepIdx+1:])
			if err != nil || s <= 0 {
				return cf, fmt.Errorf("invalid step value in %q", part)
			}
			step = s
			part = part[:stepIdx]
		}

		// Handle range (e.g., 1-5)
		start, end := min, max
		if part == "*" {
			// All values
		} else {
			dashIdx := strings.IndexByte(part, '-')
			if dashIdx >= 0 {
				val, err := strconv.Atoi(part[:dashIdx])
				if err != nil {
					return cf, err
				}
				start = val
				val, err = strconv.Atoi(part[dashIdx+1:])
				if err != nil {
					return cf, err
				}
				end = val
			} else {
				val, err := strconv.Atoi(part)
				if err != nil {
					return cf, err
				}
				start = val
				end = val
			}
		}

		for i := start; i <= end; i += step {
			if i < min || i > max {
				return cf, fmt.Errorf("value %d out of range (%d-%d)", i, min, max)
			}
			cf.values[i] = true
		}
	}

	return cf, nil
}

// matchesCron checks if a time matches a cron field spec.
func matchesCron(t time.Time, fields []cronField) bool {
	// Minute, Hour, Day, Month, Weekday
	if !fields[0].values[t.Minute()] {
		return false
	}
	if !fields[1].values[t.Hour()] {
		return false
	}
	if !fields[2].values[t.Day()] {
		return false
	}
	if !fields[3].values[int(t.Month())] {
		return false
	}
	// Weekday: cron uses 0=Sunday, time.Weekday() uses 0=Sunday too
	if !fields[4].values[int(t.Weekday())] {
		return false
	}
	return true
}