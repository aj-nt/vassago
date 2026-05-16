// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package messaging

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"sync"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
)

// EmailConfig holds email adapter configuration.
type EmailConfig struct {
	IMAPHost     string // e.g., "127.0.0.1:1143" (Proton Bridge)
	SMTPHost     string // e.g., "127.0.0.1:1025" (Proton Bridge)
	Username     string
	Password     string
	FromAddress  string        // e.g., "agent@example.com"
	AllowedFrom  []string      // Only process emails from these senders; empty = all
	Folders      []string      // IMAP folders to check; default ["INBOX"]
	PollInterval time.Duration // default 30s
}

// EmailMessage represents an email message from IMAP.
type EmailMessage struct {
	UID       int32
	From      string
	Subject   string
	Body      string
	Date      time.Time
	MessageID string
}

// emailSender is the interface for sending emails (mockable).
type emailSender interface {
	SendSMTP(from string, to []string, subject, body string) error
}

// emailFetcher is the interface for fetching emails from IMAP (mockable).
type emailFetcher interface {
	FetchNewMessages(sinceUID int32) ([]EmailMessage, error)
	MarkSeen(uid int32) error
	Close() error
}

// smtpSender implements emailSender using net/smtp.
type smtpSender struct {
	host     string
	auth     smtp.Auth
	fromAddr string
}

func newSMTPSender(host, username, password, fromAddr string) *smtpSender {
	// Extract hostname without port for PlainAuth
	hostWithoutPort := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		hostWithoutPort = host[:idx]
	}
	return &smtpSender{
		host:     host,
		auth:     smtp.PlainAuth("", username, password, hostWithoutPort),
		fromAddr: fromAddr,
	}
}

func (s *smtpSender) SendSMTP(from string, to []string, subject, body string) error {
	// Build email message
	var msg strings.Builder

	// Headers
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")

	// Split body into lines of max 78 characters for SMTP compliance
	for len(body) > 0 {
		line := body
		if len(line) > 78 {
			// Find a good break point
			line = body[:78]
		}
		msg.WriteString(line)
		msg.WriteString("\r\n")
		if len(body) <= 78 {
			break
		}
		body = body[78:]
	}

	// Connect and send
	client, err := smtp.Dial(s.host)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Close()

	if err := client.Auth(s.auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	if err := client.Mail(s.fromAddr); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start email data: %w", err)
	}

	_, err = w.Write([]byte(msg.String()))
	if err != nil {
		return fmt.Errorf("failed to write email: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close email writer: %w", err)
	}

	return client.Quit()
}

// imapFetcher is a stub implementation for IMAP fetching.
// A proper IMAP implementation will be added when a pure-Go IMAP library is integrated.
type imapFetcher struct{}

func newIMAPFetcher() *imapFetcher {
	return &imapFetcher{}
}

func (f *imapFetcher) FetchNewMessages(sinceUID int32) ([]EmailMessage, error) {
	// Stub: returns error indicating IMAP is not yet implemented
	return nil, errors.New("IMAP fetching not yet implemented — requires IMAP library integration")
}

func (f *imapFetcher) MarkSeen(uid int32) error {
	return nil
}

func (f *imapFetcher) Close() error {
	return nil
}

// EmailAdapter implements MessagingAdapter for email (IMAP/SMTP).
type EmailAdapter struct {
	config      EmailConfig
	sender      emailSender
	fetcher     emailFetcher
	lastSeenUID int32
	mu          sync.Mutex
	stopChan    chan struct{}
	connected   bool
}

// PlatformName returns the platform identifier for Email.
func (e *EmailAdapter) PlatformName() string {
	return "email"
}

// NewEmailAdapter creates a new Email adapter.
func NewEmailAdapter(config EmailConfig) *EmailAdapter {
	if config.PollInterval == 0 {
		config.PollInterval = 30 * time.Second
	}
	if len(config.Folders) == 0 {
		config.Folders = []string{"INBOX"}
	}

	sender := newSMTPSender(config.SMTPHost, config.Username, config.Password, config.FromAddress)
	fetcher := newIMAPFetcher()

	return &EmailAdapter{
		config:   config,
		sender:   sender,
		fetcher:  fetcher,
		stopChan: make(chan struct{}),
	}
}

// setSender injects a custom emailSender (for testing).
func (a *EmailAdapter) setSender(sender emailSender) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sender = sender
}

// setFetcher injects a custom emailFetcher (for testing).
func (a *EmailAdapter) setFetcher(fetcher emailFetcher) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fetcher = fetcher
}

// Connect verifies SMTP connectivity.
func (a *EmailAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return nil
	}

	// Verify SMTP connectivity by dialing
	// We don't keep the connection open — SMTP connections are per-send
	if a.config.SMTPHost != "" {
		client, err := smtp.Dial(a.config.SMTPHost)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		client.Close()
	}

	a.connected = true
	return nil
}

