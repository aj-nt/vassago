// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddSavedTool(t *testing.T) {
	s := testStore(t)

	// Test: add a saved tool
	row, err := s.AddSavedTool("default", 
		"calculate_natal_chart",
		"Calculate a natal chart from birth data",
		"script",
		`{"type":"object","properties":{"birth_date":{"type":"string"},"birth_time":{"type":"string"},"latitude":{"type":"number"},"longitude":{"type":"number"}},"required":["birth_date","birth_time","latitude","longitude"]}`,
		"python3 calculate_chart.py --date {{birth_date}} --time {{birth_time}} --lat {{latitude}} --lon {{longitude}}",
		"astrology",
		[]string{"natal", "chart", "astrology"},
		"agent-1",
		4,
	)
	require.NoError(t, err)
	assert.NotEmpty(t, row.ID)
	assert.Equal(t, "calculate_natal_chart", row.Name)
	assert.Equal(t, "script", row.Type)
	assert.Equal(t, "astrology", row.Category)
	assert.Equal(t, "agent-1", row.Author)
	assert.Equal(t, 4, row.Priority)
	assert.Equal(t, 1, row.Version, "first version should be 1")
	assert.True(t, row.RequiresApprovalEveryCall, "default should be true")
	assert.Greater(t, row.CreatedAt, int64(0))
	assert.Equal(t, row.CreatedAt, row.UpdatedAt)

	// Test: duplicate name should fail
	_, err = s.AddSavedTool("default", 
		"calculate_natal_chart", "desc", "script", "{}", "echo hi", "test", nil, "agent-2", 3,
	)
	assert.Error(t, err, "duplicate name should fail due to UNIQUE constraint")
}

func TestGetSavedTool(t *testing.T) {
	s := testStore(t)

	row, err := s.AddSavedTool("default", "deploy", "Deploy a service", "script",
		`{"type":"object","properties":{"env":{"type":"string"}},"required":["env"]}`,
		"deploy.sh --env {{env}}", "devops", []string{"deploy", "staging"}, "agent-1", 5,
	)
	require.NoError(t, err)

	// Test: get by ID
	fetched, err := s.GetSavedTool(row.ID)
	require.NoError(t, err)
	assert.Equal(t, row.ID, fetched.ID)
	assert.Equal(t, "deploy", fetched.Name)
	assert.Equal(t, 5, fetched.Priority)

	// Test: get non-existent ID
	_, err = s.GetSavedTool("non-existent")
	assert.Error(t, err)
}

func TestListSavedTools(t *testing.T) {
	s := testStore(t)

	// Add tools in different categories
	s.AddSavedTool("default", "tool-a", "desc a", "script", "{}", "echo a", "devops", nil, "agent-1", 3)
	s.AddSavedTool("default", "tool-b", "desc b", "script", "{}", "echo b", "astrology", nil, "agent-1", 5)
	s.AddSavedTool("default", "tool-c", "desc c", "script", "{}", "echo c", "devops", nil, "agent-2", 4)

	// Test: list all
	all, err := s.ListSavedTools("default", "", "", 100)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Test: filter by category
	devops, err := s.ListSavedTools("default", "devops", "", 100)
	require.NoError(t, err)
	assert.Len(t, devops, 2)

	// Test: filter by author
	agent2, err := s.ListSavedTools("default", "", "agent-2", 100)
	require.NoError(t, err)
	assert.Len(t, agent2, 1)
	assert.Equal(t, "tool-c", agent2[0].Name)

	// Test: limit
	limited, err := s.ListSavedTools("default", "", "", 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}

func TestSearchSavedTools(t *testing.T) {
	s := testStore(t)

	s.AddSavedTool("default", "deploy_service", "Deploy to staging or production", "script",
		`{"type":"object","properties":{"env":{"type":"string"}},"required":["env"]}`,
		"deploy.sh --env {{env}}", "devops", []string{"deploy", "staging", "production"}, "agent-1", 4,
	)
	s.AddSavedTool("default", "calculate_chart", "Calculate natal chart", "script",
		`{"type":"object","properties":{"birth_date":{"type":"string"}},"required":["birth_date"]}`,
		"python chart.py", "astrology", []string{"natal", "chart"}, "agent-1", 3,
	)

	// Test: search by name
	results, err := s.SearchSavedTools("default", "deploy", "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "deploy_service", results[0].Name)

	// Test: search by trigger keyword
	results, err = s.SearchSavedTools("default", "natal", "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "calculate_chart", results[0].Name)

	// Test: search with no matches
	results, err = s.SearchSavedTools("default", "nonexistent", "", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestUpdateSavedTool(t *testing.T) {
	s := testStore(t)

	row, err := s.AddSavedTool("default", "update-test", "original", "script",
		`{"type":"object","properties":{"x":{"type":"number"}}}`,
		"echo {{x}}", "test", nil, "agent-1", 3,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, row.Version)

	// Test: update description and priority
	updated, err := s.UpdateSavedTool("default", row.ID, map[string]interface{}{
		"description": "updated description",
		"priority":    5,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version, "version should increment")
	assert.Equal(t, "updated description", updated.Description)
	assert.Equal(t, 5, updated.Priority)
	assert.GreaterOrEqual(t, updated.UpdatedAt, row.UpdatedAt)

	// Verify persistence
	fetched, _ := s.GetSavedTool(row.ID)
	assert.Equal(t, 2, fetched.Version)
	assert.Equal(t, "updated description", fetched.Description)

	// Test: update with empty changes should still return current state
	updated2, err := s.UpdateSavedTool("default", row.ID, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, 2, updated2.Version, "no-op update should not increment version")
}

func TestRemoveSavedTool(t *testing.T) {
	s := testStore(t)

	row, err := s.AddSavedTool("default", "remove-me", "desc", "script", "{}", "echo hi", "test", nil, "agent-1", 3)
	require.NoError(t, err)

	// Test: remove existing
	removed, err := s.RemoveSavedTool("default", row.ID)
	require.NoError(t, err)
	assert.True(t, removed)

	// Verify gone
	_, err = s.GetSavedTool(row.ID)
	assert.Error(t, err)

	// Test: remove non-existent
	removed, err = s.RemoveSavedTool("default", "non-existent")
	require.NoError(t, err)
	assert.False(t, removed)
}
