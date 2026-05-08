// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"testing"
)

func TestRegisterSkillTools(t *testing.T) {
	registry := NewToolRegistry()
	RegisterSkillTools(registry, nil, "test-agent")

	for _, name := range []string{"AddSkill", "GetSkill", "ListSkills", "SearchSkills", "RemoveSkill"} {
		handler, err := registry.Get(name)
		if err != nil {
			t.Errorf("Expected %s to be registered: %v", name, err)
		}
		if handler == nil {
			t.Errorf("Expected %s handler to not be nil", name)
		}
	}
}

func TestAddSkillSchema(t *testing.T) {
	name, ok := AddSkillSchema["name"]
	if !ok || name != "AddSkill" {
		t.Error("AddSkillSchema should have name 'AddSkill'")
	}

	params, ok := AddSkillSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("AddSkillSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("AddSkillSchema parameters should have required")
	}
	if len(required) != 5 {
		t.Errorf("AddSkillSchema should require 5 fields, got %d", len(required))
	}
	expectedRequired := map[string]bool{
		"name": true, "description": true, "triggers": true, "category": true, "content": true,
	}
	for _, r := range required {
		if !expectedRequired[r] {
			t.Errorf("unexpected required field %q", r)
		}
	}
}

func TestGetSkillSchema(t *testing.T) {
	name, ok := GetSkillSchema["name"]
	if !ok || name != "GetSkill" {
		t.Error("GetSkillSchema should have name 'GetSkill'")
	}

	params, ok := GetSkillSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("GetSkillSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("GetSkillSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "id" {
		t.Errorf("GetSkillSchema should require 'id', got %v", required)
	}
}

func TestSearchSkillsSchema(t *testing.T) {
	name, ok := SearchSkillsSchema["name"]
	if !ok || name != "SearchSkills" {
		t.Error("SearchSkillsSchema should have name 'SearchSkills'")
	}

	params, ok := SearchSkillsSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("SearchSkillsSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("SearchSkillsSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("SearchSkillsSchema should require 'query', got %v", required)
	}
}

func TestRemoveSkillSchema(t *testing.T) {
	name, ok := RemoveSkillSchema["name"]
	if !ok || name != "RemoveSkill" {
		t.Error("RemoveSkillSchema should have name 'RemoveSkill'")
	}

	params, ok := RemoveSkillSchema["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("RemoveSkillSchema should have parameters")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("RemoveSkillSchema parameters should have required")
	}
	if len(required) != 1 || required[0] != "id" {
		t.Errorf("RemoveSkillSchema should require 'id', got %v", required)
	}
}