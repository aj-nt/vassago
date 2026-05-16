// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package messaging

import (
	"context"
	"fmt"
	"log"
	"sync"

	pb "github.com/aj-nt/vassago-sdk/proto"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// telegramAPI defines the interface for Telegram Bot API operations.
// This abstraction allows mocking in tests — tgbotapi.BotAPI is a concrete
// struct and cannot be mocked directly.
type telegramAPI interface {
	GetUpdatesChan(config tgbotapi.UpdateConfig) (tgbotapi.UpdatesChannel, error)
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	StopReceivingUpdates()
}

// telegramAPIWrapper wraps a real tgbotapi.BotAPI to implement the telegramAPI interface.
type telegramAPIWrapper struct {
	bot *tgbotapi.BotAPI
}

func (w *telegramAPIWrapper) GetUpdatesChan(config tgbotapi.UpdateConfig) (tgbotapi.UpdatesChannel, error) {
	return w.bot.GetUpdatesChan(config), nil
}

func (w *telegramAPIWrapper) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	return w.bot.Send(c)
}

func (w *telegramAPIWrapper) StopReceivingUpdates() {
	w.bot.StopReceivingUpdates()
}

// TelegramConfig holds Telegram adapter configuration.
type TelegramConfig struct {
	Token        string
	AllowedChats []int64 // If empty, all chats are allowed
}

// TelegramAdapter implements MessagingAdapter for Telegram.
type TelegramAdapter struct {
	config      TelegramConfig
	api         telegramAPI
	botID       int64
	messageChan chan *pb.MessageEvent
	closeChan   chan struct{}
	mu          sync.Mutex
}

// PlatformName returns the platform identifier for Telegram.
func (t *TelegramAdapter) PlatformName() string {
	return "telegram"
}

// NewTelegramAdapter creates a new Telegram adapter.
func NewTelegramAdapter(config TelegramConfig) *TelegramAdapter {
	return &TelegramAdapter{
		config:      config,
		messageChan: make(chan *pb.MessageEvent, 100),
		closeChan:   make(chan struct{}),
	}
}

// setAPI injects a telegramAPI (for testing).
func (ta *TelegramAdapter) setAPI(api telegramAPI, botID int64) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.api = api
	ta.botID = botID
}

// Connect establishes connection to Telegram.
func (ta *TelegramAdapter) Connect(ctx context.Context) error {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if ta.api != nil {
		return nil // Already connected (or injected by test)
	}

	bot, err := tgbotapi.NewBotAPI(ta.config.Token)
	if err != nil {
		return fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	bot.Debug = false
	ta.api = &telegramAPIWrapper{bot: bot}
	ta.botID = int64(bot.Self.ID)

	log.Printf("Telegram adapter connected as @%s (ID: %d)", bot.Self.UserName, bot.Self.ID)
	return nil
}

// Listen starts listening for incoming messages via long-polling.
// Returns a channel that emits MessageEvent for each incoming message.
func (ta *TelegramAdapter) Listen(ctx context.Context) (<-chan *pb.MessageEvent, error) {
	if ta.api == nil {
		return nil, fmt.Errorf("not connected to Telegram")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := ta.api.GetUpdatesChan(u)
	if err != nil {
		return nil, fmt.Errorf("failed to get updates channel: %w", err)
	}

	go ta.processUpdates(ctx, updates)

	return ta.messageChan, nil
}

// processUpdates reads from the Telegram updates channel and forwards
// messages to the internal messageChan.
func (ta *TelegramAdapter) processUpdates(ctx context.Context, updates tgbotapi.UpdatesChannel) {
	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}

			if update.Message == nil || update.Message.From == nil || update.Message.Chat == nil {
				continue
			}

			// Ignore non-text messages (photos, stickers, etc.) — nothing useful for the agent
			if update.Message.Text == "" {
				continue
			}

			// Filter out messages from the bot itself
			if int64(update.Message.From.ID) == ta.botID {
				continue
			}

			// Filter by allowed chats if specified
			if len(ta.config.AllowedChats) > 0 {
				allowed := false
				for _, chatID := range ta.config.AllowedChats {
					if update.Message.Chat.ID == chatID {
						allowed = true
						break
					}
				}
				if !allowed {
					continue
				}
			}

			// Convert Telegram message to proto.MessageEvent
			chatIDStr := fmt.Sprintf("%d", update.Message.Chat.ID)
			channelName := update.Message.Chat.Title
			if channelName == "" {
				channelName = chatIDStr
			}

			senderName := update.Message.From.UserName
			if senderName == "" {
				senderName = update.Message.From.FirstName
			}

			event := &pb.MessageEvent{
				Platform:    "telegram",
				ChannelId:   chatIDStr,
				ChannelName: channelName,
				SenderId:    fmt.Sprintf("%d", update.Message.From.ID),
				SenderName:  senderName,
				Message:     update.Message.Text,
				Timestamp:   update.Message.Time().Unix(),
				MessageId:   fmt.Sprintf("%d", update.Message.MessageID),
			}

			select {
			case ta.messageChan <- event:
			case <-ta.closeChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// Send sends a message to a Telegram chat.
func (ta *TelegramAdapter) Send(ctx context.Context, target, message string) (string, error) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if ta.api == nil {
		return "", fmt.Errorf("not connected to Telegram")
	}

	// Parse target as chat ID (Telegram uses int64 chat IDs)
	var chatID int64
	if _, err := fmt.Sscanf(target, "%d", &chatID); err != nil {
		return "", fmt.Errorf("invalid Telegram chat ID: %s: %w", target, err)
	}

	// Split message if it exceeds Telegram's 4096 character limit
	if len(message) > 4096 {
		return ta.sendSplitMessage(chatID, message)
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeHTML

	sentMsg, err := ta.api.Send(msg)
	if err != nil {
		return "", fmt.Errorf("failed to send Telegram message: %w", err)
	}

	return fmt.Sprintf("%d", sentMsg.MessageID), nil
}

// sendSplitMessage handles messages longer than 4096 characters.
func (ta *TelegramAdapter) sendSplitMessage(chatID int64, message string) (string, error) {
	chunks := SplitMessageForPlatform("telegram", message)

	var lastMsgID string
	for i, chunk := range chunks {
		// SplitMessageForPlatform already adds "(continued) " prefix
		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeHTML

		sentMsg, err := ta.api.Send(msg)
		if err != nil {
			return "", fmt.Errorf("failed to send Telegram message chunk %d: %w", i, err)
		}
		lastMsgID = fmt.Sprintf("%d", sentMsg.MessageID)
	}

	return lastMsgID, nil
}

// Close disconnects from Telegram.
func (ta *TelegramAdapter) Close(ctx context.Context) error {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	if ta.api == nil {
		return nil
	}

	ta.api.StopReceivingUpdates()
	close(ta.closeChan)
	ta.api = nil
	return nil
}

// ListChannels returns available Telegram chats.
// Telegram doesn't provide a direct API to list all chats,
// so we return an empty list. The daemon can track active chats
// from incoming messages.
func (ta *TelegramAdapter) ListChannels(ctx context.Context) ([]*pb.Channel, error) {
	return []*pb.Channel{}, nil
}
