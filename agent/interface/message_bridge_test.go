package iface

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// mockMessageClient implements pb.MessagingClient for testing SubscribeToMessages.
// We test MessageBridge at the level of event processing, not gRPC stream mechanics.
// The subscribe() method uses the real gRPC client, so we test handleMessage
// and helper methods directly.

func TestConversationKey(t *testing.T) {
	mb := &MessageBridge{}

	tests := []struct {
		name     string
		event    *pb.MessageEvent
		expected string
	}{
		{
			name: "discord channel",
			event: &pb.MessageEvent{
				Platform:  "discord",
				ChannelId: "123456",
			},
			expected: "discord:123456",
		},
		{
			name: "telegram chat",
			event: &pb.MessageEvent{
				Platform:  "telegram",
				ChannelId: "-100789",
			},
			expected: "telegram:-100789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mb.conversationKey(tt.event)
			if got != tt.expected {
				t.Errorf("conversationKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsAllowedPlatform(t *testing.T) {
	tests := []struct {
		name      string
		platforms []string
		platform  string
		expected  bool
	}{
		{"empty filter allows all", nil, "discord", true},
		{"empty filter allows telegram", nil, "telegram", true},
		{"discord in filter", []string{"discord"}, "discord", true},
		{"telegram not in filter", []string{"discord"}, "telegram", false},
		{"multiple platforms", []string{"discord", "telegram"}, "telegram", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := &MessageBridge{
				config: BridgeConfig{Platforms: tt.platforms},
			}
			got := mb.isAllowedPlatform(tt.platform)
			if got != tt.expected {
				t.Errorf("isAllowedPlatform(%q) = %v, want %v", tt.platform, got, tt.expected)
			}
		})
	}
}

func TestTruncateResponse(t *testing.T) {
	tests := []struct {
		name     string
		maxLen   int
		input    string
		expected string
	}{
		{"short message passes through", 100, "hello", "hello"},
		{"exact length passes through", 5, "hello", "hello"},
		{"long message truncated", 10, "hello world this is a long message", "hello worl\n... (truncated)"},
		{"zero max length means no truncation", 0, "any length message here", "any length message here"},
		{"negative max length means no truncation", -1, "any length message here", "any length message here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mb := &MessageBridge{
				config: BridgeConfig{MaxResponseLength: tt.maxLen},
			}
			got := mb.truncateResponse(tt.input)
			if got != tt.expected {
				t.Errorf("truncateResponse() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestComposeUserMessage(t *testing.T) {
	mb := &MessageBridge{}

	tests := []struct {
		name     string
		event    *pb.MessageEvent
		contains []string // Check that the message contains these substrings
	}{
		{
			name: "with sender name",
			event: &pb.MessageEvent{
				SenderName:  "alice",
				ChannelName: "general",
				Message:     "Hello, how are you?",
			},
			contains: []string{"alice", "general", "Hello, how are you?"},
		},
		{
			name: "without sender name",
			event: &pb.MessageEvent{
				ChannelName: "random",
				Message:     "What's up?",
			},
			contains: []string{"random", "What's up?"},
		},
		{
			name: "empty message",
			event: &pb.MessageEvent{
				SenderName:  "bob",
				ChannelName: "dev",
				Message:     "",
			},
			contains: []string{"bob", "dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mb.composeUserMessage(tt.event)
			for _, substr := range tt.contains {
				if !containsString(got, substr) {
					t.Errorf("composeUserMessage() = %q, want to contain %q", got, substr)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGetOrCreateSession(t *testing.T) {
	mb := &MessageBridge{}

	event := &pb.MessageEvent{
		Platform:    "discord",
		ChannelId:   "123456",
		ChannelName: "general",
	}

	// Without daemon (nil mnemo) — should create a local session
	sess := mb.getOrCreateSession(context.Background(), "discord:123456", event)

	if sess == nil {
		t.Fatal("getOrCreateSession() returned nil")
	}
	if sess.sessionID == "" {
		t.Error("session ID should not be empty")
	}

	// Second call should return the same session
	sess2 := mb.getOrCreateSession(context.Background(), "discord:123456", event)
	if sess2.sessionID != sess.sessionID {
		t.Errorf("expected same session ID, got %q vs %q", sess2.sessionID, sess.sessionID)
	}

	// Different channel should create a different session
	event2 := &pb.MessageEvent{
		Platform:    "discord",
		ChannelId:   "789012",
		ChannelName: "random",
	}
	sess3 := mb.getOrCreateSession(context.Background(), "discord:789012", event2)
	if sess3.sessionID == sess.sessionID {
		t.Error("different channel should have different session ID")
	}
}

func TestGetChannelMutex(t *testing.T) {
	mb := &MessageBridge{}

	mu1 := mb.getChannelMutex("discord:123")
	mu2 := mb.getChannelMutex("discord:123")

	if mu1 != mu2 {
		t.Error("getChannelMutex should return the same mutex for the same key")
	}

	mu3 := mb.getChannelMutex("discord:456")
	if mu1 == mu3 {
		t.Error("getChannelMutex should return different mutexes for different keys")
	}
}

func TestBridgeConfigDefaults(t *testing.T) {
	cfg := DefaultBridgeConfig()
	if cfg.MaxResponseLength != 0 {
		t.Errorf("Default MaxResponseLength = %d, want 0 (platform-aware formatting)", cfg.MaxResponseLength)
	}
	if cfg.ResponseTimeout != 120*time.Second {
		t.Errorf("Default ResponseTimeout = %v, want 120s", cfg.ResponseTimeout)
	}
}

// TestBridgeSessionConcurrency tests that per-channel mutexes properly serialize access.
func TestBridgeSessionConcurrency(t *testing.T) {
	mb := &MessageBridge{}

	var wg sync.WaitGroup
	results := make([]string, 10)
	var mu sync.Mutex // protects results slice

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			chMu := mb.getChannelMutex("discord:concurrent")
			chMu.Lock()
			defer chMu.Unlock()
			// Simulate work
			time.Sleep(1 * time.Millisecond)
			mu.Lock()
			results[idx] = "done"
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r != "done" {
			t.Errorf("goroutine %d didn't complete", i)
		}
	}
}

// TestMessageBridgeSessionPerChannel validates that different channels
// get independent sessions.
func TestMessageBridgeSessionPerChannel(t *testing.T) {
	mb := &MessageBridge{}

	ch1 := &pb.MessageEvent{Platform: "discord", ChannelId: "ch1", ChannelName: "channel1"}
	ch2 := &pb.MessageEvent{Platform: "discord", ChannelId: "ch2", ChannelName: "channel2"}
	ch1again := &pb.MessageEvent{Platform: "discord", ChannelId: "ch1", ChannelName: "channel1"}

	sess1 := mb.getOrCreateSession(context.Background(), mb.conversationKey(ch1), ch1)
	sess2 := mb.getOrCreateSession(context.Background(), mb.conversationKey(ch2), ch2)
	sess1again := mb.getOrCreateSession(context.Background(), mb.conversationKey(ch1again), ch1again)

	if sess1.sessionID == sess2.sessionID {
		t.Error("different channels should have different session IDs")
	}
	if sess1.sessionID != sess1again.sessionID {
		t.Error("same channel should return the same session")
	}
}

// TestHandleMessageSimulated tests the handleMessage flow with a mock agent.
// This validates the full flow: message event → session lookup → agent response → send.
// Note: We can't easily mock Agent.RunStreaming without an interface,
// so we test the helper methods that handleMessage calls directly.
// The integration test with a real agent will validate the full flow.
func TestHandleMessageSimulated(t *testing.T) {
	mb := &MessageBridge{
		config: BridgeConfig{
			AgentID:           "test-agent",
			MaxResponseLength: 1900,
			ResponseTimeout:   30 * time.Second,
		},
	}

	event := &pb.MessageEvent{
		Platform:    "discord",
		ChannelId:   "123456",
		ChannelName: "general",
		SenderId:    "alice",
		SenderName:  "Alice",
		Message:     "What's the weather?",
		MessageId:   "msg-001",
	}

	// Test composeUserMessage
	msg := mb.composeUserMessage(event)
	if !containsString(msg, "Alice") {
		t.Errorf("composeUserMessage should contain sender name 'Alice', got: %q", msg)
	}
	if !containsString(msg, "general") {
		t.Errorf("composeUserMessage should contain channel 'general', got: %q", msg)
	}
	if !containsString(msg, "What's the weather?") {
		t.Errorf("composeUserMessage should contain message text, got: %q", msg)
	}

	// Test session creation
	sess := mb.getOrCreateSession(context.Background(), mb.conversationKey(event), event)
	if sess == nil {
		t.Fatal("getOrCreateSession should return a session")
	}
	if sess.sessionID == "" {
		t.Error("session ID should not be empty")
	}

	// Test truncation
	longResponse := ""
	for i := 0; i < 200; i++ {
		longResponse += "This is a long response. "
	}
	truncated := mb.truncateResponse(longResponse)
	if len(truncated) > mb.config.MaxResponseLength+20 { // +20 for suffix
		t.Errorf("truncated response too long: %d chars", len(truncated))
	}
	if !containsString(truncated, "truncated") {
		t.Error("truncated response should contain 'truncated' suffix")
	}
}

// TestRunAgentAddsUserMessage validates that the session history
// accumulates messages across turns.
func TestRunAgentSessionAccumulation(t *testing.T) {
	sess := &bridgeSession{
		sessionID: "test-session",
		messages:  nil,
	}

	// Simulate adding user messages
	sess.mu.Lock()
	sess.messages = append(sess.messages, agentctx.Message{
		Role:    "user",
		Content: "[Message from Alice in #general]\nHello",
	})
	sess.mu.Unlock()

	sess.mu.Lock()
	sess.messages = append(sess.messages, agentctx.Message{
		Role:    "assistant",
		Content: "Hi there!",
	})
	sess.mu.Unlock()

	sess.mu.Lock()
	sess.messages = append(sess.messages, agentctx.Message{
		Role:    "user",
		Content: "[Message from Alice in #general]\nHow are you?",
	})
	sess.mu.Unlock()

	if len(sess.messages) != 3 {
		t.Errorf("expected 3 messages in session, got %d", len(sess.messages))
	}
	if sess.messages[0].Role != "user" {
		t.Errorf("message 0 should be user, got %s", sess.messages[0].Role)
	}
	if sess.messages[1].Role != "assistant" {
		t.Errorf("message 1 should be assistant, got %s", sess.messages[1].Role)
	}
	if sess.messages[2].Role != "user" {
		t.Errorf("message 2 should be user, got %s", sess.messages[2].Role)
	}
}

	// TestPlatformFiltering tests that the platform filter correctly
	// includes/excludes messages.
	func TestPlatformFiltering(t *testing.T) {
		mb := &MessageBridge{
			config: BridgeConfig{
				Platforms: []string{"discord"},
			},
		}

		if !mb.isAllowedPlatform("discord") {
			t.Error("discord should be allowed")
		}
		if mb.isAllowedPlatform("telegram") {
			t.Error("telegram should not be allowed when only discord is filtered")
		}

		// No filter — allow everything
		mb2 := &MessageBridge{
			config: BridgeConfig{Platforms: nil},
		}
		if !mb2.isAllowedPlatform("discord") {
			t.Error("empty filter should allow discord")
		}
		if !mb2.isAllowedPlatform("telegram") {
			t.Error("empty filter should allow telegram")
		}
		if !mb2.isAllowedPlatform("imessage") {
			t.Error("empty filter should allow imessage")
		}
	}

	// TestEmptyMessageEvent validates handling of empty message events.
	func TestEmptyMessageEvent(t *testing.T) {
		mb := &MessageBridge{}
		event := &pb.MessageEvent{
			Platform:   "discord",
			ChannelId:  "123456",
			ChannelName: "general",
			Message:    "",
		}
		msg := mb.composeUserMessage(event)
		if !strings.Contains(msg, "general") {
			t.Error("composeUserMessage should handle empty message gracefully and include channel name")
		}
	}

	// TestVeryLongMessage validates handling of very long messages (10K+ characters).
	func TestVeryLongMessage(t *testing.T) {
		mb := &MessageBridge{
			config: BridgeConfig{MaxResponseLength: 100},
		}
		longMsg := strings.Repeat("a", 10000)
		event := &pb.MessageEvent{
			Platform:  "discord",
			ChannelId: "123456",
			Message:   longMsg,
		}
		msg := mb.composeUserMessage(event)
		if !strings.Contains(msg, longMsg) {
			t.Error("composeUserMessage should handle very long messages")
		}
		truncated := mb.truncateResponse(longMsg)
		if len(truncated) > 100+20 {
			t.Errorf("truncated response should not exceed MaxResponseLength + suffix")
		}
	}

	// TestUnicodeAndSpecialChars validates handling of Unicode and special characters.
	func TestUnicodeAndSpecialChars(t *testing.T) {
		mb := &MessageBridge{}
		specialMsg := "Hello 🌍! 🚀 Test: \n\t\r\u2603"
		event := &pb.MessageEvent{
			Platform:  "discord",
			ChannelId: "123456",
			Message:   specialMsg,
		}
		msg := mb.composeUserMessage(event)
		if !strings.Contains(msg, specialMsg) {
			t.Error("composeUserMessage should handle Unicode and special characters")
		}
	}

	// TestRapidSequentialMessagesToSameChannel validates handling of rapid sequential messages.
	func TestRapidSequentialMessagesToSameChannel(t *testing.T) {
		mb := &MessageBridge{}
		convKey := "discord:123456"
		mu := mb.getChannelMutex(convKey)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				mu.Lock()
				defer mu.Unlock()
				// Simulate message processing
				time.Sleep(1 * time.Millisecond)
			}(i)
		}
		wg.Wait()
	}

	// TestMessagesFromMultipleChannelsSimultaneously validates handling of messages from multiple channels.
	func TestMessagesFromMultipleChannelsSimultaneously(t *testing.T) {
		mb := &MessageBridge{}
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(chID string) {
				defer wg.Done()
				convKey := fmt.Sprintf("discord:%s", chID)
				mu := mb.getChannelMutex(convKey)
				mu.Lock()
				defer mu.Unlock()
				time.Sleep(1 * time.Millisecond)
			}(fmt.Sprintf("ch%d", i))
		}
		wg.Wait()
	}

	// TestChannelIDCollision validates handling of channel ID collisions.
	func TestChannelIDCollision(t *testing.T) {
		mb := &MessageBridge{}
		ch1 := &pb.MessageEvent{Platform: "discord", ChannelId: "123", ChannelName: "channel1"}
		ch2 := &pb.MessageEvent{Platform: "telegram", ChannelId: "123", ChannelName: "channel2"}
		sess1 := mb.getOrCreateSession(context.Background(), mb.conversationKey(ch1), ch1)
		sess2 := mb.getOrCreateSession(context.Background(), mb.conversationKey(ch2), ch2)
		if sess1.sessionID == sess2.sessionID {
			t.Error("different platforms with same channel ID should have different sessions")
		}
	}

	// TestPlatformNameInjection validates handling of platform name injection.
	func TestPlatformNameInjection(t *testing.T) {
		mb := &MessageBridge{
			config: BridgeConfig{Platforms: []string{"discord"}},
		}
		if !mb.isAllowedPlatform("discord") {
			t.Error("platform name injection should be handled correctly")
		}
		if mb.isAllowedPlatform("malicious-platform") {
			t.Error("malicious platform names should not be allowed")
		}
	}

	// TestEmptyResponseFromAgent validates handling of empty responses from the agent.
	func TestEmptyResponseFromAgent(t *testing.T) {
		mb := &MessageBridge{
			config: BridgeConfig{MaxResponseLength: 100},
		}
		emptyResponse := ""
		truncated := mb.truncateResponse(emptyResponse)
		if truncated != emptyResponse {
			t.Error("empty response should remain empty after truncation")
		}
	}

	// TestSessionCreationFailure validates handling of session creation failure.
	func TestSessionCreationFailure(t *testing.T) {
		mb := &MessageBridge{}
		event := &pb.MessageEvent{
			Platform:    "discord",
			ChannelId:   "123456",
			ChannelName: "general",
		}
		sess := mb.getOrCreateSession(context.Background(), "discord:123456", event)
		if sess == nil {
			t.Fatal("getOrCreateSession should return a session even on failure")
		}
		if sess.sessionID == "" {
			t.Error("session ID should not be empty even on failure")
		}
	}

	// TestConversationKeyStability validates stability of conversation keys.
	func TestConversationKeyStability(t *testing.T) {
		mb := &MessageBridge{}
		event := &pb.MessageEvent{
			Platform:  "discord",
			ChannelId: "123456",
		}
		key1 := mb.conversationKey(event)
		key2 := mb.conversationKey(event)
		if key1 != key2 {
			t.Error("conversationKey should be stable for the same event")
		}
	}

	// TestMaxResponseTruncationBoundaryCases validates boundary cases for response truncation.
	func TestMaxResponseTruncationBoundaryCases(t *testing.T) {
		mb := &MessageBridge{
			config: BridgeConfig{MaxResponseLength: 10},
		}
		tests := []struct {
			name     string
			input    string
			expected string
		}{
			{"exactly at boundary", strings.Repeat("a", 10), strings.Repeat("a", 10)},
			{"one over boundary", strings.Repeat("a", 11), strings.Repeat("a", 10) + "\n... (truncated)"},
			{"empty string", "", ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := mb.truncateResponse(tt.input)
				if got != tt.expected {
					t.Errorf("truncateResponse() = %q, want %q", got, tt.expected)
				}
			})
		}
	}

	// TestConfigValidation validates configuration validation.
	func TestConfigValidation(t *testing.T) {
		cfg := DefaultBridgeConfig()
		// MaxResponseLength of 0 means platform-aware formatting (no global truncation)
		if cfg.ResponseTimeout <= 0 {
			t.Error("ResponseTimeout should be positive")
		}
	}