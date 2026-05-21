package sdk

import (
	"context"
	"testing"
)

// --- Cron Jobs ---

func TestMemStoreScheduleJob(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	job, err := store.ScheduleJob(ctx, "memory-backup", "0 3 * * *", "Backup memory", true)
	if err != nil {
		t.Fatalf("ScheduleJob: %v", err)
	}
	if job.ID == "" {
		t.Error("ScheduleJob: ID is empty")
	}
	if job.Name != "memory-backup" {
		t.Errorf("ScheduleJob: name = %q, want %q", job.Name, "memory-backup")
	}
	if job.Schedule != "0 3 * * *" {
		t.Errorf("ScheduleJob: schedule = %q", job.Schedule)
	}
	if job.Message != "Backup memory" {
		t.Errorf("ScheduleJob: message = %q", job.Message)
	}
	if job.AgentID != "test-agent" {
		t.Errorf("ScheduleJob: AgentID = %q, want %q", job.AgentID, "test-agent")
	}
	if !job.Enabled {
		t.Error("ScheduleJob: enabled should be true")
	}
	if job.CreatedAt == 0 {
		t.Error("ScheduleJob: CreatedAt is zero")
	}
}

func TestMemStoreScheduleJobDisabled(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	job, err := store.ScheduleJob(ctx, "paused-job", "*/5 * * * *", "Paused task", false)
	if err != nil {
		t.Fatalf("ScheduleJob: %v", err)
	}
	if job.Enabled {
		t.Error("ScheduleJob: disabled job returned Enabled=true")
	}
}

func TestMemStoreListJobsEmpty(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	jobs, err := store.ListJobs(ctx, false)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("ListJobs(empty): got %d jobs, want 0", len(jobs))
	}
}

func TestMemStoreListJobsEnabledOnly(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.ScheduleJob(ctx, "enabled-1", "* * * * *", "msg1", true)
	store.ScheduleJob(ctx, "disabled-1", "* * * * *", "msg2", false)
	store.ScheduleJob(ctx, "enabled-2", "* * * * *", "msg3", true)

	// Only enabled.
	jobs, err := store.ListJobs(ctx, false)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("ListJobs(enabled only): got %d, want 2", len(jobs))
	}
	for _, j := range jobs {
		if !j.Enabled {
			t.Errorf("ListJobs(enabled only): got disabled job %q", j.Name)
		}
	}
}

func TestMemStoreListJobsIncludeDisabled(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.ScheduleJob(ctx, "job-a", "* * * * *", "msg-a", true)
	store.ScheduleJob(ctx, "job-b", "* * * * *", "msg-b", false)

	jobs, err := store.ListJobs(ctx, true)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("ListJobs(includeDisabled): got %d, want 2", len(jobs))
	}
}

func TestMemStoreUpdateJob(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	original, _ := store.ScheduleJob(ctx, "updatable", "0 0 * * *", "original message", true)

	newName := "updated-name"
	newSched := "*/10 * * * *"
	newMsg := "updated message"
	disabled := false

	updated, err := store.UpdateJob(ctx, original.ID, &newName, &newSched, &newMsg, &disabled)
	if err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	if updated.Name != "updated-name" {
		t.Errorf("UpdateJob: name = %q", updated.Name)
	}
	if updated.Schedule != "*/10 * * * *" {
		t.Errorf("UpdateJob: schedule = %q", updated.Schedule)
	}
	if updated.Message != "updated message" {
		t.Errorf("UpdateJob: message = %q", updated.Message)
	}
	if updated.Enabled {
		t.Error("UpdateJob: enabled should be false")
	}
	if updated.ID != original.ID {
		t.Error("UpdateJob: ID changed")
	}
}

func TestMemStoreUpdateJobPartial(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	original, _ := store.ScheduleJob(ctx, "partial-update", "0 12 * * *", "partial", true)

	// Only update name, leave other fields alone.
	newName := "renamed"
	updated, err := store.UpdateJob(ctx, original.ID, &newName, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	if updated.Name != "renamed" {
		t.Errorf("UpdateJob: name = %q", updated.Name)
	}
	if updated.Schedule != "0 12 * * *" {
		t.Errorf("UpdateJob: schedule should be unchanged, got %q", updated.Schedule)
	}
	if updated.Message != "partial" {
		t.Errorf("UpdateJob: message should be unchanged, got %q", updated.Message)
	}
	if !updated.Enabled {
		t.Error("UpdateJob: enabled should be unchanged (true)")
	}
}

func TestMemStoreUpdateJobNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	_, err := store.UpdateJob(context.Background(), "nonexistent", nil, nil, nil, nil)
	if err == nil {
		t.Error("UpdateJob(nonexistent): expected error")
	}
}

func TestMemStoreDeleteJob(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	job, _ := store.ScheduleJob(ctx, "delete-me", "* * * * *", "delete me", true)

	if err := store.DeleteJob(ctx, job.ID); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}

	// Verify gone.
	_, err := store.UpdateJob(ctx, job.ID, nil, nil, nil, nil)
	if err == nil {
		t.Error("DeleteJob: job still exists after deletion")
	}
}

func TestMemStoreDeleteJobNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	if err := store.DeleteJob(context.Background(), "nonexistent"); err != nil {
		t.Errorf("DeleteJob(nonexistent): expected nil, got error: %v", err)
	}
}

func TestMemStoreScheduleJobUniqueIDs(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	j1, _ := store.ScheduleJob(ctx, "job-1", "* * * * *", "m1", true)
	j2, _ := store.ScheduleJob(ctx, "job-2", "* * * * *", "m2", true)

	if j1.ID == j2.ID {
		t.Error("ScheduleJob: two jobs got the same ID")
	}
}
