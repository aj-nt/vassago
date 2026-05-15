// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"
)

func TestAddCronJob(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-1", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	job, err := s.AddCronJob("default", "Daily report", "0 9 * * *", "test-agent-1", "Generate daily report", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}

	if job.Id == "" {
		t.Error("expected non-empty job ID")
	}
	if job.Name != "Daily report" {
		t.Errorf("expected name 'Daily report', got %q", job.Name)
	}
	if !job.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestAddCronJobDisabled(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-2", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	job, err := s.AddCronJob("default", "Disabled job", "0 0 * * *", "test-agent-2", "Do nothing", false)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}
	if job.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestGetCronJob(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-3", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	created, err := s.AddCronJob("default", "Test job", "*/5 * * * *", "test-agent-3", "Run every 5 min", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}

	got, err := s.GetCronJob(created.Id)
	if err != nil {
		t.Fatalf("GetCronJob: %v", err)
	}
	if got.Name != created.Name {
		t.Errorf("expected name %q, got %q", created.Name, got.Name)
	}
}

func TestGetCronJobNotFound(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, err := s.GetCronJob("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent cron job")
	}
}

func TestListCronJobs(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-4", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	s.AddCronJob("default", "Job 1", "0 9 * * *", "test-agent-4", "Morning report", true)
	s.AddCronJob("default", "Job 2", "0 17 * * *", "test-agent-4", "Evening report", true)
	s.AddCronJob("default", "Job 3", "0 0 * * 0", "test-agent-4", "Weekly review", false)

	// List only enabled for this agent
	jobs, err := s.ListCronJobs("default", "test-agent-4", false)
	if err != nil {
		t.Fatalf("ListCronJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 enabled jobs, got %d", len(jobs))
	}

	// List all including disabled
	allJobs, err := s.ListCronJobs("default", "test-agent-4", true)
	if err != nil {
		t.Fatalf("ListCronJobs include_disabled: %v", err)
	}
	if len(allJobs) != 3 {
		t.Errorf("expected 3 total jobs, got %d", len(allJobs))
	}

	// List all agents
	allAll, err := s.ListCronJobs("default", "", true)
	if err != nil {
		t.Fatalf("ListCronJobs all: %v", err)
	}
	if len(allAll) != 3 {
		t.Errorf("expected 3 jobs across all agents, got %d", len(allAll))
	}
}

func TestUpdateCronJob(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-5", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	created, err := s.AddCronJob("default", "Original", "0 9 * * *", "test-agent-5", "Original message", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}

	newName := "Updated"
	updated, err := s.UpdateCronJob("default", created.Id, &newName, nil, nil, nil)
	if err != nil {
		t.Fatalf("UpdateCronJob: %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", updated.Name)
	}

	disabled := false
	updated2, err := s.UpdateCronJob("default", created.Id, nil, nil, nil, &disabled)
	if err != nil {
		t.Fatalf("UpdateCronJob disable: %v", err)
	}
	if updated2.Enabled {
		t.Error("expected enabled=false after update")
	}

	newSchedule := "0 10 * * *"
	newMessage := "New message"
	updated3, err := s.UpdateCronJob("default", created.Id, nil, &newSchedule, &newMessage, nil)
	if err != nil {
		t.Fatalf("UpdateCronJob schedule+message: %v", err)
	}
	if updated3.Schedule != "0 10 * * *" {
		t.Errorf("expected schedule '0 10 * * *', got %q", updated3.Schedule)
	}
}

func TestUpdateCronJobNotFound(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	name := "test"
	_, err := s.UpdateCronJob("default", "nonexistent", &name, nil, nil, nil)
	if err == nil {
		t.Error("expected error for nonexistent cron job")
	}
}

func TestDeleteCronJob(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-6", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	created, err := s.AddCronJob("default", "To delete", "0 0 * * *", "test-agent-6", "Bye", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}

	err = s.DeleteCronJob("default", created.Id)
	if err != nil {
		t.Fatalf("DeleteCronJob: %v", err)
	}

	_, err = s.GetCronJob(created.Id)
	if err == nil {
		t.Error("expected error getting deleted cron job")
	}
}

func TestDeleteCronJobNotFound(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	err := s.DeleteCronJob("default", "nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent cron job")
	}
}

func TestGetDueCronJobs(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-7", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	job1, err := s.AddCronJob("default", "Due job", "0 9 * * *", "test-agent-7", "Overdue", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}
	s.UpdateCronJobFireTimestamps(job1.Id, 0, 1000)

	job2, err := s.AddCronJob("default", "Future job", "0 9 * * *", "test-agent-7", "Not yet", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}
	s.UpdateCronJobFireTimestamps(job2.Id, 0, 999999999999)

	job3, err := s.AddCronJob("default", "Disabled due job", "0 9 * * *", "test-agent-7", "Disabled", false)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}
	s.UpdateCronJobFireTimestamps(job3.Id, 0, 1000)

	due, err := s.GetDueCronJobs(2000)
	if err != nil {
		t.Fatalf("GetDueCronJobs: %v", err)
	}

	if len(due) != 1 {
		t.Fatalf("expected 1 due job, got %d", len(due))
	}
	if due[0].Id != job1.Id {
		t.Errorf("expected job %s, got %s", job1.Id, due[0].Id)
	}
}

func TestUpdateCronJobFireTimestamps(t *testing.T) {
	s := testStore(t)
	defer s.Close()

	_, _, err := s.EnsureAgent("test-agent-8", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	job, err := s.AddCronJob("default", "Timestamp test", "0 9 * * *", "test-agent-8", "Test", true)
	if err != nil {
		t.Fatalf("AddCronJob: %v", err)
	}

	err = s.UpdateCronJobFireTimestamps(job.Id, 1000, 2000)
	if err != nil {
		t.Fatalf("UpdateCronJobFireTimestamps: %v", err)
	}

	updated, err := s.GetCronJob(job.Id)
	if err != nil {
		t.Fatalf("GetCronJob: %v", err)
	}
	if updated.LastFireAt != 1000 {
		t.Errorf("expected last_fire_at=1000, got %d", updated.LastFireAt)
	}
	if updated.NextFireAt != 2000 {
		t.Errorf("expected next_fire_at=2000, got %d", updated.NextFireAt)
	}
}
