package grpc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockAdapter implements MessagingAdapter for testing
type mockAdapter struct {
	sendFunc      func(ctx context.Context, target, message string) (string, error)
	listFunc      func(ctx context.Context) ([]*pb.Channel, error)
	listenFunc    func(ctx context.Context) (<-chan *pb.MessageEvent, error)
	closeFunc     func(ctx context.Context) error
	connectFunc   func(ctx context.Context) error
}

func (m *mockAdapter) Send(ctx context.Context, target, message string) (string, error) {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, target, message)
	}
	return "msg1", nil
}

func (m *mockAdapter) ListChannels(ctx context.Context) ([]*pb.Channel, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []*pb.Channel{
		{Id: "channel1", Name: "test-channel", Platform: "mock"},
	}, nil
}

func (m *mockAdapter) Listen(ctx context.Context) (<-chan *pb.MessageEvent, error) {
	if m.listenFunc != nil {
		return m.listenFunc(ctx)
	}
	ch := make(chan *pb.MessageEvent, 1)
	return ch, nil
}

func (m *mockAdapter) Close(ctx context.Context) error {
	if m.closeFunc != nil {
		return m.closeFunc(ctx)
	}
	return nil
}

func (m *mockAdapter) Connect(ctx context.Context) error {
	if m.connectFunc != nil {
		return m.connectFunc(ctx)
	}
	return nil
}

// --- QA Tests ---

func TestMessagingService_SendMessage_Success(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			if target != "channel1" {
				t.Errorf("expected target channel1, got %s", target)
			}
			if message != "test message" {
				t.Errorf("expected message 'test message', got %s", message)
			}
			return "msg1", nil
		},
	}
	svc.AddAdapter("mock", mock)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "channel1",
		Message:  "test message",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected success = true")
	}
	if resp.MessageId != "msg1" {
		t.Errorf("expected message ID msg1, got %s", resp.MessageId)
	}
}

func TestMessagingService_SendMessage_Validation(t *testing.T) {
	svc := NewMessagingService()
	tests := []struct {
		name     string
		request  *pb.SendMessageRequest
		expected codes.Code
	}{
		{
			name:     "empty platform",
			request:  &pb.SendMessageRequest{Target: "channel1", Message: "test"},
			expected: codes.InvalidArgument,
		},
		{
			name:     "empty target",
			request:  &pb.SendMessageRequest{Platform: "mock", Message: "test"},
			expected: codes.InvalidArgument,
		},
		{
			name:     "empty message",
			request:  &pb.SendMessageRequest{Platform: "mock", Target: "channel1"},
			expected: codes.InvalidArgument,
		},
		{
			name:     "unsupported platform",
			request:  &pb.SendMessageRequest{Platform: "unsupported", Target: "channel1", Message: "test"},
			expected: codes.Unimplemented,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SendMessage(context.Background(), tt.request)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Errorf("expected gRPC status error, got: %v", err)
				return
			}

			if st.Code() != tt.expected {
				t.Errorf("expected code %s, got %s", tt.expected, st.Code())
			}
		})
	}
}

func TestMessagingService_ListChannels_Success(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		listFunc: func(ctx context.Context) ([]*pb.Channel, error) {
			return []*pb.Channel{
				{Id: "channel1", Name: "test-channel", Platform: "mock"},
				{Id: "channel2", Name: "another-channel", Platform: "mock"},
			}, nil
		},
	}
	svc.AddAdapter("mock", mock)

	req := &pb.ListChannelsRequest{Platform: "mock"}
	resp, err := svc.ListChannels(context.Background(), req)
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}
	if len(resp.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(resp.Channels))
	}
}

func TestMessagingService_ListChannels_Validation(t *testing.T) {
	svc := NewMessagingService()

	_, err := svc.ListChannels(context.Background(), &pb.ListChannelsRequest{})
	if err == nil {
		t.Error("expected error for empty platform, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code InvalidArgument, got %s", st.Code())
	}
}

