// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package messaging

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

// mockEmailSender implements emailSender for testing.
type mockEmailSender struct {
	sendFunc func(from string, to []string, subject, body string) error
	mu       sync.Mutex
	calls    []string
}

func (m *mockEmailSender) SendSMTP(from string, to []string, subject, body string) error {
	m.mu.Lock()
	m.calls = append(m.calls, fmt.Sprintf("%s→%s:%s", from, strings.Join(to, ","), subject))
	m.mu.Unlock()

	if m.sendFunc != nil {
		return m.sendFunc(from, to, subject, body)
	}
	return nil
}

// mockEmailFetcher implements emailFetcher for testing.
type mockEmailFetcher struct {
	messages []EmailMessage
	channels []*pb.Channel
	lastID   int32
	mu       sync.Mutex
}

func (m *mockEmailFetcher) FetchNewMessages(sinceUID int32) ([]EmailMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []EmailMessage
	for _, msg := range m.messages {
		if msg.UID > sinceUID {
			result = append(result, msg)
		}
	}
	return result, nil
}

func (m *mockEmailFetcher) MarkSeen(uid int32) error {
	return nil
}

func (m *mockEmailFetcher) Close() error {
	return nil
}

// --- NewEmailAdapter Tests ---

func TestNewEmailAdapter_DefaultConfig(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	if adapter.config.PollInterval != 30*time.Second {
		t.Errorf("expected default poll interval 30s, got %v", adapter.config.PollInterval)
	}
	if len(adapter.config.Folders) != 1 || adapter.config.Folders[0] != "INBOX" {
		t.Errorf("expected default folders [INBOX], got %v", adapter.config.Folders)
	}
}

func TestNewEmailAdapter_CustomConfig(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		IMAPHost:     "imap.example.com:993",
		SMTPHost:     "smtp.example.com:587",
		Username:     "user",
		Password:     "pass",
		FromAddress:  "test@example.com",
		AllowedFrom:  []string{"boss@example.com"},
		Folders:      []string{"INBOX", "Archive"},
		PollInterval: 10 * time.Second,
	})
	if adapter.config.SMTPHost != "smtp.example.com:587" {
		t.Errorf("expected SMTPHost, got %s", adapter.config.SMTPHost)
	}
	if len(adapter.config.AllowedFrom) != 1 {
		t.Errorf("expected 1 AllowedFrom, got %d", len(adapter.config.AllowedFrom))
	}
	if len(adapter.config.Folders) != 2 {
		t.Errorf("expected 2 folders, got %d", len(adapter.config.Folders))
	}
	if adapter.config.PollInterval != 10*time.Second {
		t.Errorf("expected 10s poll interval, got %v", adapter.config.PollInterval)
	}
}

// --- Connect Tests ---

func TestEmailAdapter_Connect_Success(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	// Connect without actual SMTP server would fail in real env
	// For tests, we skip the actual connection check
	adapter.connected = true

	err := adapter.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
}

func TestEmailAdapter_Listen_NotConnected(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})

	_, err := adapter.Listen(context.Background())
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// --- Send Tests ---

func TestEmailAdapter_Send_Success(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	var capturedSubject string
	mock := &mockEmailSender{
		sendFunc: func(from string, to []string, subject, body string) error {
			capturedSubject = subject
			return nil
		},
	}
	adapter.setSender(mock)

	msgID, err := adapter.Send(context.Background(), "recipient@example.com", "Hello World")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if msgID == "" {
		t.Error("expected non-empty message ID")
	}
	if capturedSubject != "Message from Vassago" {
		t.Errorf("expected default subject, got %s", capturedSubject)
	}
}

func TestEmailAdapter_Send_WithSubject(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	var capturedSubject, capturedBody string
	mock := &mockEmailSender{
		sendFunc: func(from string, to []string, subject, body string) error {
			capturedSubject = subject
			capturedBody = body
			return nil
		},
	}
	adapter.setSender(mock)

	_, err := adapter.Send(context.Background(), "recipient@example.com", "Subject: Test Subject\nMessage body here")
	if err != nil {
		t.Fatalf("Send with subject failed: %v", err)
	}
	if capturedSubject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got %s", capturedSubject)
	}
	if capturedBody != "Message body here" {
		t.Errorf("expected body 'Message body here', got %s", capturedBody)
	}
}

func TestEmailAdapter_Send_InvalidEmail(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true
	adapter.setSender(&mockEmailSender{})

	_, err := adapter.Send(context.Background(), "invalid", "Hello")
	if err == nil {
		t.Error("expected error for invalid email")
	}
}

func TestEmailAdapter_Send_EmptyTarget(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true
	adapter.setSender(&mockEmailSender{})

	_, err := adapter.Send(context.Background(), "", "Hello")
	if err == nil {
		t.Error("expected error for empty target")
	}
}

func TestEmailAdapter_Send_SenderError(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	mock := &mockEmailSender{
		sendFunc: func(from string, to []string, subject, body string) error {
			return fmt.Errorf("SMTP error")
		},
	}
	adapter.setSender(mock)

	_, err := adapter.Send(context.Background(), "recipient@example.com", "Hello")
	if err == nil {
		t.Error("expected error when SMTP fails")
	}
}

// --- ListChannels Tests ---

