// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

// Package messaging provides platform-agnostic messaging adapters.
//
// Message formatting and platform limits are defined in the shared
// messaging/format package so both agent and daemon can use them
// without circular imports.
package messaging

import (
	"context"

	"github.com/aj-nt/vassago-sdk/messaging/format"
	pb "github.com/aj-nt/vassago-sdk/proto"
)

// Re-export format package functions for convenience within this package.
var (
	// GetPlatformLimits returns the limits for a platform, with a safe default.
	GetPlatformLimits = format.GetPlatformLimits

	// ValidatePlatform checks if a platform is supported.
	ValidatePlatform = format.ValidatePlatform

	// ValidateEmailTarget validates an email address.
	ValidateEmailTarget = format.ValidateEmailTarget

	// ComposePlatformContext returns a platform-aware context label.
	ComposePlatformContext = format.ComposePlatformContext

	// FormatMessageForPlatform formats a response message for a specific platform.
	FormatMessageForPlatform = format.FormatMessageForPlatform

	// SplitMessageForPlatform splits a message into chunks appropriate for the platform.
	SplitMessageForPlatform = format.SplitMessageForPlatform
)

// MessagingAdapter defines the interface for platform-agnostic messaging.
// Implementations: DiscordAdapter, TelegramAdapter, IMessageAdapter, EmailAdapter.
type MessagingAdapter interface {
	// PlatformName returns the adapter's platform identifier (e.g., "discord", "telegram").
	PlatformName() string

	// Connect establishes connection to the messaging platform
	Connect(ctx context.Context) error

	// Listen starts listening for incoming messages
	// Returns a channel that emits MessageEvent
	Listen(ctx context.Context) (<-chan *pb.MessageEvent, error)

	// Send sends a message to a target channel/user
	Send(ctx context.Context, target, message string) (string, error)

	// Close disconnects from the platform
	Close(ctx context.Context) error

	// ListChannels returns available channels for the platform
	ListChannels(ctx context.Context) ([]*pb.Channel, error)
}

// MessageHandler processes incoming messages
// Implementations should call this for each received message
type MessageHandler func(ctx context.Context, event *pb.MessageEvent) error
