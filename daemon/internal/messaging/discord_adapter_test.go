// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package messaging

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aj-nt/vassago-sdk/proto"
	"github.com/bwmarrin/discordgo"
)

// mockDiscordSession implements discordSession for testing.
type mockDiscordSession struct {
	sendFunc         func(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	guildFunc        func(guildID string, options ...discordgo.RequestOption) (*discordgo.Guild, error)
	channelsFunc     func(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Channel, error)
	channelFunc      func(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	openFunc         func() error
	closeFunc        func() error
	addHandlerCalled bool
}

func (m *mockDiscordSession) ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.sendFunc != nil {
		return m.sendFunc(channelID, content, options...)
	}
	return &discordgo.Message{ID: "msg1"}, nil
}

func (m *mockDiscordSession) Guild(guildID string, options ...discordgo.RequestOption) (*discordgo.Guild, error) {
	if m.guildFunc != nil {
		return m.guildFunc(guildID, options...)
	}
	return &discordgo.Guild{ID: guildID, Name: "Test Guild"}, nil
}

func (m *mockDiscordSession) GuildChannels(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Channel, error) {
	if m.channelsFunc != nil {
		return m.channelsFunc(guildID, options...)
	}
	return []*discordgo.Channel{
		{ID: "channel1", Name: "test-channel", Type: discordgo.ChannelTypeGuildText},
	}, nil
}

func (m *mockDiscordSession) Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	if m.channelFunc != nil {
		return m.channelFunc(channelID, options...)
	}
	return &discordgo.Channel{ID: channelID, Name: "test-channel"}, nil
}

func (m *mockDiscordSession) Open() error {
	m.addHandlerCalled = true
	if m.openFunc != nil {
		return m.openFunc()
	}
	return nil
}

func (m *mockDiscordSession) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockDiscordSession) AddHandler(_ interface{}) func() {
	m.addHandlerCalled = true
	return func() {}
}

// --- Tests ---

func TestNewDiscordAdapter(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		Guilds:          []string{"guild1"},
		AllowedChannels: []string{"channel1"},
	}
	adapter := NewDiscordAdapter(config)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if len(adapter.config.AllowedChannels) != 1 {
		t.Errorf("expected 1 allowed channel, got %d", len(adapter.config.AllowedChannels))
	}
}

func TestDiscordAdapter_Send_ShortMessage(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		Guilds:          []string{"guild1"},
		AllowedChannels: []string{"channel1"},
	}
	mock := &mockDiscordSession{
		sendFunc: func(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
			if channelID != "channel1" {
				t.Errorf("expected channel1, got %s", channelID)
			}
			if content != "test message" {
				t.Errorf("expected 'test message', got %s", content)
			}
			return &discordgo.Message{ID: "msg1"}, nil
		},
	}

	adapter := NewDiscordAdapter(config)
	adapter.setSession(mock)

	msgID, err := adapter.Send(context.Background(), "channel1", "test message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgID != "msg1" {
		t.Errorf("expected msg1, got %s", msgID)
	}
}

func TestDiscordAdapter_Send_SplitLongMessage(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		Guilds:          []string{"guild1"},
		AllowedChannels: []string{"channel1"},
	}
	longMessage := strings.Repeat("a", 2500)

	callCount := 0
	mock := &mockDiscordSession{
		sendFunc: func(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
			callCount++
			// Second chunk has "(continued) " prefix
			if callCount == 2 && !strings.HasPrefix(content, "(continued) ") {
				t.Errorf("expected second chunk to have '(continued) ' prefix, got: %s", content[:20])
			}
			return &discordgo.Message{ID: "msg" + string(rune('0'+callCount))}, nil
		},
	}

	adapter := NewDiscordAdapter(config)
	adapter.setSession(mock)

	_, err := adapter.Send(context.Background(), "channel1", longMessage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 send calls for 2500-char message, got %d", callCount)
	}
}

func TestDiscordAdapter_Send_NotConnected(t *testing.T) {
	config := DiscordConfig{Token: "test-token"}
	adapter := NewDiscordAdapter(config)
	// No session set — should return error

	_, err := adapter.Send(context.Background(), "channel1", "test")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestDiscordAdapter_ListChannels(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		Guilds:          []string{"guild1"},
		AllowedChannels: []string{"channel1"},
	}
	mock := &mockDiscordSession{
		channelsFunc: func(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Channel, error) {
			return []*discordgo.Channel{
				{ID: "channel1", Name: "general", Type: discordgo.ChannelTypeGuildText},
				{ID: "channel2", Name: "voice-chat", Type: discordgo.ChannelTypeGuildVoice},
			}, nil
		},
	}

	adapter := NewDiscordAdapter(config)
	adapter.setSession(mock)

	channels, err := adapter.ListChannels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only text channels should be included
	if len(channels) != 1 {
		t.Errorf("expected 1 text channel, got %d", len(channels))
	}
	if channels[0].Id != "channel1" {
		t.Errorf("expected channel1, got %s", channels[0].Id)
	}
	if channels[0].Platform != "discord" {
		t.Errorf("expected platform discord, got %s", channels[0].Platform)
	}
}

