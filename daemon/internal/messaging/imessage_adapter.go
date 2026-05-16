// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package messaging

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
	_ "modernc.org/sqlite"
)

// iMessageSender is the interface for sending iMessages (mockable).
type iMessageSender interface {
	Send(target, message string) (string, error)
}

// iMessageQuerier is the interface for querying iMessage DB (mockable).
type iMessageQuerier interface {
	QueryMessages(sinceROWID int64) ([]iMessageRow, error)
	QueryChats() ([]*pb.Channel, error)
	LastROWID() (int64, error)
}

// iMessageRow represents a message row from the Messages database.
type iMessageRow struct {
	ROWID     int64
	Text      string
	HandleID  string // phone number or email of sender
	IsFromMe  bool
	Date      int64  // Mac absolute time (nanoseconds since 2001-01-01)
	RoomName  string // chat identifier (group chat name or individual)
	CacheRoom string // cache_roomnames field
}

// IMessageConfig holds iMessage adapter configuration.
type IMessageConfig struct {
	Enabled           bool
	AllowedRecipients []string      // phone numbers or emails; empty = all
	DBPath            string        // path to chat.db; empty = ~/Library/Messages/chat.db
	PollInterval      time.Duration // default 5s
}

// IMessageAdapter implements MessagingAdapter for iMessage on macOS.
// It uses AppleScript for sending and polls the Messages database for receiving.
type IMessageAdapter struct {
	config      IMessageConfig
	sender      iMessageSender
	querier     iMessageQuerier
	db          *sql.DB
	lastROWID   int64
	mu          sync.Mutex
	stopPolling chan struct{}
	connected   bool
}

// commandRunner is a function type for executing osascript commands (mockable).
type commandRunner func(name string, args ...string) ([]byte, error)

// realCommandRunner executes commands via os/exec.
func realCommandRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// appleScriptSender sends iMessages via osascript.
type appleScriptSender struct {
	runCmd commandRunner // defaults to realCommandRunner; overridable for testing
}

// newAppleScriptSender creates an appleScriptSender with the real command runner.
func newAppleScriptSender() *appleScriptSender {
	return &appleScriptSender{runCmd: realCommandRunner}
}

// buildScript constructs the AppleScript for sending a message.
// Extracted for testability — verifies script generation without running osascript.
func (s *appleScriptSender) buildScript(target, message string) string {
	// Escape AppleScript special characters
	escaped := strings.ReplaceAll(message, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)

	if strings.HasPrefix(target, "chat:") {
		// Group chat syntax
		return fmt.Sprintf(`tell application "Messages" to send "%s" to chat "%s"`, escaped, target)
	}
	// Individual buddy syntax
	return fmt.Sprintf(`tell application "Messages" to send "%s" to buddy "%s"`, escaped, target)
}

