package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/aj-nt/mnemosyne/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

func TestMessagingService_StartAdapters_Idempotent(t *testing.T) {
	svc := NewMessagingService()
	connectCalls := 0
	listenCalls := 0
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error {
			connectCalls++
			return nil
		},
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			listenCalls++
			ch := make(chan *pb.MessageEvent, 10)
			return ch, nil
		},
	}
	svc.AddAdapter("mock", mock)

	// Calling startAdapters twice should only connect and listen once
	svc.startAdapters(context.Background())
	svc.startAdapters(context.Background())

	if connectCalls != 1 {
		t.Errorf("expected Connect to be called once, got %d", connectCalls)
	}
	if listenCalls != 1 {
		t.Errorf("expected Listen to be called once, got %d", listenCalls)
	}
}

func TestMessagingService_StartAdapters_ConnectError(t *testing.T) {
	svc := NewMessagingService()
	connectCalls := 0
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error {
			connectCalls++
			return errors.New("connection failed")
		},
	}
	svc.AddAdapter("mock", mock)

	svc.startAdapters(context.Background())

	if connectCalls != 1 {
		t.Errorf("expected Connect to be called once, got %d", connectCalls)
	}
	// Adapter should not be marked as started
	svc.mu.Lock()
	started := svc.adaptersStarted["mock"]
	svc.mu.Unlock()
	if started {
		t.Error("adapter should not be marked as started after Connect error")
	}
}

func TestMessagingService_FanOut(t *testing.T) {
	svc := NewMessagingService()

	// Create a mock adapter that produces messages
	msgCh := make(chan *pb.MessageEvent, 10)
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return msgCh, nil
		},
	}
	svc.AddAdapter("mock", mock)

	// Start the adapter
	svc.startAdapters(context.Background())

	// Add two subscriber channels
	svc.mu.Lock()
	svc.subscriptionID++
	sub1 := make(chan *pb.MessageEvent, 10)
	svc.subscribers["sub-1"] = sub1
	svc.subscriptionID++
	sub2 := make(chan *pb.MessageEvent, 10)
	svc.subscribers["sub-2"] = sub2
	svc.mu.Unlock()

	// Send a message through the adapter channel
	testEvent := &pb.MessageEvent{
		Platform:    "mock",
		ChannelId:   "test-channel",
		Message:     "hello from adapter",
		SenderName:  "testbot",
	}
	msgCh <- testEvent

	// Give the fan-out goroutine time to process
	time.Sleep(50 * time.Millisecond)

	// Both subscribers should receive the message
	select {
	case msg := <-sub1:
		if msg.Message != "hello from adapter" {
			t.Errorf("sub1: expected 'hello from adapter', got %q", msg.Message)
		}
	default:
		t.Error("sub1: expected to receive message, but channel empty")
	}

	select {
	case msg := <-sub2:
		if msg.Message != "hello from adapter" {
			t.Errorf("sub2: expected 'hello from adapter', got %q", msg.Message)
		}
	default:
		t.Error("sub2: expected to receive message, but channel empty")
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

// --- New Red Team Tests ---

// Test fan-out with subscriber channel full (backpressure/dropping)
func TestRedTeam_MessagingService_FanOutChannelFull(t *testing.T) {
	svc := NewMessagingService()

	// Create a mock adapter that produces messages
	msgCh := make(chan *pb.MessageEvent, 10)
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return msgCh, nil
		},
	}
	svc.AddAdapter("mock", mock)

	// Start the adapter
	svc.startAdapters(context.Background())

	// Add a subscriber channel with size 1 (will fill quickly)
	svc.mu.Lock()
	svc.subscriptionID++
	sub1 := make(chan *pb.MessageEvent, 1)
	svc.subscribers["sub-1"] = sub1
	svc.mu.Unlock()

	// Fill the subscriber channel
	testEvent1 := &pb.MessageEvent{
		Platform:    "mock",
		ChannelId:   "test-channel",
		Message:     "message 1",
		SenderName:  "testbot",
	}
	msgCh <- testEvent1

	// Give the fan-out goroutine time to process
	time.Sleep(50 * time.Millisecond)

	// Send another message - should be dropped due to full channel
	testEvent2 := &pb.MessageEvent{
		Platform:    "mock",
		ChannelId:   "test-channel",
		Message:     "message 2",
		SenderName:  "testbot",
	}
	msgCh <- testEvent2

	// Give the fan-out goroutine time to process
	time.Sleep(50 * time.Millisecond)

	// Verify only the first message was received
	select {
	case msg := <-sub1:
		if msg.Message != "message 1" {
			t.Errorf("expected 'message 1', got %q", msg.Message)
		}
	default:
		t.Error("expected to receive message 1, but channel empty")
	}

	// Second message should have been dropped
	select {
	case msg := <-sub1:
		t.Errorf("expected message 2 to be dropped, but received: %q", msg.Message)
	default:
		// This is expected - message was dropped
	}
}