func TestDiscordAdapter_ListChannels_NotConnected(t *testing.T) {
	config := DiscordConfig{Token: "test-token"}
	adapter := NewDiscordAdapter(config)

	_, err := adapter.ListChannels(context.Background())
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestDiscordAdapter_Close(t *testing.T) {
	config := DiscordConfig{Token: "test-token"}
	closeCalled := false
	mock := &mockDiscordSession{
		closeFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	adapter := NewDiscordAdapter(config)
	adapter.setSession(mock)

	err := adapter.Close(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closeCalled {
		t.Error("expected Close to be called on session")
	}
}

func TestDiscordAdapter_IsAllowedChannel(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		AllowedChannels: []string{"channel1", "channel2"},
	}
	adapter := NewDiscordAdapter(config)

	if !adapter.isAllowedChannel("channel1") {
		t.Error("expected channel1 to be allowed")
	}
	if !adapter.isAllowedChannel("channel2") {
		t.Error("expected channel2 to be allowed")
	}
	if adapter.isAllowedChannel("channel3") {
		t.Error("expected channel3 to be blocked")
	}
}

func TestDiscordAdapter_IsAllowedChannel_EmptyListAllowsAll(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		AllowedChannels: []string{}, // empty = all channels allowed
	}
	adapter := NewDiscordAdapter(config)

	if !adapter.isAllowedChannel("any-channel") {
		t.Error("expected empty allowed list to allow all channels")
	}
}

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		count   int
	}{
		{"short message", "hello", 1},
		{"exact limit", strings.Repeat("a", 2000), 1},
		{"over limit by 1", strings.Repeat("a", 2001), 2},
		{"long message", strings.Repeat("a", 5000), 3},
		{"empty message", "", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := SplitMessageForPlatform("discord", tt.message)
			if len(chunks) != tt.count {
				t.Errorf("expected %d chunks, got %d", tt.count, len(chunks))
			}
		})
	}
}

func TestDiscordAdapter_MessageCreate_AllowedChannel(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		AllowedChannels: []string{"channel1"},
	}
	messageChan := make(chan *proto.MessageEvent, 1)
	adapter := NewDiscordAdapter(config)
	adapter.messageChan = messageChan
	adapter.closeChan = make(chan struct{})

	// Create a fake discordgo.Session with minimal state
	session := &discordgo.Session{
		State: &discordgo.State{
			Ready: discordgo.Ready{
				User: &discordgo.User{ID: "bot1"},
			},
		},
	}

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg1",
			ChannelID: "channel1",
			Content:   "hello from discord",
			Author:    &discordgo.User{ID: "user1", Username: "testuser"},
			Timestamp: time.Now(),
		},
	}

	adapter.messageCreate(session, msg)

	select {
	case event := <-messageChan:
		if event.Platform != "discord" {
			t.Errorf("expected platform discord, got %s", event.Platform)
		}
		if event.ChannelId != "channel1" {
			t.Errorf("expected channel1, got %s", event.ChannelId)
		}
		if event.SenderId != "user1" {
			t.Errorf("expected user1, got %s", event.SenderId)
		}
		if event.Message != "hello from discord" {
			t.Errorf("expected 'hello from discord', got %s", event.Message)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message event")
	}
}

func TestDiscordAdapter_MessageCreate_BlockedChannel(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		AllowedChannels: []string{"channel1"},
	}
	messageChan := make(chan *proto.MessageEvent, 1)
	adapter := NewDiscordAdapter(config)
	adapter.messageChan = messageChan
	adapter.closeChan = make(chan struct{})

	session := &discordgo.Session{
		State: &discordgo.State{
			Ready: discordgo.Ready{
				User: &discordgo.User{ID: "bot1"},
			},
		},
	}

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg1",
			ChannelID: "channel2", // not in allowed list
			Content:   "hello from discord",
			Author:    &discordgo.User{ID: "user1", Username: "testuser"},
			Timestamp: time.Now(),
		},
	}

	adapter.messageCreate(session, msg)

	select {
	case <-messageChan:
		t.Error("expected no message event for blocked channel")
	default:
		// Good — no event sent
	}
}

func TestDiscordAdapter_MessageCreate_IgnoresBotMessages(t *testing.T) {
	config := DiscordConfig{
		Token:           "test-token",
		AllowedChannels: []string{"channel1"},
	}
	messageChan := make(chan *proto.MessageEvent, 1)
	adapter := NewDiscordAdapter(config)
	adapter.messageChan = messageChan
	adapter.closeChan = make(chan struct{})

	session := &discordgo.Session{
		State: &discordgo.State{
			Ready: discordgo.Ready{
				User: &discordgo.User{ID: "bot1"},
			},
		},
	}

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg1",
			ChannelID: "channel1",
			Content:   "my own message",
			Author:    &discordgo.User{ID: "bot1", Username: "bot"}, // same as bot
			Timestamp: time.Now(),
		},
	}

	adapter.messageCreate(session, msg)

	select {
	case <-messageChan:
		t.Error("expected no message event for bot's own message")
	default:
		// Good — bot's own messages are ignored
	}
}
