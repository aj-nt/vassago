// Package messaging provides platform-agnostic messaging adapters
package messaging

import (
	"context"
	"github.com/aj-nt/mnemosyne/proto"
)

// MessagingAdapter defines the interface for platform-agnostic messaging
// Implementations: DiscordAdapter, TelegramAdapter, etc.
type MessagingAdapter interface {
	// Connect establishes connection to the messaging platform
	Connect(ctx context.Context) error

	// Listen starts listening for incoming messages
	// Returns a channel that emits MessageEvent
	Listen(ctx context.Context) (<-chan *proto.MessageEvent, error)

	// Send sends a message to a target channel/user
	Send(ctx context.Context, target, message string) (string, error)

	// Close disconnects from the platform
	Close(ctx context.Context) error

	// ListChannels returns available channels for the platform
	ListChannels(ctx context.Context) ([]*proto.Channel, error)
}

// MessageHandler processes incoming messages
// Implementations should call this for each received message
type MessageHandler func(ctx context.Context, event *proto.MessageEvent) error