// Test startAdapters Connect error then retry
func TestRedTeam_MessagingService_StartAdapters_ConnectErrorThenRetry(t *testing.T) {
	svc := NewMessagingService()
	connectCalls := 0
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error {
			connectCalls++
			if connectCalls == 1 {
				return errors.New("connection failed")
			}
			return nil
		},
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return make(chan *pb.MessageEvent, 10), nil
		},
	}
	svc.AddAdapter("mock", mock)

	// First call should fail
	svc.startAdapters(context.Background())

	if connectCalls != 1 {
		t.Errorf("expected Connect to be called once, got %d", connectCalls)
	}

	svc.mu.Lock()
	started := svc.adaptersStarted["mock"]
	svc.mu.Unlock()
	if started {
		t.Error("adapter should not be marked as started after Connect error")
	}

	// Second call should succeed
	svc.startAdapters(context.Background())

	if connectCalls != 2 {
		t.Errorf("expected Connect to be called twice, got %d", connectCalls)
	}

	svc.mu.Lock()
	started = svc.adaptersStarted["mock"]
	svc.mu.Unlock()
	if !started {
		t.Error("adapter should be marked as started after successful Connect")
	}
}

// Test SubscribeToMessages with no adapters
func TestRedTeam_MessagingService_SubscribeToMessages_NoAdapters(t *testing.T) {
	svc := NewMessagingService()

	// Create a context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create a mock stream
	mockStream := &mockSubscribeToMessagesServer{
		sendFunc: func(msg *pb.MessageEvent) error {
			return nil
		},
		contextFunc: func() context.Context {
			return ctx
		},
	}

	// Subscribe with no adapters - should not crash
	err := svc.SubscribeToMessages(&pb.SubscribeToMessagesRequest{}, mockStream)
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		t.Errorf("expected no error or context.Canceled/DeadlineExceeded, got %v", err)
	}
}

// Test multiple rapid messages through fan-out
func TestRedTeam_MessagingService_FanOut_RapidMessages(t *testing.T) {
	svc := NewMessagingService()

	// Create a mock adapter that produces messages
	msgCh := make(chan *pb.MessageEvent, 100)
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return msgCh, nil
		},
	}
	svc.AddAdapter("mock", mock)

	// Start the adapter
	svc.startAdapters(context.Background())

	// Add a subscriber channel
	svc.mu.Lock()
	svc.subscriptionID++
	sub1 := make(chan *pb.MessageEvent, 100)
	svc.subscribers["sub-1"] = sub1
	svc.mu.Unlock()

	// Send multiple rapid messages
	for i := 0; i < 10; i++ {
		testEvent := &pb.MessageEvent{
			Platform:    "mock",
			ChannelId:   "test-channel",
			Message:     fmt.Sprintf("message %d", i),
			SenderName:  "testbot",
		}
		msgCh <- testEvent
	}

	// Give the fan-out goroutine time to process
	time.Sleep(100 * time.Millisecond)

	// Verify all messages were received
	receivedCount := 0
	for i := 0; i < 10; i++ {
		select {
		case msg := <-sub1:
			if msg.Message != fmt.Sprintf("message %d", i) {
				t.Errorf("expected 'message %d', got %q", i, msg.Message)
			}
			receivedCount++
		case <-time.After(100 * time.Millisecond):
			t.Errorf("timed out waiting for message %d", i)
			return
		}
	}

	if receivedCount != 10 {
		t.Errorf("expected to receive 10 messages, got %d", receivedCount)
	}
}

