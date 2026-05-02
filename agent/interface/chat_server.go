package iface

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aj-nt/mnemosyne/agent"
	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	memoclient "github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// ChatServer implements the gRPC AgentChat service for the agent binary.
// It allows clients (like Hermes) to have interactive conversations with the agent
// over gRPC, with streaming responses.
type ChatServer struct {
	pb.UnimplementedAgentChatServer
	agent         *agent.Agent
	mnemo         *memoclient.Client
	sessions      sync.Map // sessionID -> *chatSession
	sessionCounter atomic.Int64
}
// chatSession holds the persistent state for a single conversation.
type chatSession struct {
	sessionID string
	agentID   string
	messages  []agentctx.Message
	mu        sync.Mutex
}

// NewChatServer creates a new ChatServer.
func NewChatServer(a *agent.Agent, mnemo *memoclient.Client) *ChatServer {
	return &ChatServer{
		agent: a,
		mnemo: mnemo,
	}
}

// Ping implements the health check for the AgentChat service.
func (cs *ChatServer) Ping(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status:    "SERVING",
		Version:   "1.0.0",
		Timestamp: 0,
	}, nil
}

// ChatStream handles a streaming chat request.
// It creates or resumes a session, runs the agent, and streams back events.
func (cs *ChatServer) ChatStream(req *pb.ChatRequest, stream pb.AgentChat_ChatStreamServer) error {
	ctx := stream.Context()

	// Get or create the session
	sess, err := cs.getOrCreateSession(ctx, req.SessionId, req.AgentId)
	if err != nil {
		return fmt.Errorf("session error: %w", err)
	}

	sess.mu.Lock()
	// Add the user message to the conversation
	sess.messages = append(sess.messages, agentctx.Message{
		Role:    "user",
		Content: req.Message,
	})
	messages := make([]agentctx.Message, len(sess.messages))
	copy(messages, sess.messages)
	sess.mu.Unlock()

	// Refresh the hot block
	hotBlock, err := cs.agent.GetHotBlock(ctx)
	if err != nil {
		log.Printf("ChatStream: hot block error: %v", err)
		hotBlock = ""
	}

	// Build conversation context
	toolDefs := cs.agent.BuildToolDefinitions()
	convCtx := &agentctx.ConversationContext{
		SessionID:       sess.sessionID,
		AgentID:         cs.agent.Config.AgentID,
		Messages:        messages,
		SystemPrompt:    "",
		Tools:           toolDefs,
		MaxIterations:   cs.agent.Config.MaxTurns,
		Iteration:       0,
		Interrupt:       agentctx.NewInterruptSignal(),
		HotBlockContent: hotBlock,
		Temperature:     cs.agent.Config.Temperature,
	}

	// Auto-approve all tool calls in gRPC mode
	cs.agent.Stage4.Approver = func(toolName string, args map[string]interface{}) bool {
		return true // Always approve — the caller is a program, not a human
	}

	// Stream the agent response
	eventCh := make(chan agent.StreamEvent, 64)
	go cs.agent.RunStreaming(ctx, convCtx, eventCh)

	// Send initial session ID
	if err := stream.Send(&pb.ChatEvent{
		EventType:  pb.ChatEvent_THINKING,
		SessionId:  sess.sessionID,
	}); err != nil {
		return err
	}

	var totalTokens int32
	turnNumber := int32(0)

	for event := range eventCh {
		var chatEvent *pb.ChatEvent

		switch event.Type {
		case agentctx.StreamText:
			chatEvent = &pb.ChatEvent{
				EventType: pb.ChatEvent_TEXT,
				Text:      event.Text,
				SessionId: sess.sessionID,
			}

		case agentctx.StreamToolStart:
			chatEvent = &pb.ChatEvent{
				EventType: pb.ChatEvent_TOOL_START,
				ToolName:  event.ToolName,
				SessionId: sess.sessionID,
			}

		case agentctx.StreamToolEnd:
			// Tool done parsing — skip, we'll send TOOL_RESULT after execution

		case agentctx.StreamToolExec:
			chatEvent = &pb.ChatEvent{
				EventType: pb.ChatEvent_TOOL_START,
				ToolName:  event.ToolName,
				Text:       fmt.Sprintf("Executing %s...", event.ToolName),
				SessionId: sess.sessionID,
			}

		case agentctx.StreamDone:
			if event.Usage != nil {
				totalTokens += int32(event.Usage.TotalTokens)
			}
			turnNumber++
			chatEvent = &pb.ChatEvent{
				EventType:  pb.ChatEvent_DONE,
				SessionId:   sess.sessionID,
				TokensUsed:  totalTokens,
				TurnNumber:   turnNumber,
			}

		case agentctx.StreamError:
			chatEvent = &pb.ChatEvent{
				EventType: pb.ChatEvent_ERROR,
				Text:       event.Error.Error(),
				SessionId: sess.sessionID,
			}

		default:
			// Skip unknown event types
			continue
		}

		if chatEvent != nil {
			if err := stream.Send(chatEvent); err != nil {
				return err
			}
		}
	}

	// Update the session with the new messages
	sess.mu.Lock()
	sess.messages = convCtx.Messages
	sess.mu.Unlock()

	// Persist the session to the daemon
	if cs.mnemo != nil && sess.sessionID != "" {
		cs.persistSession(ctx, sess)
	}

	return nil
}

