// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"

	pb "github.com/aj-nt/vassago/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SearchSessions searches session titles matching the query.
func (s *Service) SearchSessions(ctx context.Context, req *pb.SearchSessionsRequest) (*pb.SearchSessionsResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	var limit int
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	sessions, err := s.store.SearchSessions(req.Query, limit)
	if err != nil {
		slog.Error("SearchSessions failed", "error", err, "query", req.Query)
		return nil, status.Errorf(codes.Internal, "search sessions: %v", err)
	}

	pbSessions := make([]*pb.Session, len(sessions))
	for i, sess := range sessions {
		pbSessions[i] = sessionToProto(sess)
	}

	return &pb.SearchSessionsResponse{Sessions: pbSessions}, nil
}

// ListSessions returns the most recently active sessions.
func (s *Service) ListSessions(ctx context.Context, req *pb.ListSessionsRequest) (*pb.ListSessionsResponse, error) {
	var limit int
	if req.Limit != nil {
		limit = int(*req.Limit)
	}

	sessions, err := s.store.ListRecentSessions(limit)
	if err != nil {
		slog.Error("ListSessions failed", "error", err)
		return nil, status.Errorf(codes.Internal, "list sessions: %v", err)
	}

	pbSessions := make([]*pb.Session, len(sessions))
	for i, sess := range sessions {
		pbSessions[i] = sessionToProto(sess)
	}

	return &pb.ListSessionsResponse{Sessions: pbSessions}, nil
}
