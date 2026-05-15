// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"

	"github.com/aj-nt/mnemosyne/daemon/internal/cron"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CronService implements the CronService gRPC service.
type CronService struct {
	pb.UnimplementedCronServiceServer
	store *store.Store
}

// NewCronService creates a new CronService with the given store.
func NewCronService(s *store.Store) *CronService {
	return &CronService{store: s}
}

// CreateCronJob creates a new cron job, scoped to owner (Phase 28).
func (s *CronService) CreateCronJob(ctx context.Context, req *pb.CreateCronJobRequest) (*pb.CronJob, error) {
	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	if req.Schedule == "" {
		return nil, status.Errorf(codes.InvalidArgument, "schedule is required")
	}
	if req.AgentId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "agent_id is required")
	}
	if req.Message == "" {
		return nil, status.Errorf(codes.InvalidArgument, "message is required")
	}

	if _, err := cron.NextFireTime(req.Schedule, 0); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid schedule: %v", err)
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	job, err := s.store.AddCronJob(ownerFromCtx(ctx), req.Name, req.Schedule, req.AgentId, req.Message, enabled)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create cron job: %v", err)
	}
	return job, nil
}

// ListCronJobs returns cron jobs, scoped to owner (Phase 28).
func (s *CronService) ListCronJobs(ctx context.Context, req *pb.ListCronJobsRequest) (*pb.CronJobList, error) {
	includeDisabled := req.IncludeDisabled
	jobs, err := s.store.ListCronJobs(ownerFromCtx(ctx), req.AgentId, includeDisabled)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list cron jobs: %v", err)
	}
	return &pb.CronJobList{Jobs: jobs}, nil
}

// UpdateCronJob updates specific fields of a cron job, scoped to owner (Phase 28).
func (s *CronService) UpdateCronJob(ctx context.Context, req *pb.UpdateCronJobRequest) (*pb.CronJob, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}

	if req.Schedule != nil {
		if _, err := cron.NextFireTime(*req.Schedule, 0); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid schedule: %v", err)
		}
	}

	job, err := s.store.UpdateCronJob(ownerFromCtx(ctx), req.Id, req.Name, req.Schedule, req.Message, req.Enabled)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "update cron job: %v", err)
	}
	return job, nil
}

// DeleteCronJob deletes a cron job, scoped to owner (Phase 28).
func (s *CronService) DeleteCronJob(ctx context.Context, req *pb.DeleteCronJobRequest) (*pb.Empty, error) {
	if req.Id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "id is required")
	}

	if err := s.store.DeleteCronJob(ownerFromCtx(ctx), req.Id); err != nil {
		return nil, status.Errorf(codes.NotFound, "delete cron job: %v", err)
	}
	return &pb.Empty{}, nil
}
