// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

// Package iface provides the agent's user interfaces (CLI, gRPC chat, messaging).
package iface

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aj-nt/mnemosyne/agent"
	agentlog "github.com/aj-nt/mnemosyne/agent/log"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	memoclient "github.com/aj-nt/mnemosyne/client"
	"github.com/aj-nt/mnemosyne/messaging/format"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// MessageBridge subscribes to incoming messages from the daemon's messaging
// service and routes them through the agent, sending responses back to the
// originating platform. It owns per-channel session management and handles
// stream reconnection on failures.
//
// Architecture:
//
//	Discord → daemon SubscribeToMessages → MessageBridge → Agent.RunStreaming
//	→ text response → daemon SendMessage → Discord
type MessageBridge struct {
	agent      *agent.Agent
	mnemo memoclient.MnemoClient
	config     BridgeConfig

	// Per-channel session state. Key = "platform:channelID"
	sessions sync.Map // string -> *bridgeSession

	// mu serializes concurrent agent invocations for the same channel.
	// A single channel should only process one message at a time so the
	// conversation context stays coherent.
	chanMu sync.Map // string -> *sync.Mutex

	// cancel stops the subscription loop
	cancel context.CancelFunc
}

// BridgeConfig holds configuration for the MessageBridge.
type BridgeConfig struct {
	// AgentID is used as the agent identifier for sessions and messages.
	AgentID string

	// Platforms filters which platforms to handle. Empty means all.
	Platforms []string

	// MaxResponseLength is the maximum response text length before truncation.
	// Messages exceeding this are truncated with a "... (truncated)" suffix.
	MaxResponseLength int

	// ResponseTimeout is the maximum time to wait for the agent to respond.
	// 0 means no timeout.
	ResponseTimeout time.Duration
}

// bridgeSession holds state for a single conversation across multiple turns.
type bridgeSession struct {
	sessionID string
	messages  []agentctx.Message
	mu        sync.Mutex
}

// DefaultBridgeConfig returns sensible defaults for a BridgeConfig.
// MaxResponseLength is 0 (no truncation) by default because formatting
// is now platform-aware — each platform has its own limits.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		MaxResponseLength: 0, // 0 = no truncation; platform limits apply
		ResponseTimeout:   120 * time.Second,
	}
}

// NewMessageBridge creates a new MessageBridge.
func NewMessageBridge(a *agent.Agent, mnemo memoclient.MnemoClient, cfg BridgeConfig) *MessageBridge {
	return &MessageBridge{
		agent:  a,
		mnemo:  mnemo,
		config: cfg,
	}
}

// Start begins subscribing to incoming messages. It blocks until the context
// is cancelled or a fatal error occurs. It automatically reconnects on
// stream failures with exponential backoff.
func (mb *MessageBridge) Start(ctx context.Context) error {
	ctx, mb.cancel = context.WithCancel(ctx)
	defer mb.cancel()

	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := mb.subscribe(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			agentlog.Printf("MessageBridge: subscription error: %v", err)
		}

		// Exponential backoff on reconnect (1s, 2s, 4s, 8s, 16s, 30s, 30s, ...)
		agentlog.Printf("MessageBridge: reconnecting in %v...", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// Stop cancels the subscription loop.
func (mb *MessageBridge) Stop() {
	if mb.cancel != nil {
		mb.cancel()
	}
}

// subscribe opens a gRPC stream to the daemon's SubscribeToMessages and
// processes each incoming message.
func (mb *MessageBridge) subscribe(ctx context.Context) error {
	// SubscribeToMessages is on ClientInterface, not MnemoClient.
	// If connected to a real daemon, use it; otherwise, skip messaging.
	realClient, ok := mb.mnemo.(*memoclient.Client)
	if !ok {
		return fmt.Errorf("subscribe: messaging requires daemon connection")
	}
	stream, err := realClient.SubscribeToMessages(ctx, mb.config.AgentID)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	defer stream.CloseSend()

	agentlog.Printf("MessageBridge: subscribed to incoming messages (agent=%s)", mb.config.AgentID)

	for {
		event, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("stream recv: %w", err)
		}

		// Filter by platform if configured
		if !mb.isAllowedPlatform(event.Platform) {
			continue
		}

		// Process the message — don't block the stream, run in a goroutine
		// so we can receive more messages while processing this one.
		go mb.handleMessage(event)
	}
}

// handleMessage processes a single incoming message event.
func (mb *MessageBridge) handleMessage(event *pb.MessageEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), mb.config.ResponseTimeout)
	defer cancel()

	// Build a conversation key from platform + channel
	convKey := mb.conversationKey(event)

	// Serialize per-channel: only one message processed at a time per channel
	mu := mb.getChannelMutex(convKey)
	mu.Lock()
	defer mu.Unlock()

	// Get or create session for this conversation
	sess := mb.getOrCreateSession(ctx, convKey, event)

	// Compose the user message with context prefix
	userMsg := mb.composeUserMessage(event)

	// Run the agent and collect the response
	responseText, err := mb.runAgent(ctx, sess, userMsg)
	if err != nil {
		agentlog.Printf("MessageBridge: agent error for %s: %v", convKey, err)
		// Still try to send a brief error message
		responseText = fmt.Sprintf("Sorry, I encountered an error processing your message. (%v)", err)
	}

	// Truncate if needed
	responseText = mb.truncateResponse(responseText)

	// Send the response back via the messaging service
	if mb.mnemo != nil {
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer sendCancel()

		msgID, err := mb.mnemo.SendMessage(sendCtx, event.Platform, event.ChannelId, responseText)
		if err != nil {
			agentlog.Printf("MessageBridge: failed to send response to %s/%s: %v", event.Platform, event.ChannelId, err)
		} else {
			agentlog.Printf("MessageBridge: sent response to %s/%s (msgID=%s, %d chars)", event.Platform, event.ChannelId, msgID, len(responseText))
		}
	}
}

