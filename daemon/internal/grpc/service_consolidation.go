// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Consolidation ---

func (s *Service) Consolidate(ctx context.Context, req *pb.ConsolidateRequest) (*pb.ConsolidateResponse, error) {
	if err := s.checkDB(); err != nil {
		return nil, err
	}
	var batchSize int
	if req.BatchSize != nil {
		batchSize = int(*req.BatchSize)
	}

	scope := protoToConsolidateScope(req.Scope)
	var category string
	if req.Category != nil {
		category = *req.Category
	}
	result, err := s.store.Consolidate(ownerFromCtx(ctx), scope, category, batchSize)
	if err != nil {
		slog.Error("Consolidate failed", "error", err, "scope", req.Scope)
		return nil, status.Errorf(codes.Internal, "consolidate: %v", err)
	}

	return &pb.ConsolidateResponse{
		Removed: int32(result.Removed),
		Merged:  int32(result.Merged),
		Summary: result.Summary,
	}, nil
}

func (s *Service) GetConsolidationCandidates(ctx context.Context, req *pb.GetConsolidationCandidatesRequest) (*pb.ConsolidationCandidatesResponse, error) {
	var batchSize int
	if req.BatchSize != nil {
		batchSize = int(*req.BatchSize)
	}

	scope := protoToConsolidateScope(req.Scope)
	var category string
	if req.Category != nil {
		category = *req.Category
	}
	candidates, err := s.store.GetConsolidationCandidates(ownerFromCtx(ctx), scope, category, batchSize)
	if err != nil {
		slog.Error("GetConsolidationCandidates failed", "error", err, "scope", req.Scope)
		return nil, status.Errorf(codes.Internal, "consolidation candidates: %v", err)
	}

	pbCandidates := make([]*pb.ConsolidationCandidate, len(candidates))
	for i, c := range candidates {
		pbCandidates[i] = &pb.ConsolidationCandidate{
			Entry:  memoryEntryToProto(c.Entry),
			Reason: c.Reason,
		}
	}

	return &pb.ConsolidationCandidatesResponse{Candidates: pbCandidates}, nil
}
