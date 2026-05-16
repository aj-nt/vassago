// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"

	pb "github.com/aj-nt/vassago-sdk/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RegisterAgent registers an agent with the daemon.
func (s *Service) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	agentID := req.AgentId
	if agentID == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	_, isNew, err := s.store.EnsureAgent(agentID, req.Name, req.Role)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ensure agent: %v", err)
	}

	slog.Info("agent registered", "agent_id", agentID, "name", req.Name, "role", req.Role, "is_new", isNew)

	return &pb.RegisterAgentResponse{
		AgentId: agentID,
		IsNew:   isNew,
	}, nil
}

// Heartbeat receives a keep-alive from an agent.
func (s *Service) Heartbeat(ctx context.Context, req *pb.AgentId) (*pb.Empty, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Just log it — the real disconnect detection is via the Subscribe stream
	slog.Debug("heartbeat received", "agent_id", req.AgentId)
	return &pb.Empty{}, nil
}
