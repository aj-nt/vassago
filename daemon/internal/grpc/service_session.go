// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aj-nt/vassago/daemon/internal/store"
	pb "github.com/aj-nt/vassago/proto"
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

	// Trigger debounced memory extraction if configured
	if s.OnMessagesAdded != nil {
		go s.OnMessagesAdded(req.SessionId, ownerFromCtx(ctx))
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

	// Get session before ending — we need the agent_id
	sess, err := s.store.GetSession(req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "session not found: %s", req.SessionId)
	}

	if err := s.store.EndSession(req.SessionId); err != nil {
		slog.Error("EndSession failed", "error", err, "session_id", req.SessionId)
		return nil, status.Errorf(codes.Internal, "end session: %v", err)
	}

	// Generate summary and update agent
	summary := generateSessionSummary(s.store, s.summaryClient, sess)
	if err := s.store.SetAgentLastSession(sess.AgentID, req.SessionId, summary); err != nil {
		slog.Error("EndSession: failed to update agent last session", "error", err)
		// Non-fatal — session is already ended
	}

	return &pb.EndSessionResponse{Ended: true}, nil
}

// generateSessionSummary produces a one-line summary of a session.
// If an LLM client is available and the session has enough messages,
// it uses AI summarization. Otherwise falls back to metadata.
func generateSessionSummary(st *store.Store, client LLMClient, sess *store.Session) string {
	// Fallback: metadata summary
	metaSummary := fmt.Sprintf("%d messages", sess.MessageCount)
	if sess.CreatedAt > 0 && sess.LastActiveAt > sess.CreatedAt {
		dur := time.Duration(sess.LastActiveAt-sess.CreatedAt) * time.Second
		if dur >= time.Minute {
			metaSummary = fmt.Sprintf("%d messages, %s", sess.MessageCount,
				dur.Truncate(time.Minute).String())
		}
	}

	if client == nil || sess.MessageCount < 2 {
		return metaSummary
	}

	msgs, err := st.GetSessionMessages(sess.ID)
	if err != nil || len(msgs) == 0 {
		return metaSummary
	}

	// Build prompt from first few messages
	var sb strings.Builder
	sb.WriteString("Summarize this AI agent session in ONE short line (max 100 chars). What was the main topic or accomplishment?\n\n")
	count := 0
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			content := m.Content
			if len(content) > 200 {
				content = content[:200]
			}
			sb.WriteString(fmt.Sprintf("[%s] %s\n", m.Role, content))
			count++
			if count >= 10 {
				break
			}
		}
	}

	summary, err := client.Complete(sb.String())
	if err != nil {
		slog.Debug("EndSession: LLM summary failed, using fallback", "error", err)
		return metaSummary
	}

	summary = strings.TrimSpace(summary)
	if len(summary) > 200 {
		summary = summary[:200]
	}
	if summary == "" {
		return metaSummary
	}
	return summary
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
					Role:       m.Role,
					Content:    m.Content,
					ToolCallId: m.ToolCallID,
					ToolName:   m.ToolName,
					Timestamp:  m.Timestamp,
				}
			}
		}
	}

	return pbSession, nil
}
