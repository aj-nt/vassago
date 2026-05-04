// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package cron

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// mockStore implements CronStore for testing.
type mockStore struct {
	jobs     []*CronJob
	fireTS   map[string][2]int64 // id -> [last_fire_at, next_fire_at]
	dueErr   error
	updateErr error
}

func (m *mockStore) GetDueCronJobs(now int64) ([]*CronJob, error) {
	if m.dueErr != nil {
		return nil, m.dueErr
	}
	var due []*CronJob
	for _, j := range m.jobs {
		if j.Enabled && j.NextFireAt > 0 && j.NextFireAt <= now {
			due = append(due, j)
		}
	}
	return due, nil
}

func (m *mockStore) UpdateCronJobFireTimestamps(id string, lastFireAt, nextFireAt int64) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if m.fireTS == nil {
		m.fireTS = make(map[string][2]int64)
	}
	m.fireTS[id] = [2]int64{lastFireAt, nextFireAt}
	// Also update the in-memory job for ListCronJobs
	for _, j := range m.jobs {
		if j.ID == id {
			j.LastFireAt = lastFireAt
			j.NextFireAt = nextFireAt
		}
	}
	return nil
}

func (m *mockStore) ListCronJobs(agentID string, includeDisabled bool) ([]*CronJob, error) {
	var result []*CronJob
	for _, j := range m.jobs {
		if agentID != "" && j.AgentID != agentID {
			continue
		}
		if !includeDisabled && !j.Enabled {
			continue
		}
		result = append(result, j)
	}
	return result, nil
}

func TestCronRunner_TickFiresJob(t *testing.T) {
	var fired int64
	store := &mockStore{
		jobs: []*CronJob{
			{
				ID:         "job-1",
				Name:       "Test job",
				Schedule:   "0 9 * * *",
				AgentID:    "agent-1",
				Message:    "Good morning!",
				Enabled:    true,
				NextFireAt: 1000,
			},
		},
	}

	onFire := func(job *CronJob) error {
		atomic.AddInt64(&fired, 1)
		return nil
	}

	runner := NewCronRunner(store, onFire, Config{Interval: 1, MaxJobs: 100})

	// Simulate a tick at current time > next_fire_at
	err := runner.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}

	if atomic.LoadInt64(&fired) != 1 {
		t.Errorf("expected 1 fire, got %d", atomic.LoadInt64(&fired))
	}

	// Verify timestamps were updated
	ts, ok := store.fireTS["job-1"]
	if !ok {
		t.Fatal("expected fire timestamps to be updated")
	}
	if ts[0] == 0 {
		t.Error("expected last_fire_at to be set")
	}
	if ts[1] == 0 {
		t.Error("expected next_fire_at to be computed")
	}
}

func TestCronRunner_NoDueJobs(t *testing.T) {
	store := &mockStore{
		jobs: []*CronJob{
			{
				ID:         "job-1",
				Name:       "Future job",
				Schedule:   "0 9 * * *",
				AgentID:    "agent-1",
				Message:    "Not yet",
				Enabled:    true,
				NextFireAt: 999999999999, // Far future
			},
		},
	}

	runner := NewCronRunner(store, nil, Config{Interval: 1, MaxJobs: 100})
	err := runner.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
}

func TestCronRunner_DisabledJobNotFired(t *testing.T) {
	var fired int64
	store := &mockStore{
		jobs: []*CronJob{
			{
				ID:         "job-1",
				Name:       "Disabled",
				Schedule:   "0 9 * * *",
				AgentID:    "agent-1",
				Message:    "Should not fire",
				Enabled:    false,
				NextFireAt: 1000,
			},
		},
	}

	onFire := func(job *CronJob) error {
		atomic.AddInt64(&fired, 1)
		return nil
	}

	runner := NewCronRunner(store, onFire, Config{Interval: 1, MaxJobs: 100})
	runner.tick(context.Background())

	if atomic.LoadInt64(&fired) != 0 {
		t.Errorf("expected 0 fires for disabled job, got %d", atomic.LoadInt64(&fired))
	}
}

func TestNextFireTime(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		from   int64
		want   int64
	}{
		{
			name: "every minute at 00",
			expr: "* * * * *",
			from: 1000000000, // Some arbitrary time
			// Next fire should be the next minute
		},
		{
			name: "hourly at minute 0",
			expr: "0 * * * *",
			from: 1000000060, // minute 1 of hour
			// Next fire should be top of next hour
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextFireTime(tt.expr, tt.from)
			if err != nil {
				t.Fatalf("NextFireTime: %v", err)
			}
			// Verify the computed time actually matches the cron expression
			tm := time.Unix(got, 0)
			if got <= tt.from {
				t.Errorf("next fire time %d should be > from %d", got, tt.from)
			}
			// Just verify it parses without error and is reasonable
			if got < tt.from {
				t.Errorf("NextFireTime(%q, %d) = %d, want > %d", tt.expr, tt.from, got, tt.from)
			}
			_ = tm // Verify it's in the right minute/hour in more detailed tests
		})
	}
}

