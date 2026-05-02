package messaging

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mockTelegramAPI implements telegramAPI for testing.
type mockTelegramAPI struct {
	sendFunc    func(c tgbotapi.Chattable) (tgbotapi.Message, error)
	updatesChan chan tgbotapi.Update
	stopped     bool
	mu          sync.Mutex
}

func (m *mockTelegramAPI) GetUpdatesChan(config tgbotapi.UpdateConfig) (tgbotapi.UpdatesChannel, error) {
	if m.updatesChan != nil {
		return m.updatesChan, nil
	}
	m.updatesChan = make(chan tgbotapi.Update, 100)
	return m.updatesChan, nil
}

func (m *mockTelegramAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if m.sendFunc != nil {
		return m.sendFunc(c)
	}
	return tgbotapi.Message{MessageID: 1}, nil
}

func (m *mockTelegramAPI) StopReceivingUpdates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
}

// --- Connect Tests ---

func TestTelegramAdapter_Connect_WithMockAPI(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{}
	adapter.setAPI(mockAPI, 12345)

	err := adapter.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if adapter.botID != 12345 {
		t.Errorf("expected botID 12345, got %d", adapter.botID)
	}
}

func TestTelegramAdapter_Connect_Idempotent(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{}
	adapter.setAPI(mockAPI, 12345)

	err := adapter.Connect(context.Background())
	if err != nil {
		t.Fatalf("First Connect failed: %v", err)
	}

	err = adapter.Connect(context.Background())
	if err != nil {
		t.Fatalf("Second Connect (idempotent) failed: %v", err)
	}
}

// --- Send Tests ---

func TestTelegramAdapter_Send_Success(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{
		sendFunc: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			msg, ok := c.(tgbotapi.MessageConfig)
			if !ok {
				t.Errorf("expected MessageConfig, got %T", c)
			}
			if msg.Text != "hello world" {
				t.Errorf("expected text 'hello world', got %s", msg.Text)
			}
			if msg.ChatID != -1001234567890 {
				t.Errorf("expected ChatID -1001234567890, got %d", msg.ChatID)
			}
			return tgbotapi.Message{MessageID: 42}, nil
		},
	}
	adapter.setAPI(mockAPI, 12345)

	msgID, err := adapter.Send(context.Background(), "-1001234567890", "hello world")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if msgID != "42" {
		t.Errorf("expected message ID '42', got %s", msgID)
	}
}

func TestTelegramAdapter_Send_NotConnected(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})

	_, err := adapter.Send(context.Background(), "123", "hello")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestTelegramAdapter_Send_InvalidChatID(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{}
	adapter.setAPI(mockAPI, 12345)

	_, err := adapter.Send(context.Background(), "not_a_number", "hello")
	if err == nil {
		t.Error("expected error for invalid chat ID")
	}
}

func TestTelegramAdapter_Send_AdapterError(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{
		sendFunc: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			return tgbotapi.Message{}, fmt.Errorf("network error")
		},
	}
	adapter.setAPI(mockAPI, 12345)

	_, err := adapter.Send(context.Background(), "-100123", "hello")
	if err == nil {
		t.Error("expected error when adapter fails")
	}
}

func TestTelegramAdapter_Send_HTMLParseMode(t *testing.T) {
	var capturedMsg tgbotapi.MessageConfig
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{
		sendFunc: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			msg, ok := c.(tgbotapi.MessageConfig)
			if !ok {
				t.Errorf("expected MessageConfig, got %T", c)
			}
			capturedMsg = msg
			return tgbotapi.Message{MessageID: 1}, nil
		},
	}
	adapter.setAPI(mockAPI, 12345)

	_, err := adapter.Send(context.Background(), "-100123", "<b>bold</b>")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if capturedMsg.ParseMode != tgbotapi.ModeHTML {
		t.Errorf("expected ParseMode HTML, got %s", capturedMsg.ParseMode)
	}
}

// --- Long Message Splitting ---

