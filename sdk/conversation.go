// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package sdk

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/aj-nt/vassago-sdk/client"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// MessageHandler is called for each incoming platform message. The handler
// receives the full message event and must return the response text to send
// back. Return an empty string to not respond.
//
// The handler is called serially per-channel (not per-message), so session
// state stored via the ConversationManager remains coherent.
type MessageHandler func(ctx context.Context, event *pb.MessageEvent, session *Conversation) (response string, err error)

// Conversation holds session state for a single platform channel.
type Conversation struct {
	// SessionID is the daemon session identifier for persistence.
	SessionID string

	// Messages is the conversation history (user + assistant turns).
	Messages []client.Message

	// Metadata is free-form storage for the handler's use.
	Metadata map[string]interface{}
}

// platformKey builds a stable key for a platform + channel combination.
func platformKey(platform, channelID string) string {
	return platform + ":" + channelID
}

// ConversationManager receives messages from the daemon's messaging
// adapters and routes them through a user-provided handler. It maintains
// per-channel sessions so conversations stay coherent across turns.
//
// Usage:
//
//	store, _ := sdk.Connect(ctx, cfg)
//	cm := sdk.NewConversationManager(store, myHandler)
//	cm.Run(ctx) // blocks until ctx cancelled
type ConversationManager struct {
	store   messageStore
	handler MessageHandler

	// Per-channel session state
	sessions sync.Map // platform:channelID → *Conversation

	// Per-channel mutex to serialize message processing
	chanMu sync.Map // platform:channelID → *sync.Mutex

	// Config
	responseTimeout time.Duration
	maxMsgChars     int
}

// messageStore is the subset of MemoryStore needed by handleMessage and helpers,
// plus the streaming subscription used by Run().
// Both *MemoryStore (gRPC-backed) and *MemStore (in-memory/test) satisfy this interface.
type messageStore interface {
	CreateSession(ctx context.Context, source, title string) (string, error)
	AppendMessages(ctx context.Context, sessionID string, messages []client.Message) error
	SendMessage(ctx context.Context, platform, target, message string) (string, error)
	AgentName() string
	SubscribeToMessages(ctx context.Context) (pb.Messaging_SubscribeToMessagesClient, error)
}

// ConversationManagerConfig holds optional settings.
type ConversationManagerConfig struct {
	// ResponseTimeout is the max time the handler has to produce a response.
	// Defaults to 120s if zero.
	ResponseTimeout time.Duration

	// MaxMessageChars is the maximum incoming message length to process.
	// Messages longer than this are truncated. 0 means no limit.
	MaxMessageChars int
}

// NewConversationManager creates a ConversationManager that routes incoming
// platform messages through the provided handler.
func NewConversationManager(store messageStore, handler MessageHandler, cfg ConversationManagerConfig) *ConversationManager {
	if cfg.ResponseTimeout <= 0 {
		cfg.ResponseTimeout = 120 * time.Second
	}
	return &ConversationManager{
		store:           store,
		handler:         handler,
		responseTimeout: cfg.ResponseTimeout,
		maxMsgChars:     cfg.MaxMessageChars,
	}
}

// Run starts listening for incoming messages from all configured platform
// adapters. It blocks until ctx is cancelled or the stream fails permanently.
// Automatically reconnects with exponential backoff on transient failures.
func (cm *ConversationManager) Run(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := cm.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			slog.Error("ConversationManager: stream error, reconnecting", "error", err)
		}

		slog.Info("ConversationManager: reconnecting", "backoff", backoff)
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

// runOnce opens a subscription stream and processes messages until failure.
func (cm *ConversationManager) runOnce(ctx context.Context) error {
	stream, err := cm.store.SubscribeToMessages(ctx)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	defer stream.CloseSend()

	slog.Info("ConversationManager: subscribed to messages", "agent", cm.store.AgentName())

	for {
		event, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		// Process each message in a goroutine so we don't block the stream.
		go cm.handleMessage(event)
	}
}

// handleMessage processes a single incoming message event.
func (cm *ConversationManager) handleMessage(event *pb.MessageEvent) {
	if event.Message == "" {
		return
	}
	if cm.maxMsgChars > 0 && len(event.Message) > cm.maxMsgChars {
		event.Message = event.Message[:cm.maxMsgChars]
	}

	key := platformKey(event.Platform, event.ChannelId)

	// Serialize per-channel: only one message at a time.
	mu := cm.getChannelMutex(key)
	mu.Lock()
	defer mu.Unlock()

	// Get or create conversation state.
	conv := cm.getOrCreateConversation(key, event)

	// Build the user message for session history.
	userMsg := ComposePlatformContext(event.Platform, event.ChannelName, event.SenderName) + "\n" + event.Message

	ctx, cancel := context.WithTimeout(context.Background(), cm.responseTimeout)
	defer cancel()

	// Let the handler process it.
	response, err := cm.handler(ctx, event, conv)
	if err != nil {
		slog.Error("ConversationManager: handler error",
			"platform", event.Platform, "channel", event.ChannelName, "error", err)
		response = fmt.Sprintf("Sorry, I encountered an error: %v", err)
	}

	// Persist the turn to session history.
	conv.Messages = append(conv.Messages,
		client.Message{Role: "user", Content: userMsg},
	)
	if response != "" {
		conv.Messages = append(conv.Messages,
			client.Message{Role: "assistant", Content: response},
		)
		// Fire-and-forget persist to daemon.
		go cm.persistSession(key, conv)

		// Send response back to the platform.
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer sendCancel()
		msgID, err := cm.store.SendMessage(sendCtx, event.Platform, event.ChannelId, response)
		if err != nil {
			slog.Error("ConversationManager: failed to send response",
				"platform", event.Platform, "channel", event.ChannelId, "error", err)
		} else {
			slog.Info("ConversationManager: response sent",
				"platform", event.Platform, "channel", event.ChannelName,
				"msgID", msgID, "chars", len(response))
		}
	}
}

// getOrCreateConversation returns existing session state or creates one.
func (cm *ConversationManager) getOrCreateConversation(key string, event *pb.MessageEvent) *Conversation {
	if existing, ok := cm.sessions.Load(key); ok {
		return existing.(*Conversation)
	}

	// Create a daemon session for persistence.
	sessionID := fmt.Sprintf("local-%s", key)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if id, err := cm.store.CreateSession(ctx, "messaging",
		fmt.Sprintf("%s:%s", event.Platform, event.ChannelName)); err == nil {
		sessionID = id
	}

	conv := &Conversation{
		SessionID: sessionID,
		Metadata:  make(map[string]interface{}),
	}
	cm.sessions.Store(key, conv)
	return conv
}

// persistSession saves the current messages to the daemon.
func (cm *ConversationManager) persistSession(key string, conv *Conversation) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cm.store.AppendMessages(ctx, conv.SessionID, conv.Messages); err != nil {
		slog.Warn("ConversationManager: persist failed",
			"key", key, "session", conv.SessionID, "error", err)
	}
}

// getChannelMutex returns the mutex for a given channel key.
func (cm *ConversationManager) getChannelMutex(key string) *sync.Mutex {
	newMu := &sync.Mutex{}
	actual, _ := cm.chanMu.LoadOrStore(key, newMu)
	return actual.(*sync.Mutex)
}
