// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"testing"

	"github.com/aj-nt/mnemosyne/client"
	pb "github.com/aj-nt/mnemosyne/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSavedToolClient implements client.MnemoClient for testing saved tool handlers.
type mockSavedToolClient struct {
	client.NullMnemo
	tools map[string]*pb.SavedToolEntry // id -> entry
}

func newMockSavedToolClient() *mockSavedToolClient {
	return &mockSavedToolClient{
		tools: make(map[string]*pb.SavedToolEntry),
	}
}

func (m *mockSavedToolClient) AddSavedTool(ctx context.Context, name, description, toolType, parameters, script, category string, triggers []string, author string, priority int32) (*pb.SavedToolEntry, error) {
	// Don't allow duplicates
	for _, e := range m.tools {
		if e.Name == name {
			return nil, assert.AnError
		}
	}
	entry := &pb.SavedToolEntry{
		Id:          "tool-" + name,
		Name:        name,
		Description: description,
		Type:        toolType,
		Parameters:  parameters,
		Script:      script,
		Category:    category,
		Triggers:    triggers,
		Author:      author,
		Priority:    priority,
		Version:     1,
		RequiresApprovalEveryCall: true,
	}
	m.tools[entry.Id] = entry
	return entry, nil
}

func (m *mockSavedToolClient) GetSavedTool(ctx context.Context, id string) (*pb.SavedToolEntry, error) {
	e, ok := m.tools[id]
	if !ok {
		return nil, assert.AnError
	}
	return e, nil
}

func (m *mockSavedToolClient) ListSavedTools(ctx context.Context, category, author string, limit int32) ([]*pb.SavedToolEntry, error) {
	var result []*pb.SavedToolEntry
	for _, e := range m.tools {
		if category != "" && e.Category != category {
			continue
		}
		if author != "" && e.Author != author {
			continue
		}
		result = append(result, e)
		if limit > 0 && len(result) >= int(limit) {
			break
		}
	}
	return result, nil
}

func (m *mockSavedToolClient) SearchSavedTools(ctx context.Context, query, category string, limit int32) ([]*pb.SavedToolEntry, error) {
	return m.ListSavedTools(ctx, category, "", limit)
}

func (m *mockSavedToolClient) UpdateSavedTool(ctx context.Context, id string, updates map[string]interface{}) (*pb.SavedToolEntry, error) {
	e, ok := m.tools[id]
	if !ok {
		return nil, assert.AnError
	}
	if v, ok := updates["description"].(string); ok {
		e.Description = v
	}
	if v, ok := updates["priority"].(float64); ok {
		e.Priority = int32(v)
	} else if v, ok := updates["priority"].(int); ok {
		e.Priority = int32(v)
	}
	e.Version++
	return e, nil
}

func (m *mockSavedToolClient) RemoveSavedTool(ctx context.Context, id string) (bool, error) {
	_, ok := m.tools[id]
	if ok {
		delete(m.tools, id)
		return true, nil
	}
	return false, nil
}

func (m *mockSavedToolClient) IsConnected() bool { return true }

func TestCreateTool_ValidScript(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, err := reg.Get("CreateTool")
	require.NoError(t, err)

	result, isErr := handler(map[string]interface{}{
		"name":        "my_deploy",
		"description": "Deploy to staging",
		"type":        "script",
		"parameters": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"env": map[string]interface{}{"type": "string"}},
			"required":   []interface{}{"env"},
		},
		"script":   "deploy.sh {{env}}",
		"category": "devops",
		"triggers": []interface{}{"deploy", "staging"},
		"priority": float64(4),
	})
	require.False(t, isErr)
	assert.Contains(t, result, "Tool created")
	assert.Contains(t, result, "my_deploy")
	assert.Contains(t, result, "v1")

	// Verify it's stored
	entry, err := mock.GetSavedTool(context.Background(), "tool-my_deploy")
	require.NoError(t, err)
	assert.Equal(t, "my_deploy", entry.Name)
	assert.Equal(t, "devops", entry.Category)
}

func TestCreateTool_MissingRequiredFields(t *testing.T) {
	mock := newMockSavedToolClient()
	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("CreateTool")

	// Missing name
	result, isErr := handler(map[string]interface{}{
		"description": "test",
		"parameters":  map[string]interface{}{},
		"script":      "echo hi",
	})
	assert.True(t, isErr)
	assert.Contains(t, result, "required")
}

func TestGetTool(t *testing.T) {
	mock := newMockSavedToolClient()
	// Pre-populate
	mock.AddSavedTool(context.Background(), "existing", "Exists", "script",
		`{"type":"object","properties":{}}`, "echo hi", "test", nil, "agent-1", 3)

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("GetTool")

	// Valid get
	result, isErr := handler(map[string]interface{}{"id": "tool-existing"})
	assert.False(t, isErr)
	assert.Contains(t, result, "existing")

	// Missing ID
	_, isErr = handler(map[string]interface{}{})
	assert.True(t, isErr)

	// Non-existent
	_, isErr = handler(map[string]interface{}{"id": "nope"})
	assert.True(t, isErr)
}

func TestListTools_ByCategory(t *testing.T) {
	mock := newMockSavedToolClient()
	mock.AddSavedTool(context.Background(), "a", "A", "script", "{}", "echo a", "devops", nil, "agent-1", 3)
	mock.AddSavedTool(context.Background(), "b", "B", "script", "{}", "echo b", "astrology", nil, "agent-1", 5)

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("ListTools")

	result, isErr := handler(map[string]interface{}{
		"category": "devops",
	})
	assert.False(t, isErr)
	assert.Contains(t, result, "a")
	assert.NotContains(t, result, "b")
}

func TestUpdateTool(t *testing.T) {
	mock := newMockSavedToolClient()
	mock.AddSavedTool(context.Background(), "updateable", "Original", "script",
		`{"type":"object","properties":{"x":{"type":"number"}}}`, "echo {{x}}", "test", nil, "agent-1", 3,
	)

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("UpdateTool")

	result, isErr := handler(map[string]interface{}{
		"id":          "tool-updateable",
		"description": "Updated",
		"priority":    float64(5),
	})
	assert.False(t, isErr)
	assert.Contains(t, result, "v2")

	// Verify
	entry, _ := mock.GetSavedTool(context.Background(), "tool-updateable")
	assert.Equal(t, "Updated", entry.Description)
	assert.Equal(t, int32(5), entry.Priority)
	assert.Equal(t, int32(2), entry.Version)
}

func TestRemoveTool(t *testing.T) {
	mock := newMockSavedToolClient()
	mock.AddSavedTool(context.Background(), "remove_me", "Bye", "script", "{}", "echo bye", "test", nil, "agent-1", 3)

	reg := NewToolRegistry()
	RegisterSavedToolTools(reg, mock, "agent-1")

	handler, _ := reg.Get("RemoveTool")

	result, isErr := handler(map[string]interface{}{"id": "tool-remove_me"})
	assert.False(t, isErr)
	assert.Contains(t, result, "removed")

	// Verify gone
	_, err := mock.GetSavedTool(context.Background(), "tool-remove_me")
	assert.Error(t, err)
}
