package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/aj-nt/mnemosyne/daemon/internal/messaging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MessagingService implements the Messaging gRPC service.
type MessagingService struct {
	pb.UnimplementedMessagingServer
	mu             sync.Mutex
	adapters       map[string]messaging.MessagingAdapter
	messageSubs    map[string]chan *pb.MessageEvent
	subscriptionID int64
}

// NewMessagingService creates a new MessagingService.
func NewMessagingService() *MessagingService {
	return &MessagingService{
		adapters:    make(map[string]messaging.MessagingAdapter),
		messageSubs: make(map[string]chan *pb.MessageEvent),
	}
}

// AddAdapter adds a messaging adapter for a specific platform.
func (s *MessagingService) AddAdapter(platform string, adapter messaging.MessagingAdapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapters[platform] = adapter
}

// SendMessage sends a message to a specific platform and target.
func (s *MessagingService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	if req.Platform == "" {
		return nil, status.Error(codes.InvalidArgument, "platform is required")
	}
	if req.Target == "" {
		return nil, status.Error(codes.InvalidArgument, "target is required")
	}
	if req.Message == "" {
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}

	s.mu.Lock()
	adapter, ok := s.adapters[req.Platform]
	s.mu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.Unimplemented, "platform %s is not supported", req.Platform)
	}

	messageID, err := adapter.Send(ctx, req.Target, req.Message)
	if err != nil {
		slog.Error("SendMessage failed", "error", err, "platform", req.Platform, "target", req.Target)
		return nil, status.Errorf(codes.Internal, "send message: %v", err)
	}

	return &pb.SendMessageResponse{
		Success:   true,
		MessageId: messageID,
	}, nil
}

// ListChannels lists available channels for a platform.
func (s *MessagingService) ListChannels(ctx context.Context, req *pb.ListChannelsRequest) (*pb.ListChannelsResponse, error) {
	if req.Platform == "" {
		return nil, status.Error(codes.InvalidArgument, "platform is required")
	}

	s.mu.Lock()
	adapter, ok := s.adapters[req.Platform]
	s.mu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.Unimplemented, "platform %s is not supported", req.Platform)
	}

	channels, err := adapter.ListChannels(ctx)
	if err != nil {
		slog.Error("ListChannels failed", "error", err, "platform", req.Platform)
		return nil, status.Errorf(codes.Internal, "list channels: %v", err)
	}

	return &pb.ListChannelsResponse{Channels: channels}, nil
}

// SubscribeToMessages subscribes to message events from all platforms.
func (s *MessagingService) SubscribeToMessages(req *pb.SubscribeToMessagesRequest, stream pb.Messaging_SubscribeToMessagesServer) error {
	ctx := stream.Context()

	// Create a subscription channel
	s.mu.Lock()
	s.subscriptionID++
	subID := fmt.Sprintf("sub-%d", s.subscriptionID)
	messageChan := make(chan *pb.MessageEvent, 100)
	s.messageSubs[subID] = messageChan
	s.mu.Unlock()

	// Cleanup on exit
	defer func() {
		s.mu.Lock()
		delete(s.messageSubs, subID)
		s.mu.Unlock()
	}()

	// Start all adapters and merge their message channels
	for platform, adapter := range s.adapters {
		adapterMsgChan, err := adapter.Listen(ctx)
		if err != nil {
			slog.Error("Failed to start adapter listener", "platform", platform, "error", err)
			continue
		}

		// Forward messages from this adapter to the subscription channel
		go func() {
			for {
				select {
				case msg := <-adapterMsgChan:
					select {
					case messageChan <- msg:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Forward messages to the gRPC stream
	for {
		select {
		case msg := <-messageChan:
			if err := stream.Send(msg); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}