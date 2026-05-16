// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package messaging

import (
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"

	"github.com/aj-nt/vassago-sdk/proto"
)

// DiscordConfig holds Discord adapter configuration.
type DiscordConfig struct {
	Token           string
	Guilds          []string
	AllowedChannels []string
}

// discordSession defines the interface for Discord session operations.
// This abstraction allows mocking in tests — discordgo.Session is a concrete
// struct and cannot be mocked directly.
type discordSession interface {
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	Guild(guildID string, options ...discordgo.RequestOption) (*discordgo.Guild, error)
	GuildChannels(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Channel, error)
	Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	Open() error
	Close() error
	AddHandler(handler interface{}) func()
}

// DiscordAdapter implements MessagingAdapter for Discord.
type DiscordAdapter struct {
	config      DiscordConfig
	session     discordSession
	messageChan chan *proto.MessageEvent
	closeChan   chan struct{}
	mu          sync.Mutex
}

// PlatformName returns the platform identifier for Discord.
func (d *DiscordAdapter) PlatformName() string {
	return "discord"
}

// NewDiscordAdapter creates a new Discord adapter.
func NewDiscordAdapter(config DiscordConfig) *DiscordAdapter {
	return &DiscordAdapter{
		config:      config,
		messageChan: make(chan *proto.MessageEvent, 100),
		closeChan:   make(chan struct{}),
	}
}

// Connect establishes connection to Discord.
func (d *DiscordAdapter) Connect(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.session != nil {
		return nil // Already connected (or injected by test)
	}

	var err error
	s, err := discordgo.New("Bot " + d.config.Token)
	if err != nil {
		return fmt.Errorf("failed to create Discord session: %w", err)
	}

	d.session = s
	s.AddHandler(d.messageCreate)

	if err := s.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	return nil
}

// setSession injects a discordSession (for testing).
func (d *DiscordAdapter) setSession(session discordSession) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.session = session
}

// Listen starts listening for incoming messages.
// Returns a channel that emits MessageEvent for each incoming message.
func (d *DiscordAdapter) Listen(ctx context.Context) (<-chan *proto.MessageEvent, error) {
	return d.messageChan, nil
}

// Send sends a message to a Discord channel.
func (d *DiscordAdapter) Send(ctx context.Context, target, message string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.session == nil {
		return "", fmt.Errorf("not connected to Discord")
	}

	// Split message if it exceeds Discord's 2000 character limit
	if len(message) > 2000 {
		return d.sendSplitMessage(target, message)
	}

	msg, err := d.session.ChannelMessageSend(target, message)
	if err != nil {
		return "", fmt.Errorf("failed to send Discord message: %w", err)
	}

	return msg.ID, nil
}

// sendSplitMessage handles messages longer than 2000 characters.
func (d *DiscordAdapter) sendSplitMessage(target, message string) (string, error) {
	chunks := SplitMessageForPlatform("discord", message)

	var lastMsgID string
	for i, chunk := range chunks {
		// SplitMessageForPlatform already adds "(continued) " prefix
		msg, err := d.session.ChannelMessageSend(target, chunk)
		if err != nil {
			return "", fmt.Errorf("failed to send Discord message chunk %d: %w", i, err)
		}
		lastMsgID = msg.ID
	}

	return lastMsgID, nil
}

// Close disconnects from Discord.
func (d *DiscordAdapter) Close(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.session == nil {
		return nil
	}

	close(d.closeChan)
	err := d.session.Close()
	d.session = nil
	return err
}

// ListChannels returns available Discord channels.
func (d *DiscordAdapter) ListChannels(ctx context.Context) ([]*proto.Channel, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.session == nil {
		return nil, fmt.Errorf("not connected to Discord")
	}

	var channels []*proto.Channel

	for _, guildID := range d.config.Guilds {
		_, err := d.session.Guild(guildID)
		if err != nil {
			continue
		}

		guildChannels, err := d.session.GuildChannels(guildID)
		if err != nil {
			continue
		}

		for _, channel := range guildChannels {
			if channel.Type == discordgo.ChannelTypeGuildText {
				channels = append(channels, &proto.Channel{
					Id:       channel.ID,
					Name:     channel.Name,
					Platform: "discord",
				})
			}
		}
	}

	return channels, nil
}

// messageCreate handles incoming Discord messages.
// Note: s is *discordgo.Session (required by discordgo's handler signature),
// not our discordSession interface. We use d.session (which is the same object)
// for the channel lookup so tests can override it.
func (d *DiscordAdapter) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if s.State != nil && s.State.User != nil && m.Author.ID == s.State.User.ID {
		return
	}

	// Check if channel is in the allowed list
	if !d.isAllowedChannel(m.ChannelID) {
		return
	}

	event := &proto.MessageEvent{
		Platform:    "discord",
		ChannelId:   m.ChannelID,
		ChannelName: m.ChannelID, // default to ID; enriched below
		SenderId:    m.Author.ID,
		SenderName:  m.Author.Username,
		Message:     m.Content,
		Timestamp:   m.Timestamp.Unix(),
		MessageId:   m.ID,
	}

	// Enrich with channel name via our interface (mockable in tests)
	d.mu.Lock()
	session := d.session
	d.mu.Unlock()
	if session != nil {
		if channel, err := session.Channel(m.ChannelID); err == nil && channel != nil {
			event.ChannelName = channel.Name
		}
	}

	select {
	case d.messageChan <- event:
	case <-d.closeChan:
		return
	}
}

// isAllowedChannel checks if a channel is in the allowed list.
// If no allowed channels are configured, all channels are allowed.
func (d *DiscordAdapter) isAllowedChannel(channelID string) bool {
	if len(d.config.AllowedChannels) == 0 {
		return true
	}
	for _, allowed := range d.config.AllowedChannels {
		if allowed == channelID {
			return true
		}
	}
	return false
}