// Test adapter Listen error
func TestRedTeam_MessagingService_StartAdapters_ListenError(t *testing.T) {
	svc := NewMessagingService()
	listenCalls := 0
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			listenCalls++
			return nil, errors.New("listen failed")
		},
	}
	svc.AddAdapter("mock", mock)

	svc.startAdapters(context.Background())

	if listenCalls != 1 {
		t.Errorf("expected Listen to be called once, got %d", listenCalls)
	}

	svc.mu.Lock()
	started := svc.adaptersStarted["mock"]
	svc.mu.Unlock()
	if started {
		t.Error("adapter should not be marked as started after Listen error")
	}
}

// Test concurrent subscribers adding/removing during fan-out
func TestRedTeam_MessagingService_ConcurrentSubscribers(t *testing.T) {
	svc := NewMessagingService()

	// Create a mock adapter that produces messages
	msgCh := make(chan *pb.MessageEvent, 100)
	mock := &mockAdapter{
		connectFunc: func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return msgCh, nil
		},
	}
	svc.AddAdapter("mock", mock)

	// Start the adapter
	svc.startAdapters(context.Background())

	// Add multiple subscriber channels concurrently
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			svc.mu.Lock()
			svc.subscriptionID++
			subID := fmt.Sprintf("sub-%d", id)
			subChan := make(chan *pb.MessageEvent, 10)
			svc.subscribers[subID] = subChan
			svc.mu.Unlock()

			// Remove the subscriber after a short delay
			time.Sleep(10 * time.Millisecond)
			svc.mu.Lock()
			delete(svc.subscribers, subID)
			svc.mu.Unlock()
		}(i)
	}

	// Send messages while subscribers are being added/removed
	for i := 0; i < 10; i++ {
		testEvent := &pb.MessageEvent{
			Platform:    "mock",
			ChannelId:   "test-channel",
			Message:     fmt.Sprintf("message %d", i),
			SenderName:  "testbot",
		}
		msgCh <- testEvent
	}

	// Wait for all subscribers to finish
	wg.Wait()

	// Give the fan-out goroutine time to process
	time.Sleep(100 * time.Millisecond)
}

// mockSubscribeToMessagesServer is a mock implementation of pb.Messaging_SubscribeToMessagesServer
type mockSubscribeToMessagesServer struct {
	sendFunc    func(*pb.MessageEvent) error
	contextFunc func() context.Context
}

func (m *mockSubscribeToMessagesServer) Send(msg *pb.MessageEvent) error {
	if m.sendFunc != nil {
		return m.sendFunc(msg)
	}
	return nil
}

func (m *mockSubscribeToMessagesServer) Context() context.Context {
	if m.contextFunc != nil {
		return m.contextFunc()
	}
	return context.Background()
}

func (m *mockSubscribeToMessagesServer) RecvMsg(any) error {
	// Not used in this test
	return nil
}

func (m *mockSubscribeToMessagesServer) SendHeader(md metadata.MD) error {
	// Not used in this test
	return nil
}

func (m *mockSubscribeToMessagesServer) SendMsg(msg any) error {
	if m.sendFunc != nil {
		if typedMsg, ok := msg.(*pb.MessageEvent); ok {
			return m.sendFunc(typedMsg)
		}
	}
	return nil
}

func (m *mockSubscribeToMessagesServer) SetHeader(md metadata.MD) error {
	// Not used in this test
	return nil
}

func (m *mockSubscribeToMessagesServer) SetTrailer(md metadata.MD) {
	// Not used in this test
}
