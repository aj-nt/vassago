// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"
	"time"

	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Hot Block ---

func (s *Service) GetHotBlock(ctx context.Context, req *pb.HotBlockRequest) (*pb.HotBlockResponse, error) {
	if req.Target == "" {
		return nil, status.Error(codes.InvalidArgument, "target is required")
	}

	config := store.HotBlockConfig{
		CharLimit:     req.CharLimit,
		BoostPriority: 2, // Default
	}

	if req.Config != nil {
		config.BoostCategories = req.Config.BoostCategories
		config.SuppressCategories = req.Config.SuppressCategories
		if req.Config.BoostPriority > 0 {
			config.BoostPriority = req.Config.BoostPriority
		}
	}

	if config.CharLimit <= 0 {
		config.CharLimit = store.DefaultHotBlockCharLimit
	}

	// Fetch agent's last session info if agent_id provided
	var lastSessionID, lastSessionSummary string
	if req.AgentId != "" {
		agent, err := s.store.GetAgentByID(req.AgentId)
		if err == nil {
			lastSessionID = agent.LastSessionID
			lastSessionSummary = agent.LastSessionSummary
		}
	}

	result, err := s.store.ComputeHotBlockWithSession(ownerFromCtx(ctx), req.Target, config, lastSessionID, lastSessionSummary)
	if err != nil {
		slog.Error("GetHotBlock failed", "error", err, "target", req.Target)
		return nil, status.Errorf(codes.Internal, "compute hot block: %v", err)
	}

	return &pb.HotBlockResponse{
		Content:    result.Content,
		CharCount:  result.CharCount,
		EntryCount: result.EntryCount,
		ComputedAt: time.Now().Unix(),
	}, nil
}