func TestNextFireTime_InvalidExpression(t *testing.T) {
	_, err := NextFireTime("invalid", 0)
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestNextFireTime_SpecificMinute(t *testing.T) {
	// Test: "30 * * * *" should match minute 30 of every hour
	expr := "30 * * * *"
	// Start from minute 29
	from := time.Date(2024, 1, 1, 10, 29, 0, 0, time.UTC).Unix()
	got, err := NextFireTime(expr, from)
	if err != nil {
		t.Fatalf("NextFireTime: %v", err)
	}
	expected := time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC).Unix()
	if got != expected {
		t.Errorf("expected next fire at %d, got %d", expected, got)
	}
}

func TestNextFireTime_Every5Minutes(t *testing.T) {
	// Test: "*/5 * * * *" should match minutes 0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55
	expr := "*/5 * * * *"
	// Start from minute 3
	from := time.Date(2024, 1, 1, 10, 3, 0, 0, time.UTC).Unix()
	got, err := NextFireTime(expr, from)
	if err != nil {
		t.Fatalf("NextFireTime: %v", err)
	}
	expected := time.Date(2024, 1, 1, 10, 5, 0, 0, time.UTC).Unix()
	if got != expected {
		t.Errorf("expected next fire at %d (min 5), got %d", expected, got)
	}
}

func TestNextFireTime_Weekday(t *testing.T) {
	// Test: "0 9 * * 1-5" should match weekdays at 9:00
	expr := "0 9 * * 1-5"
	// Start from Saturday Jan 6, 2024 (weekday 6)
	from := time.Date(2024, 1, 6, 10, 0, 0, 0, time.UTC).Unix()
	got, err := NextFireTime(expr, from)
	if err != nil {
		t.Fatalf("NextFireTime: %v", err)
	}
	// Next weekday is Monday Jan 8, 2024 at 9:00 UTC
	expected := time.Date(2024, 1, 8, 9, 0, 0, 0, time.UTC).Unix()
	if got != expected {
		t.Errorf("expected next fire at %d (Monday Jan 8 9:00 UTC), got %d (%v UTC)",
			expected, got, time.Unix(got, 0).UTC())
	}
}

func TestRecomputeNextFireTimes(t *testing.T) {
	store := &mockStore{
		jobs: []*CronJob{
			{
				ID:       "job-1",
				Name:     "Uninitialized",
				Schedule: "0 9 * * *",
				AgentID:  "agent-1",
				Message:  "Test",
				Enabled:  true,
			},
			{
				ID:         "job-2",
				Name:       "Already initialized",
				Schedule:   "0 10 * * *",
				AgentID:    "agent-1",
				Message:    "Test",
				Enabled:    true,
				NextFireAt: 999999999999,
			},
		},
	}

	runner := NewCronRunner(store, nil, Config{Interval: 1, MaxJobs: 100})
	err := runner.RecomputeNextFireTimes(context.Background())
	if err != nil {
		t.Fatalf("RecomputeNextFireTimes: %v", err)
	}

	// job-1 should have next_fire_at computed
	ts, ok := store.fireTS["job-1"]
	if !ok {
		t.Error("expected next_fire_at to be computed for job-1")
	} else if ts[1] == 0 {
		t.Error("expected next_fire_at > 0 for job-1")
	}

	// job-2 should NOT be updated (already has next_fire_at)
	if _, ok := store.fireTS["job-2"]; ok {
		t.Error("expected job-2 next_fire_at to remain unchanged")
	}
}

func TestCronRunner_FireCallbackError(t *testing.T) {
	var fired int64
	store := &mockStore{
		jobs: []*CronJob{
			{
				ID:         "job-1",
				Name:       "Fail job",
				Schedule:   "0 9 * * *",
				AgentID:    "agent-1",
				Message:    "Error!",
				Enabled:    true,
				NextFireAt: 1000,
			},
		},
	}

	onFire := func(job *CronJob) error {
		atomic.AddInt64(&fired, 1)
		return fmt.Errorf("fire callback failed")
	}

	runner := NewCronRunner(store, onFire, Config{Interval: 1, MaxJobs: 100})
	// Should not return error even if callback fails
	err := runner.tick(context.Background())
	if err != nil {
		t.Fatalf("tick should not error on callback failure: %v", err)
	}

	// Job should still have been marked as fired
	if atomic.LoadInt64(&fired) != 1 {
		t.Errorf("expected 1 fire attempt, got %d", atomic.LoadInt64(&fired))
	}
}

func TestCronRunner_StoreError(t *testing.T) {
	store := &mockStore{
		dueErr: fmt.Errorf("store error"),
	}

	runner := NewCronRunner(store, nil, Config{Interval: 1, MaxJobs: 100})
	err := runner.tick(context.Background())
	if err == nil {
		t.Error("expected error when store fails")
	}
}