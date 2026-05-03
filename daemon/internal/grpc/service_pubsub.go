package grpc

import (
	"fmt"
	"log/slog"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/pubsub"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Subscribe implements the server-streaming Subscribe RPC.
// Agent subscribes to memory change events. The stream stays open
// until the client disconnects or cancels.
func (s *Service) Subscribe(req *pb.SubscribeRequest, stream pb.Mnemosyne_SubscribeServer) error {
	agentID := req.AgentId
	if agentID == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Build subscription
	targets := make(map[string]bool)
	for _, t := range req.Targets {
		targets[t] = true
	}
	categories := make(map[string]bool)
	for _, c := range req.Categories {
		categories[c] = true
	}

	subID := fmt.Sprintf("%s-%d", agentID, time.Now().UnixNano())
	sub := &pubsub.Subscription{
		ID:         subID,
		AgentID:    agentID,
		Targets:    targets,
		Categories: categories,
		KeyPrefixes: req.KeyPrefixes,
		Ch:         make(chan pubsub.Event, 100),
	}

	s.hub.Subscribe(sub)
	defer s.hub.Unsubscribe(subID)

	slog.Info("agent subscribed", "agent_id", agentID, "sub_id", subID)

	// Stream events until client disconnects
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			slog.Info("agent disconnected", "agent_id", agentID, "sub_id", subID)
			return ctx.Err()
		case event := <-sub.Ch:
			updateEvent := &pb.UpdateEvent{
				EventType: event.Type,
				Target:    event.Target,
				Category:  event.Category,
				Key:       event.Key,
				AgentId:   event.AgentID,
			}
			if event.Entry != nil {
				updateEvent.Entry = event.Entry
			}
			if event.CronFire != nil {
				updateEvent.CronFire = event.CronFire
			}
			if err := stream.Send(updateEvent); err != nil {
				slog.Error("failed to send update", "agent_id", agentID, "error", err)
				return err
			}
		}
	}
}