func TestTelegramAdapter_Send_LongMessage(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})

	longMessage := ""
	for i := 0; i < 5000; i++ {
		longMessage += "a"
	}
	var sendCount int
	var mu sync.Mutex

	mockAPI := &mockTelegramAPI{
		sendFunc: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			mu.Lock()
			sendCount++
			mu.Unlock()
			return tgbotapi.Message{MessageID: sendCount}, nil
		},
	}
	adapter.setAPI(mockAPI, 12345)

	msgID, err := adapter.Send(context.Background(), "-100123", longMessage)
	if err != nil {
		t.Fatalf("Send long message failed: %v", err)
	}

	mu.Lock()
	count := sendCount
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 sends for a 5000-char message, got %d", count)
	}

	if msgID == "" {
		t.Error("expected non-empty message ID")
	}
}

// --- Listen Tests ---

func TestTelegramAdapter_Listen_NotConnected(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})

	_, err := adapter.Listen(context.Background())
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestTelegramAdapter_Listen_FiltersBotMessages(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	updates := make(chan tgbotapi.Update, 10)
	mockAPI := &mockTelegramAPI{updatesChan: updates}
	adapter.setAPI(mockAPI, 42) // Bot ID is 42

	ch, err := adapter.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// Send a message from the bot itself (should be filtered)
	updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From:      &tgbotapi.User{ID: 42, UserName: "testbot"},
			Chat:      &tgbotapi.Chat{ID: -100123, Title: "Test Group"},
			Text:      "I am a bot",
		},
	}

	// Send a message from a real user (should pass through)
	updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 2,
			From:      &tgbotapi.User{ID: 123, UserName: "alice", FirstName: "Alice"},
			Chat:      &tgbotapi.Chat{ID: -100123, Title: "Test Group"},
			Text:      "hello from alice",
		},
	}

	// Allow goroutine to process
	time.Sleep(50 * time.Millisecond)

	select {
	case event := <-ch:
		if event.SenderName != "alice" {
			t.Errorf("expected sender 'alice', got %s", event.SenderName)
		}
		if event.Message != "hello from alice" {
			t.Errorf("expected message 'hello from alice', got %s", event.Message)
		}
		if event.Platform != "telegram" {
			t.Errorf("expected platform 'telegram', got %s", event.Platform)
		}
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for message")
	}
}

func TestTelegramAdapter_Listen_AllowedChats(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{
		Token:        "test-token",
		AllowedChats: []int64{-100123, -100456},
	})

	updates := make(chan tgbotapi.Update, 10)
	mockAPI := &mockTelegramAPI{updatesChan: updates}
	adapter.setAPI(mockAPI, 42)

	ch, err := adapter.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// Message from disallowed chat (should be filtered)
	updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From:      &tgbotapi.User{ID: 999, UserName: "mallory"},
			Chat:      &tgbotapi.Chat{ID: -100789, Title: "Bad Chat"},
			Text:      "spam",
		},
	}

	// Message from allowed chat (should pass)
	updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 2,
			From:      &tgbotapi.User{ID: 123, UserName: "bob"},
			Chat:      &tgbotapi.Chat{ID: -100123, Title: "Allowed Chat"},
			Text:      "hello bob",
		},
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case event := <-ch:
		if event.ChannelId != "-100123" {
			t.Errorf("expected channel ID '-100123', got %s", event.ChannelId)
		}
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for allowed message")
	}
}

func TestTelegramAdapter_Listen_SenderNameFallback(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	updates := make(chan tgbotapi.Update, 10)
	mockAPI := &mockTelegramAPI{updatesChan: updates}
	adapter.setAPI(mockAPI, 42)

	ch, err := adapter.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// User with no username but has first name
	updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 1,
			From:      &tgbotapi.User{ID: 123, UserName: "", FirstName: "Charlie"},
			Chat:      &tgbotapi.Chat{ID: -100123, Title: "Test"},
			Text:      "hello",
		},
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case event := <-ch:
		if event.SenderName != "Charlie" {
			t.Errorf("expected sender name 'Charlie', got %s", event.SenderName)
		}
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for message")
	}
}

