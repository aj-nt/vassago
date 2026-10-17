// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SyncChanges returns memory changes since a given timestamp for a namespace.
// Peer daemons call this to pull deltas during replication.
func (s *Service) SyncChanges(ctx context.Context, req *pb.SyncChangesRequest) (*pb.SyncChangesResponse, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	entries, err := s.store.GetChangesSince(req.Namespace, req.Since)
	if err != nil {
		slog.Error("SyncChanges: query failed", "error", err, "namespace", req.Namespace, "since", req.Since)
		return nil, status.Errorf(codes.Internal, "get changes: %v", err)
	}

	pbEntries := memoryEntriesToProto(entries)

	slog.Debug("SyncChanges: served delta", "namespace", req.Namespace, "count", len(pbEntries), "peer", req.PeerId)

	return &pb.SyncChangesResponse{
		Entries:    pbEntries,
		ServerTime: time.Now().Unix(),
	}, nil
}