// getOrCreateSession returns an existing session or creates a new one.
func (cs *ChatServer) getOrCreateSession(ctx context.Context, sessionID, agentID string) (*chatSession, error) {
	// If session ID provided, look up existing session
	if sessionID != "" {
		if existing, ok := cs.sessions.Load(sessionID); ok {
			return existing.(*chatSession), nil
		}

		// Try to resume from the daemon
		if cs.mnemo != nil {
			detail, err := cs.mnemo.GetSession(ctx, sessionID)
			if err == nil && detail != nil {
				msgs := make([]agentctx.Message, 0, len(detail.Messages))
				for _, m := range detail.Messages {
					msgs = append(msgs, agentctx.Message{Role: m.Role, Content: m.Content})
				}
				sess := &chatSession{
					sessionID: sessionID,
			agentID:   agentID,
					messages:  msgs,
				}
				cs.sessions.Store(sessionID, sess)
				return sess, nil
			}
			// Session not found on daemon — create a new one
			newID, createErr := cs.mnemo.CreateSession(ctx, agentID, "grpc", "")
			if createErr != nil {
				log.Printf("ChatStream: failed to create session for unknown ID %s: %v", sessionID, createErr)
				// Fall back to using the provided ID locally
				sess := &chatSession{
					sessionID: sessionID,
			agentID:   agentID,
					messages:  nil,
				}
				cs.sessions.Store(sessionID, sess)
				return sess, nil
			}
			sess := &chatSession{
				sessionID: newID,
				agentID:   agentID,
				messages:  nil,
			}
			cs.sessions.Store(newID, sess)
			return sess, nil
		}

		// No daemon — use the provided session ID locally
		sess := &chatSession{
			sessionID: sessionID,
			agentID:   agentID,
			messages:  nil,
		}
		cs.sessions.Store(sessionID, sess)
		return sess, nil
	}

	// Create a new session
	var newSessionID string
	if cs.mnemo != nil {
		var err error
		newSessionID, err = cs.mnemo.CreateSession(ctx, agentID, "grpc", "")
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
	} else {
		// No daemon — generate a local session ID
		counter := cs.sessionCounter.Add(1)
		newSessionID = fmt.Sprintf("local-%d", counter)
	}

	sess := &chatSession{
		sessionID: newSessionID,
		agentID:   agentID,
		messages:  nil,
	}
	if newSessionID != "" {
		cs.sessions.Store(newSessionID, sess)
	}
	return sess, nil
}

// persistSession saves the session messages to the daemon.
// If the session doesn't exist on the daemon, it creates one first.
func (cs *ChatServer) persistSession(ctx context.Context, sess *chatSession) {
	if cs.mnemo == nil || sess.sessionID == "" {
		return
	}

	sess.mu.Lock()
	msgs := make([]memoclient.Message, 0, len(sess.messages))
	for _, m := range sess.messages {
		msgs = append(msgs, memoclient.Message{Role: m.Role, Content: m.Content})
	}
	sess.mu.Unlock()

	persistCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := cs.mnemo.AddMessages(persistCtx, sess.sessionID, msgs); err != nil {
		log.Printf("ChatStream: session persist failed: %v", err)
	}
}