func TestTelegramAdapter_Listen_EmptyMessageIgnored(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	updates := make(chan tgbotapi.Update, 10)
	mockAPI := &mockTelegramAPI{updatesChan: updates}
	adapter.setAPI(mockAPI, 42)

	ch, err := adapter.Listen(context.Background())
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// Update with no message (should be ignored)
	updates <- tgbotapi.Update{}

	// Valid message
	updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 2,
			From:      &tgbotapi.User{ID: 123, UserName: "alice"},
			Chat:      &tgbotapi.Chat{ID: -100123, Title: "Test"},
			Text:      "valid message",
		},
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case event := <-ch:
		if event.Message != "valid message" {
			t.Errorf("expected 'valid message', got %s", event.Message)
		}
	case <-time.After(1 * time.Second):
		t.Error("timed out waiting for valid message")
	}
}

// --- Close Tests ---

func TestTelegramAdapter_Close_Success(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{}
	adapter.setAPI(mockAPI, 12345)

	err := adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !mockAPI.stopped {
		t.Error("expected StopReceivingUpdates to be called")
	}
}

func TestTelegramAdapter_Close_NotConnected(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})

	err := adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("Close on unconnected adapter should not error, got: %v", err)
	}
}

// --- ListChannels Tests ---

func TestTelegramAdapter_ListChannels_Empty(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})

	channels, err := adapter.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}

	if len(channels) != 0 {
		t.Errorf("expected empty channels list, got %d", len(channels))
	}
}

// --- NewTelegramAdapter Tests ---

func TestNewTelegramAdapter_Config(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{
		Token:        "test-token",
		AllowedChats: []int64{-100123, -100456},
	})

	if adapter.config.Token != "test-token" {
		t.Errorf("expected token 'test-token', got %s", adapter.config.Token)
	}
	if len(adapter.config.AllowedChats) != 2 {
		t.Errorf("expected 2 allowed chats, got %d", len(adapter.config.AllowedChats))
	}
}

func TestNewTelegramAdapter_NoAllowedChats(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})

	if len(adapter.config.AllowedChats) != 0 {
		t.Errorf("expected empty allowed chats, got %d", len(adapter.config.AllowedChats))
	}
}

// --- Red Team Tests ---

func TestRedTeam_TelegramAdapter_Send_UnicodeMessage(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{
		sendFunc: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			msg, ok := c.(tgbotapi.MessageConfig)
			if !ok {
				t.Errorf("expected MessageConfig, got %T", c)
			}
			if msg.Text != "Hello 🌍! 你好! 🧠" {
				t.Errorf("unicode message not preserved, got %s", msg.Text)
			}
			return tgbotapi.Message{MessageID: 1}, nil
		},
	}
	adapter.setAPI(mockAPI, 12345)

	msgID, err := adapter.Send(context.Background(), "-100123", "Hello 🌍! 你好! 🧠")
	if err != nil {
		t.Fatalf("Send with unicode failed: %v", err)
	}
	if msgID != "1" {
		t.Errorf("expected message ID '1', got %s", msgID)
	}
}

func TestRedTeam_TelegramAdapter_Send_VeryLongChatID(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{}
	adapter.setAPI(mockAPI, 12345)

	msgID, err := adapter.Send(context.Background(), "-1009999999999", "test")
	if err != nil {
		t.Fatalf("Send with very long chat ID failed: %v", err)
	}
	if msgID != "1" {
		t.Errorf("expected message ID '1', got %s", msgID)
	}
}

func TestRedTeam_TelegramAdapter_ConcurrentSend(t *testing.T) {
	adapter := NewTelegramAdapter(TelegramConfig{Token: "test-token"})
	mockAPI := &mockTelegramAPI{
		sendFunc: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			time.Sleep(10 * time.Millisecond)
			return tgbotapi.Message{MessageID: 1}, nil
		},
	}
	adapter.setAPI(mockAPI, 12345)

	errCh := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := adapter.Send(context.Background(), "-100123", "concurrent message")
			errCh <- err
		}()
	}

	for i := 0; i < 10; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent request failed: %v", err)
		}
	}
}