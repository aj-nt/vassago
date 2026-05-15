// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	"github.com/aj-nt/vassago/daemon/internal/cron"
	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago/proto"
)

func testCronService(t *testing.T) (*CronService, *store.Store, func()) {
	s, cleanup := newTestService(t)
	// Create an agent for foreign key constraints
	_, _, err := s.store.EnsureAgent("test-agent", "Test Agent", "dev")
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}
	return NewCronService(s.store), s.store, func() { cleanup() }
}

func TestCronService_CreateCronJob(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	job, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name:     "Daily report",
		Schedule: "0 9 * * *",
		AgentId:  "test-agent",
		Message:  "Generate daily report",
	})
	if err != nil {
		t.Fatalf("CreateCronJob: %v", err)
	}
	if job.Id == "" {
		t.Error("expected non-empty job ID")
	}
	if job.Name != "Daily report" {
		t.Errorf("expected name 'Daily report', got %q", job.Name)
	}
	if !job.Enabled {
		t.Error("expected enabled=true by default")
	}
}

func TestCronService_CreateCronJobDisabled(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	disabled := false
	job, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name:     "Disabled job",
		Schedule: "0 0 * * *",
		AgentId:  "test-agent",
		Message:  "Do nothing",
		Enabled:  &disabled,
	})
	if err != nil {
		t.Fatalf("CreateCronJob: %v", err)
	}
	if job.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestCronService_CreateCronJobValidation(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	tests := []struct {
		name    string
		req     *pb.CreateCronJobRequest
		wantErr string
	}{
		{
			name:    "empty name",
			req:     &pb.CreateCronJobRequest{Schedule: "0 9 * * *", AgentId: "test-agent", Message: "msg"},
			wantErr: "name is required",
		},
		{
			name:    "empty schedule",
			req:     &pb.CreateCronJobRequest{Name: "test", AgentId: "test-agent", Message: "msg"},
			wantErr: "schedule is required",
		},
		{
			name:    "empty agent_id",
			req:     &pb.CreateCronJobRequest{Name: "test", Schedule: "0 9 * * *", Message: "msg"},
			wantErr: "agent_id is required",
		},
		{
			name:    "empty message",
			req:     &pb.CreateCronJobRequest{Name: "test", Schedule: "0 9 * * *", AgentId: "test-agent"},
			wantErr: "message is required",
		},
		{
			name:    "invalid schedule - too few fields",
			req:     &pb.CreateCronJobRequest{Name: "test", Schedule: "0 9 *", AgentId: "test-agent", Message: "msg"},
			wantErr: "expected 5 fields",
		},
		{
			name:    "invalid schedule - out of range",
			req:     &pb.CreateCronJobRequest{Name: "test", Schedule: "60 9 * * *", AgentId: "test-agent", Message: "msg"},
			wantErr: "minute value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCronJob(context.Background(), tt.req)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			}
		})
	}
}

func TestCronService_ListCronJobs(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	// Create two jobs
	svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name: "Job 1", Schedule: "0 9 * * *", AgentId: "test-agent", Message: "Morning",
	})
	disabled := false
	svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name: "Job 2", Schedule: "0 17 * * *", AgentId: "test-agent", Message: "Evening", Enabled: &disabled,
	})

	// List only enabled
	resp, err := svc.ListCronJobs(context.Background(), &pb.ListCronJobsRequest{
		AgentId: "test-agent",
	})
	if err != nil {
		t.Fatalf("ListCronJobs: %v", err)
	}
	if len(resp.Jobs) != 1 {
		t.Errorf("expected 1 enabled job, got %d", len(resp.Jobs))
	}

	// List all including disabled
	respAll, err := svc.ListCronJobs(context.Background(), &pb.ListCronJobsRequest{
		AgentId:         "test-agent",
		IncludeDisabled: true,
	})
	if err != nil {
		t.Fatalf("ListCronJobs include_disabled: %v", err)
	}
	if len(respAll.Jobs) != 2 {
		t.Errorf("expected 2 total jobs, got %d", len(respAll.Jobs))
	}
}

func TestCronService_UpdateCronJob(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	created, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name: "Original", Schedule: "0 9 * * *", AgentId: "test-agent", Message: "Original msg",
	})
	if err != nil {
		t.Fatalf("CreateCronJob: %v", err)
	}

	// Update name
	newName := "Updated"
	updated, err := svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id:   created.Id,
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("UpdateCronJob: %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", updated.Name)
	}

	// Disable
	disabled := false
	updated2, err := svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id:      created.Id,
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("UpdateCronJob disable: %v", err)
	}
	if updated2.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestCronService_UpdateCronJobValidation(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	// Empty ID
	_, err := svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id: "",
	})
	if err == nil {
		t.Error("expected error for empty ID")
	}

	// Invalid schedule
	badSchedule := "invalid"
	_, err = svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id:       "some-id",
		Schedule: &badSchedule,
	})
	if err == nil {
		t.Error("expected error for invalid schedule")
	}
}

func TestCronService_DeleteCronJob(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	created, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name: "To delete", Schedule: "0 0 * * *", AgentId: "test-agent", Message: "Bye",
	})
	if err != nil {
		t.Fatalf("CreateCronJob: %v", err)
	}

	_, err = svc.DeleteCronJob(context.Background(), &pb.DeleteCronJobRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteCronJob: %v", err)
	}

	// List should show no enabled jobs
	resp, err := svc.ListCronJobs(context.Background(), &pb.ListCronJobsRequest{
		AgentId: "test-agent",
	})
	if err != nil {
		t.Fatalf("ListCronJobs: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs after delete, got %d", len(resp.Jobs))
	}
}

func TestCronService_DeleteCronJobNotFound(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	_, err := svc.DeleteCronJob(context.Background(), &pb.DeleteCronJobRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent cron job")
	}
}

func TestCronService_DeleteCronJobEmptyID(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	_, err := svc.DeleteCronJob(context.Background(), &pb.DeleteCronJobRequest{Id: ""})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestValidateCronExpressionViaNextFireTime(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"0 9 * * *", false},    // every day at 9:00
		{"*/5 * * * *", false},  // every 5 minutes
		{"0 0 1 1 *", false},    // yearly on Jan 1
		{"0 9 * * 1-5", false},  // weekdays at 9:00
		{"0,30 * * * *", false}, // every hour at :00 and :30
		{"0 9 * *", true},       // only 4 fields
		{"0 9 * * * *", true},   // 6 fields
		{"60 9 * * *", true},    // minute out of range
		{"0 25 * * *", true},    // hour out of range
		{"0 9 0 * *", true},     // day out of range (0)
		{"0 9 * 13 *", true},    // month out of range (13)
		{"0 9 * * 7", true},     // weekday out of range (7)
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := cron.NextFireTime(tt.expr, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("NextFireTime(%q) = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}
