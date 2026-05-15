// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package grpc

import (
	"context"
	"testing"

	pb "github.com/aj-nt/mnemosyne/proto"
)

func TestAddSkill(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	skill, err := svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name:        "Test Skill",
		Description: "A test skill",
		Triggers:    []string{"test", "demo"},
		Category:    "test",
		Content:     "This is test content",
		SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}
	if skill.Id == "" {
		t.Error("expected non-empty skill ID")
	}
	if skill.Name != "Test Skill" {
		t.Errorf("expected name 'Test Skill', got %q", skill.Name)
	}
	if skill.Category != "test" {
		t.Errorf("expected category 'test', got %q", skill.Category)
	}
	if skill.Priority != 3 {
		t.Errorf("expected default priority 3, got %d", skill.Priority)
	}
}

func TestAddSkill_DefaultPriority(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	priority := int32(5)
	skill, err := svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name:        "High Priority",
		Description: "A high priority skill",
		Triggers:    []string{"urgent"},
		Category:    "ops",
		Content:     "Do this urgently",
		SourceAgent: "test-agent",
		Priority:    &priority,
	})
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}
	if skill.Priority != 5 {
		t.Errorf("expected priority 5, got %d", skill.Priority)
	}
}

func TestGetSkill(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	created, err := svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name:        "Get Test",
		Description: "A skill to test get",
		Triggers:    []string{"get"},
		Category:    "test",
		Content:     "Get test content",
		SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}

	got, err := svc.GetSkill(context.Background(), &pb.GetSkillRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if got.Name != "Get Test" {
		t.Errorf("expected name 'Get Test', got %q", got.Name)
	}
	if got.Description != "A skill to test get" {
		t.Errorf("expected description 'A skill to test get', got %q", got.Description)
	}
}

func TestGetSkill_NotFound(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	_, err := svc.GetSkill(context.Background(), &pb.GetSkillRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestListSkills(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name: "List Skill 1", Description: "First", Triggers: []string{"l1"}, Category: "cat1", Content: "C1", SourceAgent: "agent1",
	})
	svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name: "List Skill 2", Description: "Second", Triggers: []string{"l2"}, Category: "cat2", Content: "C2", SourceAgent: "agent2",
	})

	// List all
	resp, err := svc.ListSkills(context.Background(), &pb.ListSkillsRequest{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(resp.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(resp.Skills))
	}

	// Filter by category
	resp, err = svc.ListSkills(context.Background(), &pb.ListSkillsRequest{Category: "cat1"})
	if err != nil {
		t.Fatalf("ListSkills by category: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("expected 1 skill in cat1, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Category != "cat1" {
		t.Errorf("expected category 'cat1', got %q", resp.Skills[0].Category)
	}

	// Filter by source agent
	resp, err = svc.ListSkills(context.Background(), &pb.ListSkillsRequest{SourceAgent: "agent2"})
	if err != nil {
		t.Fatalf("ListSkills by source_agent: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("expected 1 skill for agent2, got %d", len(resp.Skills))
	}
}

func TestRemoveSkill(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	created, err := svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name: "ToRemove", Description: "Will be removed", Triggers: []string{"rm"}, Category: "test", Content: "x", SourceAgent: "test-agent",
	})
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}

	resp, err := svc.RemoveSkill(context.Background(), &pb.RemoveSkillRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("RemoveSkill: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}

	// Verify removal
	_, err = svc.GetSkill(context.Background(), &pb.GetSkillRequest{Id: created.Id})
	if err == nil {
		t.Error("expected error getting removed skill")
	}
}

func TestRemoveSkill_NotFound(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	resp, err := svc.RemoveSkill(context.Background(), &pb.RemoveSkillRequest{Id: "nonexistent"})
	if err != nil {
		t.Fatalf("RemoveSkill nonexistent: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for nonexistent skill")
	}
}

func TestSearchSkills(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name: "DB Debug", Description: "Debug database issues", Triggers: []string{"db", "debug"}, Category: "devops", Content: "Steps to debug database connection problems", SourceAgent: "ops-agent",
	})
	svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name: "Deploy", Description: "Deploy to production", Triggers: []string{"deploy", "prod"}, Category: "devops", Content: "How to deploy to the production environment", SourceAgent: "ops-agent",
	})
	svc.AddSkill(context.Background(), &pb.AddSkillRequest{
		Name: "Write Tests", Description: "How to write Go tests", Triggers: []string{"test", "go"}, Category: "dev", Content: "Patterns for writing effective Go tests", SourceAgent: "dev-agent",
	})

	// Search by content keyword
	resp, err := svc.SearchSkills(context.Background(), &pb.SearchSkillsRequest{Query: "database"})
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("expected 1 result for 'database', got %d", len(resp.Skills))
	}

	// Search by name
	resp, err = svc.SearchSkills(context.Background(), &pb.SearchSkillsRequest{Query: "deploy"})
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("expected 1 result for 'deploy', got %d", len(resp.Skills))
	}

	// Search with category filter
	resp, err = svc.SearchSkills(context.Background(), &pb.SearchSkillsRequest{Query: "debug", Category: "devops"})
	if err != nil {
		t.Fatalf("SearchSkills with category: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("expected 1 result for 'debug' in 'devops', got %d", len(resp.Skills))
	}
}