func (s *appleScriptSender) Send(target, message string) (string, error) {
	script := s.buildScript(target, message)

	output, err := s.runCmd("osascript", "-e", script)
	if err != nil {
		return "", fmt.Errorf("osascript failed: %w, output: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

// chatDBQuerier queries the macOS Messages database.
type chatDBQuerier struct {
	db *sql.DB
}

func (q *chatDBQuerier) QueryMessages(sinceROWID int64) ([]iMessageRow, error) {
	rows, err := q.db.Query(`
		SELECT
			m.ROWID, m.text, m.is_from_me, m.date, m.cache_roomnames,
			h.id
		FROM message m
		LEFT JOIN handle h ON m.handle_id = h.ROWID
		WHERE m.ROWID > ? AND m.is_from_me = 0 AND m.text IS NOT NULL AND m.text != ''
		ORDER BY m.ROWID ASC
	`, sinceROWID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []iMessageRow
	for rows.Next() {
		var row iMessageRow
		var text, handleID, cacheRoom sql.NullString
		var date int64
		var isFromMe int64

		if err := rows.Scan(&row.ROWID, &text, &isFromMe, &date, &cacheRoom, &handleID); err != nil {
			continue
		}

		row.Text = text.String
		row.HandleID = handleID.String
		row.IsFromMe = isFromMe != 0
		row.Date = date
		row.CacheRoom = cacheRoom.String
		row.RoomName = cacheRoom.String // use cache_roomnames as chat identifier

		messages = append(messages, row)
	}
	return messages, nil
}

func (q *chatDBQuerier) QueryChats() ([]*pb.Channel, error) {
	rows, err := q.db.Query(`
		SELECT
			c.ROWID, c.chat_identifier, c.display_name
		FROM chat c
		ORDER BY c.ROWID
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("query chats: %w", err)
	}
	defer rows.Close()

	var channels []*pb.Channel
	for rows.Next() {
		var id int64
		var identifier, displayName sql.NullString

		if err := rows.Scan(&id, &identifier, &displayName); err != nil {
			continue
		}

		name := identifier.String
		if displayName.Valid && displayName.String != "" {
			name = displayName.String
		}

		channels = append(channels, &pb.Channel{
			Id:       fmt.Sprintf("%d", id),
			Name:     name,
			Platform: "imessage",
		})
	}
	return channels, nil
}

func (q *chatDBQuerier) LastROWID() (int64, error) {
	var maxID sql.NullInt64
	err := q.db.QueryRow("SELECT MAX(ROWID) FROM message").Scan(&maxID)
	if err != nil {
		return 0, fmt.Errorf("query last rowid: %w", err)
	}
	if !maxID.Valid {
		return 0, nil
	}
	return maxID.Int64, nil
}

// PlatformName returns the platform identifier for iMessage.
func (a *IMessageAdapter) PlatformName() string {
	return "imessage"
}

// NewIMessageAdapter creates a new iMessage adapter.
func NewIMessageAdapter(config IMessageConfig) *IMessageAdapter {
	if config.PollInterval == 0 {
		config.PollInterval = 5 * time.Second
	}
	if config.DBPath == "" {
		home, _ := os.UserHomeDir()
		config.DBPath = home + "/Library/Messages/chat.db"
	}
	return &IMessageAdapter{
		config:      config,
		sender:      newAppleScriptSender(),
		stopPolling: make(chan struct{}),
	}
}

// setSender injects a custom iMessageSender (for testing).
func (a *IMessageAdapter) setSender(sender iMessageSender) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sender = sender
}

// setQuerier injects a custom iMessageQuerier (for testing).
func (a *IMessageAdapter) setQuerier(querier iMessageQuerier) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.querier = querier
}

// Connect opens the Messages database and initializes state.
func (a *IMessageAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return nil
	}

	// Check if DB file exists
	if _, err := os.Stat(a.config.DBPath); os.IsNotExist(err) {
		return fmt.Errorf("iMessage database not found at %s", a.config.DBPath)
	}

	// Open database (read-only mode — we never write to chat.db)
	db, err := sql.Open("sqlite", a.config.DBPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("failed to open iMessage database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping iMessage database: %w", err)
	}

	a.db = db
	a.querier = &chatDBQuerier{db: db}

	// Get current max ROWID to start polling from (don't process old messages)
	lastID, err := a.querier.LastROWID()
	if err != nil {
		log.Printf("iMessage: warning: could not get last message ID: %v", err)
		lastID = 0
	}
	a.lastROWID = lastID
	a.connected = true

	log.Printf("iMessage adapter connected (starting from ROWID %d)", a.lastROWID)
	return nil
}

// Listen starts polling the Messages database for new messages.
func (a *IMessageAdapter) Listen(ctx context.Context) (<-chan *pb.MessageEvent, error) {
	a.mu.Lock()
	if !a.connected {
		a.mu.Unlock()
		return nil, fmt.Errorf("not connected to iMessage database")
	}
	a.mu.Unlock()

	messageChan := make(chan *pb.MessageEvent, 100)

	go func() {
		ticker := time.NewTicker(a.config.PollInterval)
		defer ticker.Stop()
		defer close(messageChan)

		for {
			select {
			case <-ctx.Done():
				return
			case <-a.stopPolling:
				return
			case <-ticker.C:
				a.pollForMessages(ctx, messageChan)
			}
		}
	}()

	return messageChan, nil
}

// pollForMessages queries the database for new messages since lastROWID.
func (a *IMessageAdapter) pollForMessages(ctx context.Context, messageChan chan<- *pb.MessageEvent) {
	a.mu.Lock()
	querier := a.querier
	lastROWID := a.lastROWID
	a.mu.Unlock()

	if querier == nil {
		return
	}

	messages, err := querier.QueryMessages(lastROWID)
	if err != nil {
		log.Printf("iMessage: poll error: %v", err)
		return
	}

	for _, msg := range messages {
		// Skip empty messages (defense in depth — SQL query also filters these)
		if msg.Text == "" {
			continue
		}

		// Filter by allowed recipients if configured
		if len(a.config.AllowedRecipients) > 0 {
			allowed := false
			for _, r := range a.config.AllowedRecipients {
				if r == msg.HandleID || r == msg.CacheRoom {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		// Determine channel ID and name
		channelID := msg.CacheRoom
		if channelID == "" {
			channelID = msg.HandleID // individual chat
		}
		channelName := msg.RoomName
		if channelName == "" {
			channelName = channelID
		}

		// Convert Mac absolute time to Unix timestamp
		// Mac epoch is 2001-01-01 00:00:00 UTC = 978307200 seconds after Unix epoch
		// Mac time is stored in nanoseconds
		var unixTimestamp int64
		if msg.Date > 0 {
			unixTimestamp = (msg.Date/1e9 + 978307200)
		}

		event := &pb.MessageEvent{
			Platform:    "imessage",
			ChannelId:   channelID,
			ChannelName: channelName,
			SenderId:    msg.HandleID,
			SenderName:  msg.HandleID, // iMessage uses phone/email as name fallback
			Message:     msg.Text,
			Timestamp:   unixTimestamp,
			MessageId:   fmt.Sprintf("%d", msg.ROWID),
		}

		select {
		case messageChan <- event:
		case <-ctx.Done():
			return
		}

		// Update last seen ROWID
		a.mu.Lock()
		if msg.ROWID > a.lastROWID {
			a.lastROWID = msg.ROWID
		}
		a.mu.Unlock()
	}
}

// Send sends an iMessage via AppleScript.
func (a *IMessageAdapter) Send(ctx context.Context, target, message string) (string, error) {
	a.mu.Lock()
	sender := a.sender
	a.mu.Unlock()

	if sender == nil {
		return "", fmt.Errorf("not connected to iMessage")
	}

	if target == "" {
		return "", fmt.Errorf("target cannot be empty")
	}

	// Split message if it exceeds a reasonable length for iMessage
	// iMessage handles long messages natively, but we cap at 4000 chars
	// for AppleScript command line limits
	if len(message) > 4000 {
		return a.sendSplitMessage(target, message)
	}

	return sender.Send(target, message)
}

// sendSplitMessage handles messages longer than 4000 characters.
func (a *IMessageAdapter) sendSplitMessage(target, message string) (string, error) {
	chunks := SplitMessageForPlatform("imessage", message)

	var lastResult string
	for i, chunk := range chunks {
		// SplitMessageForPlatform already adds "(continued) " prefix
		result, err := a.sender.Send(target, chunk)
		if err != nil {
			return "", fmt.Errorf("failed to send iMessage chunk %d: %w", i, err)
		}
		lastResult = result
	}
	return lastResult, nil
}

// Close stops the polling goroutine and closes the database.
func (a *IMessageAdapter) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return nil
	}

	// Stop polling goroutine
	select {
	case <-a.stopPolling:
		// Already stopped
	default:
		close(a.stopPolling)
	}

	a.connected = false

	if a.db != nil {
		err := a.db.Close()
		a.db = nil
		return err
	}
	return nil
}

// ListChannels returns recent iMessage conversations.
func (a *IMessageAdapter) ListChannels(ctx context.Context) ([]*pb.Channel, error) {
	a.mu.Lock()
	querier := a.querier
	a.mu.Unlock()

	if querier == nil {
		return nil, fmt.Errorf("not connected to iMessage database")
	}

	channels, err := querier.QueryChats()
	if err != nil {
		return nil, fmt.Errorf("list iMessage channels: %w", err)
	}
	return channels, nil
}