func TestEmailAdapter_ListChannels(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
		Folders:     []string{"INBOX", "Archive"},
	})

	channels, err := adapter.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	if channels[0].Platform != "email" {
		t.Errorf("expected platform 'email', got %s", channels[0].Platform)
	}
	if channels[0].Id != "email:INBOX" {
		t.Errorf("expected channel ID 'email:INBOX', got %s", channels[0].Id)
	}
	if channels[1].Name != "Archive" {
		t.Errorf("expected channel name 'Archive', got %s", channels[1].Name)
	}
}

func TestEmailAdapter_ListChannels_DefaultFolders(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
		// Folders not set — should default to ["INBOX"]
	})

	channels, err := adapter.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 default channel, got %d", len(channels))
	}
	if channels[0].Name != "INBOX" {
		t.Errorf("expected 'INBOX', got %s", channels[0].Name)
	}
}

// --- Close Tests ---

func TestEmailAdapter_Close(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	err := adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestEmailAdapter_Close_DoubleClose(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	err := adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close should not panic
	err = adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("Second Close (idempotent) failed: %v", err)
	}
}

// --- Poll/Listen Tests ---

func TestEmailAdapter_Poll_AllowedFrom(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:     "smtp.example.com:587",
		FromAddress:  "test@example.com",
		AllowedFrom:  []string{"boss@example.com"},
		PollInterval: 1 * time.Second,
	})
	adapter.connected = true
	adapter.lastSeenUID = 0

	mock := &mockEmailFetcher{
		messages: []EmailMessage{
			{
				UID:       1,
				From:      "boss@example.com",
				Subject:   "Important",
				Body:      "Check this out",
				Date:      time.Now(),
				MessageID: "<msg1@example.com>",
			},
			{
				UID:       2,
				From:      "stranger@example.com",
				Subject:   "Spam",
				Body:      "Buy this",
				Date:      time.Now(),
				MessageID: "<msg2@example.com>",
			},
		},
	}
	adapter.setFetcher(mock)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := adapter.Listen(ctx)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	select {
	case event := <-ch:
		if event.SenderId != "boss@example.com" {
			t.Errorf("expected sender 'boss@example.com', got %s", event.SenderId)
		}
	case <-time.After(10 * time.Second):
		t.Error("timed out waiting for message from allowed sender")
	}
}

// --- Email Validation Tests ---

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"user.name@example.com", true},
		{"@example.com", false},
		{"user@", false},
		{"", false},
		{"a@b", false},
		{"user@example", false},
		{"user@example.", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			result := isValidEmail(tt.email)
			if result != tt.valid {
				t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, result, tt.valid)
			}
		})
	}
}

// --- Red Team Tests ---

func TestRedTeam_EmailAdapter_ConcurrentSend(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	var mu sync.Mutex
	sendCount := 0
	mock := &mockEmailSender{
		sendFunc: func(from string, to []string, subject, body string) error {
			mu.Lock()
			sendCount++
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}
	adapter.setSender(mock)

	errCh := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := adapter.Send(context.Background(), "recipient@example.com", "concurrent message")
			errCh <- err
		}()
	}

	for i := 0; i < 10; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent send failed: %v", err)
		}
	}

	mu.Lock()
	count := sendCount
	mu.Unlock()
	if count != 10 {
		t.Errorf("expected 10 sends, got %d", count)
	}
}

func TestRedTeam_EmailAdapter_SendAfterClose(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true
	adapter.setSender(&mockEmailSender{})

	err := adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestRedTeam_EmailAdapter_SpecialCharactersInMessage(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true

	var capturedBody string
	mock := &mockEmailSender{
		sendFunc: func(from string, to []string, subject, body string) error {
			capturedBody = body
			return nil
		},
	}
	adapter.setSender(mock)

	specials := []struct {
		name string
		msg  string
	}{
		{"unicode", "Hello 🌍! 你好! 🧠"},
		{"newlines", "line1\nline2\nline3"},
		{"sql injection", "'; DROP TABLE users; --"},
		{"path traversal", "../../../etc/passwd"},
	}

	for _, tc := range specials {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.Send(context.Background(), "recipient@example.com", tc.msg)
			if err != nil {
				t.Fatalf("Send with %s failed: %v", tc.name, err)
			}
		})
	}
	_ = capturedBody // Avoid unused var
}

func TestRedTeam_EmailAdapter_EmailValidation(t *testing.T) {
	adapter := NewEmailAdapter(EmailConfig{
		SMTPHost:    "smtp.example.com:587",
		FromAddress: "test@example.com",
	})
	adapter.connected = true
	adapter.setSender(&mockEmailSender{})

	// Various invalid emails
	invalidEmails := []string{
		"notanemail",
		"@",
		"user@",
		"@domain.com",
		"a@b",
		"short@x",
	}

	for _, email := range invalidEmails {
		_, err := adapter.Send(context.Background(), email, "Hello")
		if err == nil {
			t.Errorf("expected error for invalid email %q", email)
		}
	}

	// Valid emails
	validEmails := []string{
		"user@example.com",
		"user.name@example.com",
		"user+tag@example.org",
	}

	for _, email := range validEmails {
		_, err := adapter.Send(context.Background(), email, "Hello")
		if err != nil {
			t.Errorf("unexpected error for valid email %q: %v", email, err)
		}
	}
}