func TestMessagingService_ListChannels_UnsupportedPlatform(t *testing.T) {
	svc := NewMessagingService()

	req := &pb.ListChannelsRequest{Platform: "unsupported"}
	_, err := svc.ListChannels(context.Background(), req)
	if err == nil {
		t.Error("expected error for unsupported platform, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.Unimplemented {
		t.Errorf("expected code Unimplemented, got %s", st.Code())
	}
}

// --- Red Team Tests ---

func TestRedTeam_MessagingService_SendMessage_AdapterError(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			return "", errors.New("adapter connection failed")
		},
	}
	svc.AddAdapter("mock", mock)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "channel1",
		Message:  "test message",
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Error("expected error when adapter fails, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.Internal {
		t.Errorf("expected code Internal, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_ListChannels_AdapterError(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		listFunc: func(ctx context.Context) ([]*pb.Channel, error) {
			return nil, errors.New("adapter connection failed")
		},
	}
	svc.AddAdapter("mock", mock)

	req := &pb.ListChannelsRequest{Platform: "mock"}
	_, err := svc.ListChannels(context.Background(), req)
	if err == nil {
		t.Error("expected error when adapter fails, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.Internal {
		t.Errorf("expected code Internal, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_SendMessage_LongPlatformName(t *testing.T) {
	svc := NewMessagingService()
	longPlatform := string(make([]byte, 1000)) // Very long platform name

	req := &pb.SendMessageRequest{
		Platform: longPlatform,
		Target:   "channel1",
		Message:  "test message",
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Error("expected error for very long platform name, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.Unimplemented {
		t.Errorf("expected code Unimplemented, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_SendMessage_UnicodeInMessage(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			// Verify unicode is preserved
			if message != "Hello 🌍! 你好! 🧠" {
				t.Errorf("expected unicode message, got %s", message)
			}
			return "msg1", nil
		},
	}
	svc.AddAdapter("mock", mock)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "channel1",
		Message:  "Hello 🌍! 你好! 🧠",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage with unicode failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected success = true for unicode message")
	}
}

func TestRedTeam_MessagingService_SendMessage_NullBytesInMessage(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			// Verify null bytes are handled
			if !strings.Contains(message, "\x00") {
				t.Errorf("expected null bytes in message")
			}
			return "msg1", nil
		},
	}
	svc.AddAdapter("mock", mock)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "channel1",
		Message:  "test\x00message\x00with\x00nulls",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage with null bytes failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected success = true for message with null bytes")
	}
}

func TestRedTeam_MessagingService_ConcurrentSendMessage(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			time.Sleep(10 * time.Millisecond) // Simulate some processing time
			return "msg1", nil
		},
	}
	svc.AddAdapter("mock", mock)

	// Send multiple concurrent requests
	requests := make([]*pb.SendMessageRequest, 10)
	for i := range requests {
		requests[i] = &pb.SendMessageRequest{
			Platform: "mock",
			Target:   "channel1",
			Message:  "concurrent message",
		}
	}

	errChan := make(chan error, len(requests))
	for _, req := range requests {
		go func(r *pb.SendMessageRequest) {
			_, err := svc.SendMessage(context.Background(), r)
			errChan <- err
		}(req)
	}

	// Check all requests completed successfully
	for i := 0; i < len(requests); i++ {
		if err := <-errChan; err != nil {
			t.Errorf("concurrent request %d failed: %v", i, err)
		}
	}
}

func TestRedTeam_MessagingService_SendMessage_VeryLongChannelID(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			// Verify very long channel ID is handled
			if len(target) != 1000 {
				t.Errorf("expected 1000-char channel ID, got %d", len(target))
			}
			return "msg1", nil
		},
	}
	svc.AddAdapter("mock", mock)

	longChannelID := string(make([]byte, 1000))
	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   longChannelID,
		Message:  "test message",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage with very long channel ID failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected success = true for very long channel ID")
	}
}

func TestRedTeam_MessagingService_SendMessage_EmptyPlatform(t *testing.T) {
	svc := NewMessagingService()

	req := &pb.SendMessageRequest{
		Platform: "",
		Target:   "channel1",
		Message:  "test message",
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty platform, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code InvalidArgument, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_SendMessage_EmptyTarget(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{}
	svc.AddAdapter("mock", mock)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "",
		Message:  "test message",
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty target, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code InvalidArgument, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_SendMessage_EmptyMessage(t *testing.T) {
	svc := NewMessagingService()
	mock := &mockAdapter{}
	svc.AddAdapter("mock", mock)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "channel1",
		Message:  "",
	}

	_, err := svc.SendMessage(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty message, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code InvalidArgument, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_ListChannels_EmptyPlatform(t *testing.T) {
	svc := NewMessagingService()

	req := &pb.ListChannelsRequest{
		Platform: "",
	}

	_, err := svc.ListChannels(context.Background(), req)
	if err == nil {
		t.Error("expected error for empty platform, got nil")
		return
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Errorf("expected gRPC status error, got: %v", err)
		return
	}

	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected code InvalidArgument, got %s", st.Code())
	}
}

func TestRedTeam_MessagingService_MultipleAdaptersSamePlatform(t *testing.T) {
	svc := NewMessagingService()
	mock1 := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			return "msg-from-mock1", nil
		},
	}
	mock2 := &mockAdapter{
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			return "msg-from-mock2", nil
		},
	}

	// Add first adapter
	svc.AddAdapter("mock", mock1)
	
	// Overwrite with second adapter (should replace the first)
	svc.AddAdapter("mock", mock2)

	req := &pb.SendMessageRequest{
		Platform: "mock",
		Target:   "channel1",
		Message:  "test",
	}

	resp, err := svc.SendMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if resp.MessageId != "msg-from-mock2" {
		t.Errorf("expected message from mock2, got %s (first adapter was not replaced)", resp.MessageId)
	}
}