// Listen starts polling for new emails via IMAP.
func (a *EmailAdapter) Listen(ctx context.Context) (<-chan *pb.MessageEvent, error) {
	if !a.connected {
		return nil, fmt.Errorf("not connected — call Connect() first")
	}

	events := make(chan *pb.MessageEvent, 100)
	go a.pollEmails(ctx, events)
	return events, nil
}

// pollEmails polls the IMAP server for new emails at the configured interval.
func (a *EmailAdapter) pollEmails(ctx context.Context, events chan<- *pb.MessageEvent) {
	ticker := time.NewTicker(a.config.PollInterval)
	defer ticker.Stop()
	defer close(events)

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopChan:
			return
		case <-ticker.C:
			a.pollOnce(ctx, events)
		}
	}
}

// pollOnce performs a single poll of the IMAP server.
func (a *EmailAdapter) pollOnce(ctx context.Context, events chan<- *pb.MessageEvent) {
	a.mu.Lock()
	fetcher := a.fetcher
	lastUID := a.lastSeenUID
	allowedFrom := a.config.AllowedFrom
	folders := a.config.Folders
	a.mu.Unlock()

	if fetcher == nil {
		return
	}

	messages, err := fetcher.FetchNewMessages(lastUID)
	if err != nil {
		// Log and continue — polling errors are transient
		return
	}

	for _, msg := range messages {
		// Skip empty messages
		if msg.Body == "" && msg.Subject == "" {
			continue
		}

		// Filter by allowed senders if configured
		if len(allowedFrom) > 0 {
			allowed := false
			for _, from := range allowedFrom {
				if from == msg.From {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		// Combine subject and body for the message content
		content := msg.Body
		if msg.Subject != "" {
			content = msg.Subject + "\n\n" + msg.Body
		}

		// Use sender email as both sender ID and name
		senderName := msg.From
		if idx := strings.Index(msg.From, "@"); idx > 0 {
			senderName = msg.From[:idx] // Use part before @ as display name
		}

		channelID := fmt.Sprintf("email:%s", folders[0])

		event := &pb.MessageEvent{
			Platform:    "email",
			ChannelId:   channelID,
			ChannelName: folders[0],
			SenderId:    msg.From,
			SenderName:  senderName,
			Message:     content,
			Timestamp:   msg.Date.Unix(),
			MessageId:   msg.MessageID,
		}

		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	}

	// Update last seen UID
	if len(messages) > 0 {
		a.mu.Lock()
		maxUID := a.lastSeenUID
		for _, msg := range messages {
			if msg.UID > maxUID {
				maxUID = msg.UID
			}
		}
		a.lastSeenUID = maxUID
		a.mu.Unlock()
	}
}

// Send sends an email message.
func (a *EmailAdapter) Send(ctx context.Context, target, message string) (string, error) {
	a.mu.Lock()
	sender := a.sender
	a.mu.Unlock()

	if sender == nil {
		return "", fmt.Errorf("not connected to email")
	}

	if target == "" {
		return "", fmt.Errorf("target email address is required")
	}

	// Validate target is an email address
	if !isValidEmail(target) {
		return "", fmt.Errorf("invalid email address: %s", target)
	}

	// Parse subject from message if it starts with "Subject: "
	subject := "Message from Vassago"
	body := message
	if strings.HasPrefix(message, "Subject: ") {
		endSubject := strings.Index(message, "\n")
		if endSubject > 0 {
			subject = message[9:endSubject]
			body = message[endSubject+1:]
		} else {
			subject = message[9:]
			body = ""
		}
	}

	err := sender.SendSMTP(a.config.FromAddress, []string{target}, subject, body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("email-%d", time.Now().UnixNano()), nil
}

// Close stops the email adapter.
func (a *EmailAdapter) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return nil
	}

	select {
	case <-a.stopChan:
		// Already stopped
	default:
		close(a.stopChan)
	}

	a.connected = false

	if a.fetcher != nil {
		return a.fetcher.Close()
	}
	return nil
}

// ListChannels returns the email folders as channels.
func (a *EmailAdapter) ListChannels(ctx context.Context) ([]*pb.Channel, error) {
	var channels []*pb.Channel
	for _, folder := range a.config.Folders {
		channels = append(channels, &pb.Channel{
			Id:       fmt.Sprintf("email:%s", folder),
			Name:     folder,
			Platform: "email",
		})
	}
	return channels, nil
}

// isValidEmail performs a simple email validation.
func isValidEmail(email string) bool {
	if !strings.Contains(email, "@") || len(email) < 5 {
		return false
	}
	at := strings.Index(email, "@")
	if at == 0 || at == len(email)-1 {
		// No local part or no domain
		return false
	}
	domain := email[at+1:]
	// Domain must have at least one dot and a TLD
	if !strings.Contains(domain, ".") {
		return false
	}
	// Check for trailing dot
	if strings.HasSuffix(domain, ".") {
		return false
	}
	return true
}