// runAgent invokes the agent on a message and collects the full text response.
func (mb *MessageBridge) runAgent(ctx context.Context, sess *bridgeSession, userMsg string) (string, error) {
	sess.mu.Lock()
	// Add user message to session history
	sess.messages = append(sess.messages, agentctx.Message{
		Role:    "user",
		Content: userMsg,
	})
	// Copy messages for this turn
	messages := make([]agentctx.Message, len(sess.messages))
	copy(messages, sess.messages)
	sess.mu.Unlock()

	// Refresh hot block
	hotBlock, err := mb.agent.GetHotBlock(ctx)
	if err != nil {
		agentlog.Printf("MessageBridge: hot block error: %v", err)
		hotBlock = ""
	}

	// Build conversation context
	toolDefs := mb.agent.BuildToolDefinitions()
	skillBlock := mb.agent.GetSkillBlock(ctx)
	convCtx := &agentctx.ConversationContext{
		SessionID:       sess.sessionID,
		AgentID:         mb.config.AgentID,
		Messages:        messages,
		SystemPrompt:    "",
		Tools:           toolDefs,
		MaxIterations:   mb.agent.Config.MaxTurns,
		Iteration:       0,
		Interrupt:       agentctx.NewInterruptSignal(),
		HotBlockContent: hotBlock,
		SkillContent:    skillBlock,
		Temperature:     mb.agent.Config.Temperature,
	}

	// Auto-approve tool calls in messaging mode
	mb.agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
		return true
	}

	// Stream the agent response and collect text
	eventCh := make(chan agent.StreamEvent, 64)
	go mb.agent.RunStreaming(ctx, convCtx, eventCh)

	var responseText string
	for event := range eventCh {
		switch event.Type {
		case agentctx.StreamText:
			responseText += event.Text
		case agentctx.StreamError:
			if responseText == "" {
				return "", event.Error
			}
			// If we already have text, log but don't discard it
			agentlog.Printf("MessageBridge: stream error after partial response: %v", event.Error)
		case agentctx.StreamDone:
			// Turn complete
		}
	}

	// Update session with agent's response + any tool messages
	sess.mu.Lock()
	sess.messages = convCtx.Messages
	sess.mu.Unlock()

	return responseText, nil
}

// composeUserMessage builds the user-facing message text from the event.
// It uses platform-aware context labels so the agent knows where the
// message came from.
func (mb *MessageBridge) composeUserMessage(event *pb.MessageEvent) string {
	return format.ComposePlatformContext(event.Platform, event.ChannelName, event.SenderName) + "\n" + event.Message
}

// getOrCreateSession returns an existing session for the conversation key,
// or creates a new one (including on the daemon).
func (mb *MessageBridge) getOrCreateSession(ctx context.Context, convKey string, event *pb.MessageEvent) *bridgeSession {
	if existing, ok := mb.sessions.Load(convKey); ok {
		return existing.(*bridgeSession)
	}

	// Create a new session on the daemon
	var sessionID string
	if mb.mnemo != nil {
		// Use a descriptive title: "discord:channel_name"
		title := fmt.Sprintf("%s:%s", event.Platform, event.ChannelName)
		createCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		id, err := mb.mnemo.CreateSession(createCtx, mb.config.AgentID, "messaging", title)
		if err != nil {
			agentlog.Printf("MessageBridge: failed to create session for %s: %v, using local ID", convKey, err)
			sessionID = fmt.Sprintf("local-%s", convKey)
		} else {
			sessionID = id
		}
	} else {
		sessionID = fmt.Sprintf("local-%s", convKey)
	}

	sess := &bridgeSession{
		sessionID: sessionID,
		messages:  nil,
	}
	mb.sessions.Store(convKey, sess)
	return sess
}

// conversationKey produces a stable key for session grouping.
// Per-channel sessions (not per-user) so the agent maintains conversation
// context across all participants in a channel.
func (mb *MessageBridge) conversationKey(event *pb.MessageEvent) string {
	return fmt.Sprintf("%s:%s", event.Platform, event.ChannelId)
}

// getChannelMutex returns a mutex for serializing access to a single channel.
func (mb *MessageBridge) getChannelMutex(convKey string) *sync.Mutex {
	newMu := &sync.Mutex{}
	actual, _ := mb.chanMu.LoadOrStore(convKey, newMu)
	return actual.(*sync.Mutex)
}

// isAllowedPlatform checks if the platform is in the allowed list.
func (mb *MessageBridge) isAllowedPlatform(platform string) bool {
	if len(mb.config.Platforms) == 0 {
		return true
	}
	for _, p := range mb.config.Platforms {
		if p == platform {
			return true
		}
	}
	return false
}

// truncateResponse truncates a response that exceeds MaxResponseLength.
func (mb *MessageBridge) truncateResponse(text string) string {
	if mb.config.MaxResponseLength <= 0 || len(text) <= mb.config.MaxResponseLength {
		return text
	}
	return text[:mb.config.MaxResponseLength] + "\n... (truncated)"
}