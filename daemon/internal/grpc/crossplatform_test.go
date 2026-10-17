// This file is part of Vassago.
// See LICENSE for license information.

package grpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

// Red Team: Cross-platform message routing tests

func TestRedTeam_MessagingService_MultipleAdaptersSendMessage(t *testing.T) {
	svc := NewMessagingService()

	discordSent := 0
	telegramSent := 0
	emailSent := 0
	imessageSent := 0

	discord := &mockAdapter{
		platformName: "discord",
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			discordSent++
			return fmt.Sprintf("discord-msg-%d", discordSent), nil
		},
	}
	telegram := &mockAdapter{
		platformName: "telegram",
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			telegramSent++
			return fmt.Sprintf("telegram-msg-%d", telegramSent), nil
		},
	}
	email := &mockAdapter{
		platformName: "email",
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			emailSent++
			return fmt.Sprintf("email-msg-%d", emailSent), nil
		},
	}
	imessage := &mockAdapter{
		platformName: "imessage",
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			imessageSent++
			return fmt.Sprintf("imessage-msg-%d", imessageSent), nil
		},
	}

	svc.AddAdapter("discord", discord)
	svc.AddAdapter("telegram", telegram)
	svc.AddAdapter("email", email)
	svc.AddAdapter("imessage", imessage)

	// Send to Discord
	resp, err := svc.SendMessage(context.Background(), &pb.SendMessageRequest{
		Platform: "discord",
		Target:   "channel-1",
		Message:  "Hello Discord!",
	})
	if err != nil {
		t.Fatalf("SendMessage to discord failed: %v", err)
	}
	if !resp.Success {
		t.Error("expected success = true")
	}
	if discordSent != 1 {
		t.Errorf("expected 1 discord send, got %d", discordSent)
	}

	// Send to Telegram
	_, err = svc.SendMessage(context.Background(), &pb.SendMessageRequest{
		Platform: "telegram",
		Target:   "-100123456",
		Message:  "Hello Telegram!",
	})
	if err != nil {
		t.Fatalf("SendMessage to telegram failed: %v", err)
	}
	if telegramSent != 1 {
		t.Errorf("expected 1 telegram send, got %d", telegramSent)
	}

	// Send to Email
	_, err = svc.SendMessage(context.Background(), &pb.SendMessageRequest{
		Platform: "email",
		Target:   "user@example.com",
		Message:  "Hello Email!",
	})
	if err != nil {
		t.Fatalf("SendMessage to email failed: %v", err)
	}
	if emailSent != 1 {
		t.Errorf("expected 1 email send, got %d", emailSent)
	}

	// Send to iMessage
	_, err = svc.SendMessage(context.Background(), &pb.SendMessageRequest{
		Platform: "imessage",
		Target:   "+15551234567",
		Message:  "Hello iMessage!",
	})
	if err != nil {
		t.Fatalf("SendMessage to imessage failed: %v", err)
	}
	if imessageSent != 1 {
		t.Errorf("expected 1 imessage send, got %d", imessageSent)
	}

	// No cross-contamination: each adapter should only have been called once
	if discordSent != 1 || telegramSent != 1 || emailSent != 1 || imessageSent != 1 {
		t.Errorf("expected each adapter called once: discord=%d telegram=%d email=%d imessage=%d",
			discordSent, telegramSent, emailSent, imessageSent)
	}
}

func TestRedTeam_MessagingService_MultiPlatformFanOut(t *testing.T) {
	svc := NewMessagingService()

	// Set up 4 adapters, each producing messages on their own channel
	discordCh := make(chan *pb.MessageEvent, 10)
	telegramCh := make(chan *pb.MessageEvent, 10)
	emailCh := make(chan *pb.MessageEvent, 10)
	imessageCh := make(chan *pb.MessageEvent, 10)

	discord := &mockAdapter{
		platformName: "discord",
		connectFunc:  func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return discordCh, nil
		},
	}
	telegram := &mockAdapter{
		platformName: "telegram",
		connectFunc:  func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return telegramCh, nil
		},
	}
	email := &mockAdapter{
		platformName: "email",
		connectFunc:  func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return emailCh, nil
		},
	}
	imessage := &mockAdapter{
		platformName: "imessage",
		connectFunc:  func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return imessageCh, nil
		},
	}

	svc.AddAdapter("discord", discord)
	svc.AddAdapter("telegram", telegram)
	svc.AddAdapter("email", email)
	svc.AddAdapter("imessage", imessage)

	// Start all adapters
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.startAdapters(ctx)

	// Add subscriber
	svc.mu.Lock()
	svc.subscriptionID++
	subChan := make(chan *pb.MessageEvent, 100)
	svc.subscribers["sub-test"] = subChan
	svc.mu.Unlock()

	// Send messages from all 4 platforms simultaneously
	discordCh <- &pb.MessageEvent{Platform: "discord", ChannelId: "ch1", Message: "discord msg", SenderName: "Alice"}
	telegramCh <- &pb.MessageEvent{Platform: "telegram", ChannelId: "tg1", Message: "telegram msg", SenderName: "Bob"}
	emailCh <- &pb.MessageEvent{Platform: "email", ChannelId: "INBOX", Message: "email msg", SenderName: "Carol"}
	imessageCh <- &pb.MessageEvent{Platform: "imessage", ChannelId: "im1", Message: "imessage msg", SenderName: "Dave"}

	// Collect messages from subscriber (with timeout)
	received := make(map[string]bool)
	for i := 0; i < 4; i++ {
		select {
		case msg := <-subChan:
			received[msg.Platform] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for message %d", i)
		}
	}

	// Verify all 4 platforms were received
	for _, platform := range []string{"discord", "telegram", "email", "imessage"} {
		if !received[platform] {
			t.Errorf("expected message from %s to be forwarded to subscriber", platform)
		}
	}
}

