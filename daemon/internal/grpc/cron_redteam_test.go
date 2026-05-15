// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/vassago/proto"
)

// Red Team: Cron gRPC validation edge cases

func TestCronService_RedTeam_EmptyRequiredFields(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	tests := []struct {
		name string
		req  *pb.CreateCronJobRequest
	}{
		{
			name: "empty name",
			req:  &pb.CreateCronJobRequest{Schedule: "0 9 * * *", AgentId: "test-agent", Message: "msg"},
		},
		{
			name: "empty schedule",
			req:  &pb.CreateCronJobRequest{Name: "test", AgentId: "test-agent", Message: "msg"},
		},
		{
			name: "empty agent_id",
			req:  &pb.CreateCronJobRequest{Name: "test", Schedule: "0 9 * * *", Message: "msg"},
		},
		{
			name: "empty message",
			req:  &pb.CreateCronJobRequest{Name: "test", Schedule: "0 9 * * *", AgentId: "test-agent"},
		},
		{
			name: "all empty",
			req:  &pb.CreateCronJobRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCronJob(context.Background(), tt.req)
			if err == nil {
				t.Error("expected error for invalid request")
			}
		})
	}
}

func TestCronService_RedTeam_InvalidCronExpressions(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	tests := []struct {
		name     string
		schedule string
	}{
		{"too few fields", "0 9 *"},
		{"too many fields", "0 9 * * * *"},
		{"minute out of range", "60 9 * * *"},
		{"hour out of range", "0 25 * * *"},
		{"day zero", "0 9 0 * *"},
		{"day out of range", "0 9 32 * *"},
		{"month out of range", "0 9 * 13 *"},
		{"weekday out of range", "0 9 * * 8"},
		{"empty schedule", ""},
		{"random text", "not a schedule"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
				Name:     "test",
				Schedule: tt.schedule,
				AgentId:  "test-agent",
				Message:  "test message",
			})
			if err == nil {
				t.Errorf("expected error for invalid schedule %q", tt.schedule)
			}
		})
	}
}

func TestCronService_RedTeam_DeleteEmptyID(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	_, err := svc.DeleteCronJob(context.Background(), &pb.DeleteCronJobRequest{Id: ""})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestCronService_RedTeam_UpdateEmptyID(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	name := "test"
	_, err := svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id:   "",
		Name: &name,
	})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestCronService_RedTeam_UpdateInvalidSchedule(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	// First create a valid job
	created, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
		Name: "Valid job", Schedule: "0 9 * * *", AgentId: "test-agent", Message: "test",
	})
	if err != nil {
		t.Fatalf("CreateCronJob: %v", err)
	}

	// Try to update with invalid schedule
	badSchedule := "invalid"
	_, err = svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id:       created.Id,
		Schedule: &badSchedule,
	})
	if err == nil {
		t.Error("expected error for invalid schedule in update")
	}
}

func TestCronService_RedTeam_UpdateNonexistent(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	name := "test"
	_, err := svc.UpdateCronJob(context.Background(), &pb.UpdateCronJobRequest{
		Id:   "nonexistent-id",
		Name: &name,
	})
	if err == nil {
		t.Error("expected error for nonexistent job update")
	}
}

func TestCronService_RedTeam_DeleteNonexistent(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	_, err := svc.DeleteCronJob(context.Background(), &pb.DeleteCronJobRequest{Id: "nonexistent-id"})
	if err == nil {
		t.Error("expected error for nonexistent job delete")
	}
}

func TestCronService_RedTeam_ListEmpty(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	resp, err := svc.ListCronJobs(context.Background(), &pb.ListCronJobsRequest{
		AgentId: "test-agent",
	})
	if err != nil {
		t.Fatalf("ListCronJobs: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected empty list, got %d jobs", len(resp.Jobs))
	}
}

func TestCronService_RedTeam_ScheduleBoundaries(t *testing.T) {
	svc, _, cleanup := testCronService(t)
	defer cleanup()

	tests := []struct {
		name     string
		schedule string
		valid    bool
	}{
		{"minute boundary 0", "0 * * * *", true},
		{"minute boundary 59", "59 * * * *", true},
		{"hour boundary 0", "* 0 * * *", true},
		{"hour boundary 23", "* 23 * * *", true},
		{"day boundary 1", "* * 1 * *", true},
		{"day boundary 31", "* * 31 * *", true},
		{"month boundary 1", "* * * 1 *", true},
		{"month boundary 12", "* * * 12 *", true},
		{"weekday boundary 0 (sunday)", "* * * * 0", true},
		{"weekday boundary 6 (saturday)", "* * * * 6", true},
		{"step expression", "*/5 * * * *", true},
		{"range expression", "0 9-17 * * *", true},
		{"list expression", "0,30 * * * *", true},
		{"combined", "0,30 9-17 * * 1-5", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCronJob(context.Background(), &pb.CreateCronJobRequest{
				Name:     tt.name,
				Schedule: tt.schedule,
				AgentId:  "test-agent",
				Message:  "test",
			})
			if tt.valid && err != nil {
				t.Errorf("expected schedule %q to be valid, got error: %v", tt.schedule, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected schedule %q to be invalid, got no error", tt.schedule)
			}
		})
	}
}
