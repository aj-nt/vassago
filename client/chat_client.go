// This file is part of Mnemosyne.
// See LICENSE-Apache-2.0 for license information.

package client

import (
	"context"
	"fmt"
	"io"

	pb "github.com/aj-nt/vassago/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ChatClient connects to the agent's gRPC chat server for interactive conversations.
type ChatClient struct {
	conn   *grpc.ClientConn
	client pb.AgentChatClient
}

// NewChatClient creates a new chat client connected to the agent's gRPC endpoint.
func NewChatClient(address string) (*ChatClient, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("connect to agent chat: %w", err)
	}
	return &ChatClient{
		conn:   conn,
		client: pb.NewAgentChatClient(conn),
	}, nil
}

// Close closes the connection.
func (c *ChatClient) Close() error {
	return c.conn.Close()
}

// Ping checks if the agent chat server is reachable.
func (c *ChatClient) Ping(ctx context.Context) (string, error) {
	resp, err := c.client.Ping(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		return "", err
	}
	return resp.Status, nil
}

// ChatEvent represents a single event from the agent's chat stream.
type ChatEvent struct {
	EventType  string // "text", "tool_start", "tool_result", "done", "error", "thinking", "usage"
	Text       string
	ToolName   string
	ToolResult string
	SessionID  string
	TokensUsed int32
	TurnNumber int32
}

// Chat sends a message to the agent and returns the full response text.
// It collects all text events and returns the concatenated result.
func (c *ChatClient) Chat(ctx context.Context, sessionID, agentID, message string) (string, string, error) {
	stream, err := c.client.ChatStream(ctx, &pb.ChatRequest{
		SessionId: sessionID,
		AgentId:   agentID,
		Message:   message,
	})
	if err != nil {
		return "", "", fmt.Errorf("chat stream: %w", err)
	}

	var responseText string
	var newSessionID string

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return responseText, newSessionID, fmt.Errorf("chat stream recv: %w", err)
		}

		if event.SessionId != "" && newSessionID == "" {
			newSessionID = event.SessionId
		}

		switch event.EventType {
		case pb.ChatEvent_TEXT:
			responseText += event.Text
		case pb.ChatEvent_ERROR:
			return responseText, newSessionID, fmt.Errorf("agent error: %s", event.Text)
		case pb.ChatEvent_DONE:
			// Turn complete
		}
	}

	return responseText, newSessionID, nil
}

// processChatEvent converts a protobuf ChatEvent to the client ChatEvent type
func processChatEvent(event *pb.ChatEvent) ChatEvent {
	var eventType string
	switch event.EventType {
	case pb.ChatEvent_TEXT:
		eventType = "text"
	case pb.ChatEvent_TOOL_START:
		eventType = "tool_start"
	case pb.ChatEvent_TOOL_RESULT:
		eventType = "tool_result"
	case pb.ChatEvent_DONE:
		eventType = "done"
	case pb.ChatEvent_ERROR:
		eventType = "error"
	case pb.ChatEvent_THINKING:
		eventType = "thinking"
	case pb.ChatEvent_USAGE:
		eventType = "usage"
	default:
		eventType = "unknown"
	}

	return ChatEvent{
		EventType:  eventType,
		Text:       event.Text,
		ToolName:   event.ToolName,
		ToolResult: event.ToolResult,
		SessionID:  event.SessionId,
		TokensUsed: event.TokensUsed,
		TurnNumber: event.TurnNumber,
	}
}

// ChatStream sends a message to the agent and returns a channel of events
// for real-time streaming consumption.
func (c *ChatClient) ChatStream(ctx context.Context, sessionID, agentID, message string) (<-chan ChatEvent, error) {
	stream, err := c.client.ChatStream(ctx, &pb.ChatRequest{
		SessionId: sessionID,
		AgentId:   agentID,
		Message:   message,
	})
	if err != nil {
		return nil, fmt.Errorf("chat stream: %w", err)
	}

	ch := make(chan ChatEvent, 64)

	go func() {
		defer close(ch)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- ChatEvent{EventType: "error", Text: err.Error()}
				return
			}

			ch <- processChatEvent(event)
		}
	}()

	return ch, nil
}
