// This file is part of Vassago.
// See LICENSE for license information.

package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// inMemoryStore implements CronStore for unit tests.
type inMemoryStore struct {
	mu   sync.Mutex
	jobs map[string]*CronJob
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{jobs: make(map[string]*CronJob)}
}

func (s *inMemoryStore) CreateCronJob(job *CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *inMemoryStore) GetCronJob(id string) (*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		return j, nil
	}
	return nil, errors.New("not found")
}

func (s *inMemoryStore) ListCronJobs(owner string, enabledOnly bool) ([]*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*CronJob
	for _, j := range s.jobs {
		if (!enabledOnly || j.Enabled) && (owner == "" || j.AgentID == owner) {
			result = append(result, j)
		}
	}
	return result, nil
}

func (s *inMemoryStore) UpdateCronJob(job *CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.ID]; !ok {
		return errors.New("not found")
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *inMemoryStore) DeleteCronJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	return nil
}

func (s *inMemoryStore) GetDueCronJobs(now int64) ([]*CronJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []*CronJob
	for _, j := range s.jobs {
		if j.Enabled && j.NextFireAt > 0 && j.NextFireAt <= now {
			due = append(due, j)
		}
	}
	return due, nil
}

func (s *inMemoryStore) UpdateCronJobFireTimestamps(jobID string, lastFireAt, nextFireAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[jobID]
	if !ok {
		return errors.New("not found")
	}
	j.LastFireAt = lastFireAt
	j.NextFireAt = nextFireAt
	return nil
}

// ── DefaultConfig ──

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("default cron should be disabled")
	}
	if cfg.Interval != 60 {
		t.Errorf("expected interval 60, got %d", cfg.Interval)
	}
	if cfg.MaxJobs != 100 {
		t.Errorf("expected maxJobs 100, got %d", cfg.MaxJobs)
	}
}

// ── NewCronRunner defaults ──

func TestNewCronRunner_DefaultInterval(t *testing.T) {
	store := newInMemoryStore()
	cfg := Config{Enabled: true, Interval: 0, MaxJobs: 50}
	r := NewCronRunner(store, nil, cfg)
	if r.interval != 60*time.Second {
		t.Errorf("zero interval should default to 60s, got %v", r.interval)
	}
}

func TestNewCronRunner_DefaultMaxJobs(t *testing.T) {
	store := newInMemoryStore()
	cfg := Config{Enabled: true, Interval: 30, MaxJobs: 0}
	r := NewCronRunner(store, nil, cfg)
	if r.maxJobs != 100 {
		t.Errorf("zero maxJobs should default to 100, got %d", r.maxJobs)
	}
}

// ── Start / Stop / Running ──

func TestStartStop(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Start(ctx)
	if !r.Running() {
		t.Error("runner should be running after Start")
	}

	r.Stop()
	if r.Running() {
		t.Error("runner should not be running after Stop")
	}
}

func TestStartTwice(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r.Start(ctx)
	r.Start(ctx) // Second start should be a no-op

	if !r.Running() {
		t.Error("runner should still be running")
	}

	r.Stop()
}

// ── RecomputeNextFireTimes ──

func TestRecomputeNextFireTimes_NoJobs(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	err := r.RecomputeNextFireTimes(context.Background())
	if err != nil {
		t.Errorf("recompute with no jobs should succeed: %v", err)
	}
}

func TestRecomputeNextFireTimes_SkipsAlreadySet(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	job := &CronJob{
		ID:         "job-1",
		AgentID:    "agent-a",
		Name:       "test",
		Schedule:   "0 9 * * *",
		Enabled:    true,
		NextFireAt: 9999999999, // already set, should skip
	}
	store.CreateCronJob(job)

	err := r.RecomputeNextFireTimes(context.Background())
	if err != nil {
		t.Errorf("recompute should succeed: %v", err)
	}

	// NextFireAt should remain unchanged
	j, _ := store.GetCronJob("job-1")
	if j.NextFireAt != 9999999999 {
		t.Errorf("already-set NextFireAt should not change, got %d", j.NextFireAt)
	}
}

func TestRecomputeNextFireTimes_InitializesUnset(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	now := time.Now().Unix()
	job := &CronJob{
		ID:         "job-2",
		AgentID:    "agent-a",
		Name:       "test",
		Schedule:   "0 12 * * *",
		Enabled:    true,
		NextFireAt: 0, // uninitialized
	}
	store.CreateCronJob(job)

	err := r.RecomputeNextFireTimes(context.Background())
	if err != nil {
		t.Errorf("recompute should succeed: %v", err)
	}

	j, _ := store.GetCronJob("job-2")
	if j.NextFireAt <= now {
		t.Errorf("NextFireAt should be in the future, got %d (now=%d)", j.NextFireAt, now)
	}
}

func TestRecomputeNextFireTimes_SkipsDisabled(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	job := &CronJob{
		ID:         "job-3",
		AgentID:    "agent-a",
		Name:       "test",
		Schedule:   "0 9 * * *",
		Enabled:    false,
		NextFireAt: 0,
	}
	store.CreateCronJob(job)

	err := r.RecomputeNextFireTimes(context.Background())
	if err != nil {
		t.Errorf("recompute should succeed: %v", err)
	}

	// Disabled job should remain uninitialized
	j, _ := store.GetCronJob("job-3")
	if j.NextFireAt != 0 {
		t.Errorf("disabled job should not be initialized, got %d", j.NextFireAt)
	}
}

