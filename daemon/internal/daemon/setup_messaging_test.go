// This file is part of Vassago.
// See LICENSE for license information.

package daemon

import (
	"testing"

	"github.com/aj-nt/vassago/daemon/internal/grpc"
	"github.com/stretchr/testify/assert"
)

func TestSetupMessagingAdapters_DiscordTokenRegistersAdapter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Messaging.Discord.Token = "test-discord-token"

	svc := grpc.NewMessagingService()
	setupMessagingAdapters(cfg, svc)

	status := svc.Status()
	assert.Contains(t, status, "discord", "discord adapter should be registered")
	assert.False(t, status["discord"], "discord adapter should not be started (not connected)")
}

func TestSetupMessagingAdapters_NoTokenSkipsAdapter(t *testing.T) {
	cfg := DefaultConfig()
	// No token configured for any platform

	svc := grpc.NewMessagingService()
	setupMessagingAdapters(cfg, svc)

	status := svc.Status()
	assert.NotContains(t, status, "discord", "discord should not be registered without token")
	assert.NotContains(t, status, "telegram", "telegram should not be registered without token")
}

func TestSetupMessagingAdapters_TelegramTokenRegistersAdapter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Messaging.Telegram.Token = "test-telegram-token"

	svc := grpc.NewMessagingService()
	setupMessagingAdapters(cfg, svc)

	status := svc.Status()
	assert.Contains(t, status, "telegram", "telegram adapter should be registered")
}
