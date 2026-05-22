// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package sdk

import (
	"context"
	"testing"
)

// --- Skills ---

func TestMemStoreAddSkill(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	skill, err := store.AddSkill(ctx, "test-skill", "A test skill", []string{"trigger1", "trigger2"}, "testing", "skill content here", 3)
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}
	if skill.Id == "" {
		t.Error("AddSkill: Id is empty")
	}
	if skill.Name != "test-skill" {
		t.Errorf("AddSkill: name = %q", skill.Name)
	}
	if skill.Description != "A test skill" {
		t.Errorf("AddSkill: description = %q", skill.Description)
	}
	if len(skill.Triggers) != 2 {
		t.Errorf("AddSkill: %d triggers, want 2", len(skill.Triggers))
	}
	if skill.Category != "testing" {
		t.Errorf("AddSkill: category = %q", skill.Category)
	}
	if skill.Priority != 3 {
		t.Errorf("AddSkill: priority = %d", skill.Priority)
	}
	if skill.SourceAgent != "test-agent" {
		t.Errorf("AddSkill: SourceAgent = %q", skill.SourceAgent)
	}
}

func TestMemStoreGetSkill(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	added, _ := store.AddSkill(ctx, "my-skill", "desc", nil, "cat", "content", 1)

	got, err := store.GetSkill(ctx, added.Id)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if got.Id != added.Id {
		t.Errorf("GetSkill: ID mismatch")
	}
}

func TestMemStoreGetSkillNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	_, err := store.GetSkill(context.Background(), "nonexistent")
	if err == nil {
		t.Error("GetSkill(nonexistent): expected error")
	}
}

func TestMemStoreListSkills(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.AddSkill(ctx, "skill-a", "desc a", nil, "cat-x", "content", 1)
	store.AddSkill(ctx, "skill-b", "desc b", nil, "cat-y", "content", 2)
	store.AddSkill(ctx, "skill-c", "desc c", nil, "cat-x", "content", 3)

	// All.
	all, err := store.ListSkills(ctx, "", 10)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListSkills(all): got %d, want 3", len(all))
	}

	// Filter by category.
	skills, err := store.ListSkills(ctx, "cat-x", 10)
	if err != nil {
		t.Fatalf("ListSkills(cat-x): %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("ListSkills(cat-x): got %d, want 2", len(skills))
	}

	// Limit.
	skills, err = store.ListSkills(ctx, "", 1)
	if err != nil {
		t.Fatalf("ListSkills(limit=1): %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("ListSkills(limit=1): got %d, want 1", len(skills))
	}
}

func TestMemStoreSearchSkills(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	store.AddSkill(ctx, "test-skill", "A test skill", nil, "testing", "content", 1)
	store.AddSkill(ctx, "other-skill", "Something else", nil, "other", "content", 2)

	// Search by name.
	results, err := store.SearchSkills(ctx, "test", "", 10)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchSkills('test'): got %d, want 1", len(results))
	}

	// Search by description.
	results, err = store.SearchSkills(ctx, "else", "", 10)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchSkills('else'): got %d, want 1", len(results))
	}

	// No match.
	results, err = store.SearchSkills(ctx, "zzz", "", 10)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchSkills('zzz'): got %d, want 0", len(results))
	}
}

func TestMemStoreRemoveSkill(t *testing.T) {
	store := NewMemStore("test-agent")
	ctx := context.Background()

	skill, _ := store.AddSkill(ctx, "remove-me", "desc", nil, "cat", "content", 1)

	ok, err := store.RemoveSkill(ctx, skill.Id)
	if err != nil {
		t.Fatalf("RemoveSkill: %v", err)
	}
	if !ok {
		t.Error("RemoveSkill: returned false for existing skill")
	}

	// Verify gone.
	_, err = store.GetSkill(ctx, skill.Id)
	if err == nil {
		t.Error("RemoveSkill: skill still exists after removal")
	}
}

func TestMemStoreRemoveSkillNonexistent(t *testing.T) {
	store := NewMemStore("test-agent")
	ok, err := store.RemoveSkill(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("RemoveSkill(nonexistent): unexpected error: %v", err)
	}
	if ok {
		t.Error("RemoveSkill(nonexistent): returned true, want false")
	}
}
