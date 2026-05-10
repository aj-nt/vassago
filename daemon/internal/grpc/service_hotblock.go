// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
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
		config.CharLimit = 4000 // Default from DESIGN.md
	}

	result, err := s.store.ComputeHotBlock(ownerFromCtx(ctx), req.Target, config)
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