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
// It manages platform adapters and broadcasts incoming messages to all
// subscribers via fan-out from a single adapter read goroutine per platform.
type MessagingService struct {
	pb.UnimplementedMessagingServer
	mu              sync.Mutex
	adapters        map[string]messaging.MessagingAdapter
	adaptersStarted map[string]bool // tracks which adapters have been connected
	subscribers     map[string]chan *pb.MessageEvent // all active subscriber channels
	subscriptionID  int64
}

// NewMessagingService creates a new MessagingService.
func NewMessagingService() *MessagingService {
	return &MessagingService{
		adapters:        make(map[string]messaging.MessagingAdapter),
		adaptersStarted: make(map[string]bool),
		subscribers:     make(map[string]chan *pb.MessageEvent),
	}
}

// AddAdapter adds a messaging adapter for a specific platform.
// It verifies that the platform key matches the adapter's PlatformName().
func (s *MessagingService) AddAdapter(platform string, adapter messaging.MessagingAdapter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify platform key matches adapter's self-reported name
	if adapterName := adapter.PlatformName(); adapterName != platform {
		slog.Warn("Platform key mismatch",
			"key", platform,
			"adapter_platform", adapterName,
			"msg", "using adapter key as-is")
	}

	s.adapters[platform] = adapter
}

// Status returns the connection status of all adapters.
func (s *MessagingService) Status() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := make(map[string]bool, len(s.adaptersStarted))
	for platform := range s.adapters {
		status[platform] = s.adaptersStarted[platform]
	}
	return status
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
// It ensures adapters are started, creates a subscriber channel, and
// broadcasts all incoming messages to this subscriber via fan-out.
func (s *MessagingService) SubscribeToMessages(req *pb.SubscribeToMessagesRequest, stream pb.Messaging_SubscribeToMessagesServer) error {
	ctx := stream.Context()

	// Start adapters if not already started (idempotent)
	s.startAdapters(ctx)

	// Create subscriber channel
	s.mu.Lock()
	s.subscriptionID++
	subID := fmt.Sprintf("sub-%d", s.subscriptionID)
	subChan := make(chan *pb.MessageEvent, 100)
	s.subscribers[subID] = subChan
	s.mu.Unlock()

	// Cleanup on exit
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, subID)
		s.mu.Unlock()
	}()

	// Forward messages to the gRPC stream
	for {
		select {
		case msg := <-subChan:
			if err := stream.Send(msg); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// startAdapters connects all adapters and starts a single read goroutine
// per platform that fans out messages to all subscribers.
// This is idempotent — each adapter is started exactly once.
func (s *MessagingService) startAdapters(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for platform, adapter := range s.adapters {
		if s.adaptersStarted[platform] {
			continue
		}

		// Connect the adapter (idempotent)
		if err := adapter.Connect(ctx); err != nil {
			slog.Error("Failed to connect adapter", "platform", platform, "error", err)
			continue
		}

		// Start listening for messages
		ch, err := adapter.Listen(ctx)
		if err != nil {
			slog.Error("Failed to start adapter listener", "platform", platform, "error", err)
			continue
		}

		// Start a single goroutine that reads from this adapter's channel
		// and fans out to ALL current and future subscribers.
		go s.fanOutMessages(ctx, platform, ch)

		s.adaptersStarted[platform] = true
		slog.Info("Messaging adapter started", "platform", platform)
	}
}

// fanOutMessages reads messages from an adapter channel and broadcasts
// them to all active subscriber channels.
func (s *MessagingService) fanOutMessages(ctx context.Context, platform string, ch <-chan *pb.MessageEvent) {
	for {
		select {
		case msg := <-ch:
			s.mu.Lock()
			for _, subChan := range s.subscribers {
				select {
				case subChan <- msg:
				default:
					// Subscriber channel full — drop the message rather than block
					slog.Warn("Subscriber channel full, dropping message",
						"platform", platform,
						"channel_id", msg.ChannelId,
						"sender", msg.SenderName)
				}
			}
			s.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}