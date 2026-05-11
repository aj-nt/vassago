// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddSkill(t *testing.T) {
	store := testStore(t)

	params := AddSkillParams{
		Name:        "Test Skill",
		Description: "A test skill",
		Triggers:    []string{"test", "demo"},
		Category:    "test",
		Content:     "This is test content",
		SourceAgent: "test-agent",
		Priority:    3,
	}

	skill, err := store.AddSkill(params)
	require.NoError(t, err)
	assert.NotEmpty(t, skill.ID)
	assert.Equal(t, "Test Skill", skill.Name)
	assert.Equal(t, "A test skill", skill.Description)
	assert.Equal(t, []string{"test", "demo"}, skill.Triggers)
	assert.Equal(t, "test", skill.Category)
	assert.Equal(t, "This is test content", skill.Content)
	assert.Equal(t, "test-agent", skill.SourceAgent)
	assert.Equal(t, int32(3), skill.Priority)
	assert.NotZero(t, skill.CreatedAt)
	assert.NotZero(t, skill.UpdatedAt)
}

func TestGetSkill(t *testing.T) {
	store := testStore(t)

	// First add a skill
	params := AddSkillParams{
		Name:        "Get Test Skill",
		Description: "A skill to test get",
		Triggers:    []string{"get", "test"},
		Category:    "test",
		Content:     "Get test content",
		SourceAgent: "test-agent",
		Priority:    2,
	}

	skill, err := store.AddSkill(params)
	require.NoError(t, err)

	// Now retrieve it
	retrieved, err := store.GetSkillByID(skill.ID)
	require.NoError(t, err)
	assert.Equal(t, skill.ID, retrieved.ID)
	assert.Equal(t, skill.Name, retrieved.Name)
	assert.Equal(t, skill.Description, retrieved.Description)
	assert.Equal(t, skill.Triggers, retrieved.Triggers)
	assert.Equal(t, skill.Category, retrieved.Category)
	assert.Equal(t, skill.Content, retrieved.Content)
	assert.Equal(t, skill.SourceAgent, retrieved.SourceAgent)
	assert.Equal(t, skill.Priority, retrieved.Priority)

	// Test getting non-existent skill
	_, err = store.GetSkillByID("non-existent-id")
	assert.Error(t, err)
}

func TestListSkills(t *testing.T) {
	store := testStore(t)

	// Add multiple skills
	params1 := AddSkillParams{
		Name:        "List Skill 1",
		Description: "First skill",
		Triggers:    []string{"list1"},
		Category:    "category1",
		Content:     "Content 1",
		SourceAgent: "agent1",
		Priority:    1,
	}

	params2 := AddSkillParams{
		Name:        "List Skill 2",
		Description: "Second skill",
		Triggers:    []string{"list2"},
		Category:    "category2",
		Content:     "Content 2",
		SourceAgent: "agent2",
		Priority:    2,
	}

	_, err := store.AddSkill(params1)
	require.NoError(t, err)
	_, err = store.AddSkill(params2)
	require.NoError(t, err)

	// List all skills
	allSkills, err := store.ListSkills("default", "", "", 0)
	require.NoError(t, err)
	assert.Len(t, allSkills, 2)

	// Filter by category
	category1Skills, err := store.ListSkills("default", "category1", "", 0)
	require.NoError(t, err)
	assert.Len(t, category1Skills, 1)
	assert.Equal(t, "category1", category1Skills[0].Category)

	// Filter by source agent
	agent1Skills, err := store.ListSkills("default", "", "agent1", 0)
	require.NoError(t, err)
	assert.Len(t, agent1Skills, 1)
	assert.Equal(t, "agent1", agent1Skills[0].SourceAgent)

	// Test limit
	limitedSkills, err := store.ListSkills("default", "", "", 1)
	require.NoError(t, err)
	assert.Len(t, limitedSkills, 1)
}

func TestRemoveSkill(t *testing.T) {
	store := testStore(t)

	// Add a skill
	params := AddSkillParams{
		Name:        "Remove Test Skill",
		Description: "A skill to test removal",
		Triggers:    []string{"remove"},
		Category:    "test",
		Content:     "Remove test content",
		SourceAgent: "test-agent",
		Priority:    4,
	}

	skill, err := store.AddSkill(params)
	require.NoError(t, err)

	// Remove it
	success, err := store.RemoveSkill("default", skill.ID)
	require.NoError(t, err)
	assert.True(t, success)

	// Try to get the removed skill
	_, err = store.GetSkillByID(skill.ID)
	assert.Error(t, err)

	// Try to remove non-existent skill
	success, err = store.RemoveSkill("default", "non-existent-id")
	require.NoError(t, err)
	assert.False(t, success)
}

func TestSearchSkills(t *testing.T) {
	store := testStore(t)

	// Add skills with searchable content
	params1 := AddSkillParams{
		Name:        "Search Skill 1",
		Description: "First searchable skill",
		Triggers:    []string{"search1", "find"},
		Category:    "search",
		Content:     "This skill contains searchable content about databases",
		SourceAgent: "search-agent",
		Priority:    3,
	}

	params2 := AddSkillParams{
		Name:        "Search Skill 2",
		Description: "Second searchable skill",
		Triggers:    []string{"search2", "query"},
		Category:    "search",
		Content:     "This skill contains searchable content about algorithms",
		SourceAgent: "search-agent",
		Priority:    2,
	}

	_, err := store.AddSkill(params1)
	require.NoError(t, err)
	_, err = store.AddSkill(params2)
	require.NoError(t, err)

	// Search for skills
	results, err := store.SearchSkills("default", "database", "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "databases")

	results, err = store.SearchSkills("default", "algorithm", "", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Content, "algorithms")

	// Search with category filter
	results, err = store.SearchSkills("default", "searchable", "search", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Test empty query
	_, err = store.SearchSkills("default", "", "", 10)
	assert.Error(t, err)
}