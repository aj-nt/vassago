// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"log/slog"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Session Lifecycle ---

func (s *Service) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.Session, error) {
	if err := s.checkDB(); err != nil {
		return nil, err
	}
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	sess, err := s.store.CreateSession(req.AgentId, req.Source, req.Title)
	if err != nil {
		slog.Error("CreateSession failed", "error", err, "agent_id", req.AgentId)
		return nil, status.Errorf(codes.Internal, "create session: %v", err)
	}

	return sessionToProto(sess), nil
}

func (s *Service) AddMessages(ctx context.Context, req *pb.AddMessagesRequest) (*pb.Session, error) {
	if err := s.checkDB(); err != nil {
		return nil, err
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if len(req.Messages) == 0 {
		return nil, status.Error(codes.InvalidArgument, "messages must not be empty")
	}

	msgs := make([]store.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = store.Message{
			SessionID:  req.SessionId,
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallId,
			ToolName:   m.ToolName,
			Timestamp:  m.Timestamp,
		}
	}

	if err := s.store.AddMessages(req.SessionId, msgs); err != nil {
		slog.Error("AddMessages failed", "error", err, "session_id", req.SessionId)
		return nil, status.Errorf(codes.Internal, "add messages: %v", err)
	}

	sess, err := s.store.GetSession(req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get session after add messages: %v", err)
	}

	return sessionToProto(sess), nil
}

func (s *Service) EndSession(ctx context.Context, req *pb.EndSessionRequest) (*pb.EndSessionResponse, error) {
	if err := s.checkDB(); err != nil {
		return nil, err
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	if err := s.store.EndSession(req.SessionId); err != nil {
		slog.Error("EndSession failed", "error", err, "session_id", req.SessionId)
		return nil, status.Errorf(codes.Internal, "end session: %v", err)
	}

	return &pb.EndSessionResponse{Ended: true}, nil
}

func (s *Service) GetSession(ctx context.Context, req *pb.GetSessionRequest) (*pb.Session, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	sess, err := s.store.GetSession(req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "session not found: %s", req.SessionId)
	}

	pbSession := sessionToProto(sess)

	// Include messages if requested
	if req.IncludeMessages != nil && *req.IncludeMessages {
		msgs, err := s.store.GetSessionMessages(req.SessionId)
		if err != nil {
			slog.Error("GetSessionMessages failed", "error", err, "session_id", req.SessionId)
		} else {
			pbSession.Messages = make([]*pb.Message, len(msgs))
			for i, m := range msgs {
				pbSession.Messages[i] = &pb.Message{
					Role:      m.Role,
					Content:   m.Content,
					ToolCallId: m.ToolCallID,
					ToolName:  m.ToolName,
					Timestamp: m.Timestamp,
				}
			}
		}
	}

	return pbSession, nil
}