func TestRecomputeNextFireTimes_InvalidSchedule(t *testing.T) {
	store := newInMemoryStore()
	r := NewCronRunner(store, nil, Config{Interval: 60})

	job := &CronJob{
		ID:         "job-bad",
		AgentID:    "agent-a",
		Name:       "test",
		Schedule:   "not a cron expression",
		Enabled:    true,
		NextFireAt: 0,
	}
	store.CreateCronJob(job)

	// Should not error — invalid schedules are logged and skipped
	err := r.RecomputeNextFireTimes(context.Background())
	if err != nil {
		t.Errorf("recompute with invalid schedule should not error: %v", err)
	}
}

// ── NextFireTime ──

func TestNextFireTime_SpecificHour(t *testing.T) {
	// "0 15 * * *" = every day at 15:00 UTC
	ref := time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC).Unix()

	next, err := NextFireTime("0 15 * * *", ref)
	if err != nil {
		t.Fatalf("NextFireTime: %v", err)
	}

	nextTime := time.Unix(next, 0).UTC()
	if nextTime.Hour() != 15 || nextTime.Day() != 1 {
		t.Errorf("expected 2025-01-01 15:00, got %s", nextTime.Format(time.RFC3339))
	}
}

func TestNextFireTime_InvalidExpr(t *testing.T) {
	_, err := NextFireTime("not valid", 0)
	if err == nil {
		t.Error("invalid cron expression should return error")
	}
}

// ── parseCronFields ──

func TestParseCronFields_Valid(t *testing.T) {
	fields, err := parseCronFields("0 12 * * 1")
	if err != nil {
		t.Fatalf("parseCronFields: %v", err)
	}
	if len(fields) != 5 {
		t.Errorf("expected 5 fields, got %d", len(fields))
	}
}

func TestParseCronFields_WrongCount(t *testing.T) {
	_, err := parseCronFields("* * *")
	if err == nil {
		t.Error("expected error for 3-field expression")
	}
}

func TestParseCronFields_InvalidField(t *testing.T) {
	_, err := parseCronFields("99 * * * *") // minute 99 is invalid
	if err == nil {
		t.Error("expected error for invalid minute")
	}
}

// ── parseFieldSpec ──

func TestParseFieldSpec_Wildcard(t *testing.T) {
	cf, err := parseFieldSpec("*", 0, 59)
	if err != nil {
		t.Fatalf("parseFieldSpec: %v", err)
	}
	if len(cf.values) != 60 {
		t.Errorf("wildcard should match all 60 values, got %d", len(cf.values))
	}
}

func TestParseFieldSpec_SingleValue(t *testing.T) {
	cf, err := parseFieldSpec("30", 0, 59)
	if err != nil {
		t.Fatalf("parseFieldSpec: %v", err)
	}
	if !cf.values[30] || len(cf.values) != 1 {
		t.Errorf("expected only value 30, got %v", cf.values)
	}
}

func TestParseFieldSpec_Range(t *testing.T) {
	cf, err := parseFieldSpec("1-5", 0, 59)
	if err != nil {
		t.Fatalf("parseFieldSpec: %v", err)
	}
	for i := 1; i <= 5; i++ {
		if !cf.values[i] {
			t.Errorf("expected value %d in range", i)
		}
	}
}

func TestParseFieldSpec_Step(t *testing.T) {
	cf, err := parseFieldSpec("*/15", 0, 59)
	if err != nil {
		t.Fatalf("parseFieldSpec: %v", err)
	}
	if cf.values[0] && cf.values[15] && cf.values[30] && cf.values[45] {
		// correct
	} else {
		t.Error("step should produce 0,15,30,45")
	}
}

func TestParseFieldSpec_CommaSeparated(t *testing.T) {
	cf, err := parseFieldSpec("1,3,5", 0, 59)
	if err != nil {
		t.Fatalf("parseFieldSpec: %v", err)
	}
	if !cf.values[1] || !cf.values[3] || !cf.values[5] {
		t.Error("expected values 1,3,5")
	}
	if len(cf.values) != 3 {
		t.Errorf("expected 3 values, got %d", len(cf.values))
	}
}

func TestParseFieldSpec_OutOfRange(t *testing.T) {
	_, err := parseFieldSpec("99", 0, 59)
	if err == nil {
		t.Error("expected error for out of range value")
	}
}

func TestParseFieldSpec_InvalidStep(t *testing.T) {
	_, err := parseFieldSpec("*/0", 0, 59)
	if err == nil {
		t.Error("step 0 should be invalid")
	}
}

// ── matchesCron ──

func TestMatchesCron_Exact(t *testing.T) {
	// 12:30 on Jan 1 = a Wednesday in 2025
	fields := []cronField{
		{values: map[int]bool{30: true}},  // minute
		{values: map[int]bool{12: true}},  // hour
		{values: map[int]bool{1: true}},   // day
		{values: map[int]bool{1: true}},   // month
		{values: map[int]bool{3: true}},   // weekday (Wed)
	}
	tm := time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC)
	if !matchesCron(tm, fields) {
		t.Error("should match exact fields")
	}
}

func TestMatchesCron_WrongMinute(t *testing.T) {
	fields := []cronField{
		{values: map[int]bool{30: true}},
		{values: map[int]bool{12: true}},
		{values: map[int]bool{1: true}},
		{values: map[int]bool{1: true}},
		{values: map[int]bool{3: true}},
	}
	tm := time.Date(2025, 1, 1, 12, 31, 0, 0, time.UTC)
	if matchesCron(tm, fields) {
		t.Error("should not match wrong minute")
	}
}
