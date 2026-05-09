// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agent

import (
	"context"
	"testing"

	agentctx "github.com/aj-nt/mnemosyne/agent/context"
	"github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
)

// mockExtractProvider implements provider.Provider for extraction testing.
type mockExtractProvider struct {
	response agentctx.ParsedResponse
	err      error
}

func (m *mockExtractProvider) Name() string                                                                              { return "mock" }
func (m *mockExtractProvider) Capabilities() agentctx.ProviderCapabilities                                                { return agentctx.ProviderCapabilities{} }
func (m *mockExtractProvider) EndpointURL() string                                                                        { return "https://mock.test" }
func (m *mockExtractProvider) Prepare(ctx context.Context, convCtx *agentctx.ConversationContext, req agentctx.PreparedRequest) (interface{}, error) {
	return req, nil
}
func (m *mockExtractProvider) Execute(ctx context.Context, req interface{}) (agentctx.ProviderResult, error) {
	return agentctx.ProviderResult{}, nil
}
func (m *mockExtractProvider) Parse(ctx context.Context, result agentctx.ProviderResult) (agentctx.ParsedResponse, error) {
	return m.response, m.err
}
func (m *mockExtractProvider) Stream(ctx context.Context, req interface{}) (<-chan agentctx.StreamEvent, error) {
	ch := make(chan agentctx.StreamEvent)
	close(ch)
	return ch, nil
}

// recordingClient embeds NullMnemo and overrides AddMemory to record calls.
type recordingClient struct {
	client.NullMnemo
	savedMemories []*pb.MemoryEntry
}

func (r *recordingClient) AddMemory(ctx context.Context, target, category, key, content string, priority int32, sourceAgent string) (*pb.MemoryEntry, error) {
	entry := &pb.MemoryEntry{Key: key, Content: content, Category: category, Priority: priority}
	r.savedMemories = append(r.savedMemories, entry)
	return entry, nil
}

func TestExtractMemories_EmptyMessages(t *testing.T) {
	mockProv := &mockExtractProvider{}
	mockClient := &recordingClient{}

	err := ExtractMemories(context.Background(), []agentctx.Message{}, mockProv, mockClient, "session1", "test-agent")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(mockClient.savedMemories) != 0 {
		t.Fatalf("expected 0 memories, got %d", len(mockClient.savedMemories))
	}
}

func TestExtractMemories_ExtractsFacts(t *testing.T) {
	mockProv := &mockExtractProvider{
		response: agentctx.ParsedResponse{
			Text: `[{"key":"user_name","content":"User's name is AJ","category":"user","priority":3}]`,
		},
	}
	mockClient := &recordingClient{}

	messages := []agentctx.Message{
		{Role: "user", Content: "My name is AJ"},
		{Role: "assistant", Content: "Nice to meet you, AJ!"},
	}

	err := ExtractMemories(context.Background(), messages, mockProv, mockClient, "session1", "test-agent")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(mockClient.savedMemories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(mockClient.savedMemories))
	}
	if mockClient.savedMemories[0].Key != "user_name" {
		t.Errorf("expected key 'user_name', got %q", mockClient.savedMemories[0].Key)
	}
}

func TestExtractMemories_NothingWorthSaving(t *testing.T) {
	mockProv := &mockExtractProvider{
		response: agentctx.ParsedResponse{
			Text: `[]`,
		},
	}
	mockClient := &recordingClient{}

	messages := []agentctx.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	err := ExtractMemories(context.Background(), messages, mockProv, mockClient, "session1", "test-agent")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(mockClient.savedMemories) != 0 {
		t.Fatalf("expected 0 memories, got %d", len(mockClient.savedMemories))
	}
}