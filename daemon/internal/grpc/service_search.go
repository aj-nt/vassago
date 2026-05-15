// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"

	pb "github.com/aj-nt/vassago/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Search ---

func (s *Service) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	var limit int
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	entries, err := s.store.Search(ownerFromCtx(ctx), req.Query, req.Target, limit)
	if err != nil {
		slog.Error("Search failed", "error", err, "query", req.Query)
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}

	pbEntries := make([]*pb.MemoryEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = memoryEntryToProto(e)
	}

	return &pb.SearchResponse{Entries: pbEntries}, nil
}
