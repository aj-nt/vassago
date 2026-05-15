// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ping checks the health of the store and returns a health status.
func (s *Service) Ping(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	// Check database connectivity
	if err := s.store.Ping(); err != nil {
		slog.Error("Health check failed", "error", err)
		return nil, status.Errorf(codes.Internal, "health check failed: %v", err)
	}

	return &pb.HealthCheckResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		Timestamp: time.Now().Unix(),
	}, nil
}