func TestRedTeam_MessagingService_PlatformNameMismatch(t *testing.T) {
	svc := NewMessagingService()

	// Register adapter with key "discord" but PlatformName() returns "telegram"
	// This should log a warning but still work
	discord := &mockAdapter{
		platformName: "telegram", // Mismatch!
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			return "msg1", nil
		},
	}

	// AddAdapter should warn but not error
	svc.AddAdapter("discord", discord)

	// SendMessage should still route by key, not PlatformName()
	resp, err := svc.SendMessage(context.Background(), &pb.SendMessageRequest{
		Platform: "discord",
		Target:   "channel-1",
		Message:  "test",
	})
	if err != nil {
		t.Fatalf("SendMessage should succeed even with mismatched PlatformName: %v", err)
	}
	if !resp.Success {
		t.Error("expected success = true")
	}
}

func TestRedTeam_MessagingService_Status(t *testing.T) {
	svc := NewMessagingService()

	// Before any adapters — empty status
	status := svc.Status()
	if len(status) != 0 {
		t.Errorf("expected empty status before adding adapters, got %d entries", len(status))
	}

	// Add adapters
	discord := &mockAdapter{
		platformName: "discord",
		connectFunc:  func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return make(chan *pb.MessageEvent, 10), nil
		},
	}
	telegram := &mockAdapter{
		platformName: "telegram",
		connectFunc:  func(ctx context.Context) error { return nil },
		listenFunc: func(ctx context.Context) (<-chan *pb.MessageEvent, error) {
			return make(chan *pb.MessageEvent, 10), nil
		},
	}

	svc.AddAdapter("discord", discord)
	svc.AddAdapter("telegram", telegram)

	// Status should show both adapters as not started
	status = svc.Status()
	if len(status) != 2 {
		t.Errorf("expected 2 adapters in status, got %d", len(status))
	}
	if status["discord"] {
		t.Error("discord should not be started yet")
	}
	if status["telegram"] {
		t.Error("telegram should not be started yet")
	}

	// Start adapters
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.startAdapters(ctx)

	// Status should now show both as started
	status = svc.Status()
	if !status["discord"] {
		t.Error("discord should be started after startAdapters")
	}
	if !status["telegram"] {
		t.Error("telegram should be started after startAdapters")
	}
}

func TestRedTeam_MessagingService_ConcurrentMultiPlatformSend(t *testing.T) {
	svc := NewMessagingService()

	var mu sync.Mutex
	sentCounts := map[string]int{
		"discord":  0,
		"telegram": 0,
	}

	discord := &mockAdapter{
		platformName: "discord",
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			mu.Lock()
			sentCounts["discord"]++
			mu.Unlock()
			return "msg-d", nil
		},
	}
	telegram := &mockAdapter{
		platformName: "telegram",
		sendFunc: func(ctx context.Context, target, message string) (string, error) {
			mu.Lock()
			sentCounts["telegram"]++
			mu.Unlock()
			return "msg-t", nil
		},
	}

	svc.AddAdapter("discord", discord)
	svc.AddAdapter("telegram", telegram)

	// Send 50 messages to each platform concurrently
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			svc.SendMessage(context.Background(), &pb.SendMessageRequest{
				Platform: "discord",
				Target:   fmt.Sprintf("ch-%d", idx),
				Message:  fmt.Sprintf("discord msg %d", idx),
			})
		}(i)
		go func(idx int) {
			defer wg.Done()
			svc.SendMessage(context.Background(), &pb.SendMessageRequest{
				Platform: "telegram",
				Target:   fmt.Sprintf("tg-%d", idx),
				Message:  fmt.Sprintf("telegram msg %d", idx),
			})
		}(i)
	}
	wg.Wait()

	mu.Lock()
	discordCount := sentCounts["discord"]
	telegramCount := sentCounts["telegram"]
	mu.Unlock()

	if discordCount != 50 {
		t.Errorf("expected 50 discord sends, got %d", discordCount)
	}
	if telegramCount != 50 {
		t.Errorf("expected 50 telegram sends, got %d", telegramCount)
	}
}

func TestRedTeam_MessagingService_ListChannels_MultiplePlatforms(t *testing.T) {
	svc := NewMessagingService()

	discord := &mockAdapter{
		platformName: "discord",
		listFunc: func(ctx context.Context) ([]*pb.Channel, error) {
			return []*pb.Channel{
				{Id: "ch1", Name: "general", Platform: "discord"},
				{Id: "ch2", Name: "dev", Platform: "discord"},
			}, nil
		},
	}
	email := &mockAdapter{
		platformName: "email",
		listFunc: func(ctx context.Context) ([]*pb.Channel, error) {
			return []*pb.Channel{
				{Id: "INBOX", Name: "INBOX", Platform: "email"},
				{Id: "Archive", Name: "Archive", Platform: "email"},
			}, nil
		},
	}

	svc.AddAdapter("discord", discord)
	svc.AddAdapter("email", email)

	// List Discord channels
	resp, err := svc.ListChannels(context.Background(), &pb.ListChannelsRequest{Platform: "discord"})
	if err != nil {
		t.Fatalf("ListChannels for discord failed: %v", err)
	}
	if len(resp.Channels) != 2 {
		t.Errorf("expected 2 discord channels, got %d", len(resp.Channels))
	}

	// List Email channels
	resp, err = svc.ListChannels(context.Background(), &pb.ListChannelsRequest{Platform: "email"})
	if err != nil {
		t.Fatalf("ListChannels for email failed: %v", err)
	}
	if len(resp.Channels) != 2 {
		t.Errorf("expected 2 email channels, got %d", len(resp.Channels))
	}
}
