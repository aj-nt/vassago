package tools

import (
	"testing"
)

func TestRegisterCronTools(t *testing.T) {
	registry := NewToolRegistry()
	RegisterCronTools(registry, nil, "test-agent")

	handler, err := registry.Get("ScheduleCron")
	if err != nil {
		t.Fatalf("Expected ScheduleCron to be registered: %v", err)
	}
	if handler == nil {
		t.Fatal("Expected ScheduleCron handler to not be nil")
	}
}

func TestScheduleCronSchema(t *testing.T) {
	// Verify schema has required fields
	action, ok := ScheduleCronSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("ScheduleCronSchema should have properties")
	}

	// Verify action property
	actionProp, ok := action["action"].(map[string]interface{})
	if !ok {
		t.Fatal("action property should be a map")
	}
	if actionProp["type"] != "string" {
		t.Error("action property should be type string")
	}

	// Verify required fields
	required, ok := ScheduleCronSchema["required"].([]string)
	if !ok {
		t.Fatal("ScheduleCronSchema should have required fields")
	}
	if len(required) != 1 || required[0] != "action" {
		t.Errorf("ScheduleCronSchema should require 'action', got %v", required)
	}

	// Verify all expected properties exist
	expectedProps := []string{"action", "name", "schedule", "message", "job_id", "enabled", "include_disabled"}
	for _, prop := range expectedProps {
		if _, ok := action[prop]; !ok {
			t.Errorf("ScheduleCronSchema missing property: %s", prop)
		}
	}
}

func TestScheduleCronSchema_ActionEnum(t *testing.T) {
	props, _ := ScheduleCronSchema["properties"].(map[string]interface{})
	actionProp, _ := props["action"].(map[string]interface{})
	enum, ok := actionProp["enum"].([]string)
	if !ok {
		t.Fatal("action property should have enum")
	}
	expectedEnum := []string{"create", "list", "update", "delete"}
	if len(enum) != len(expectedEnum) {
		t.Fatalf("expected %d enum values, got %d", len(expectedEnum), len(enum))
	}
	for i, v := range expectedEnum {
		if enum[i] != v {
			t.Errorf("enum[%d] = %q, want %q", i, enum[i], v)
		}
	}
}