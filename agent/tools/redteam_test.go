// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"strings"
	"testing"
)

// --- RED TEAM QA: Tool Input Validation ---

func TestRedTeam_Validation_SQLInjection(t *testing.T) {
	t.Parallel()

	injectionStrings := []string{
		"'; DROP TABLE memories; --",
		"' OR '1'='1",
		"Robert'); DROP TABLE students; --",
		"${7*7}",
		"{{7*7}}",
		"<script>alert('xss')</script>",
	}

	for _, inj := range injectionStrings {
		t.Run("key_"+inj, func(t *testing.T) {
			args := map[string]interface{}{
				"key":      inj,
				"content":  "test content",
				"category": "project",
				"target":   "memory",
			}
			schema := AddMemorySchema
			// ValidateArgs should not crash on injection strings
			err := ValidateArgs("AddMemory", args, schema)
			if err != nil {
				t.Logf("Validation rejected key %q: %v (acceptable)", inj, err)
			}
		})

		t.Run("content_"+inj, func(t *testing.T) {
			args := map[string]interface{}{
				"key":      "test-key",
				"content":  inj,
				"category": "project",
				"target":   "memory",
			}
			schema := AddMemorySchema
			err := ValidateArgs("AddMemory", args, schema)
			if err != nil {
				t.Errorf("Validation rejected content %q: %v (content injection strings should be allowed)", inj, err)
			}
		})
	}
}

func TestRedTeam_Validation_TypeConfusion(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{"PriorityAsString", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "memory", "priority": "high",
		}},
		{"PriorityAsBool", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "memory", "priority": true,
		}},
		{"PriorityAsFloat", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "memory", "priority": 3.5,
		}},
		{"PriorityAsNegative", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "memory", "priority": -1,
		}},
		{"PriorityAsZero", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "memory", "priority": 0,
		}},
		{"PriorityAsHuge", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "memory", "priority": 99999,
		}},
		{"TargetAsNumber", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": 42, "priority": 3,
		}},
		{"EmptyTarget", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "", "priority": 3,
		}},
		{"WrongTarget", map[string]interface{}{
			"key": "k", "content": "c", "category": "cat", "target": "invalid_target", "priority": 3,
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateArgs("AddMemory", tc.args, schema)
			if err != nil {
				t.Logf("Validation caught %s: %v (good)", tc.name, err)
			} else {
				t.Logf("Validation passed for %s (may be a finding if type confusion is dangerous)", tc.name)
			}
		})
	}
}

func TestRedTeam_Validation_MissingRequired(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{"NoKey", map[string]interface{}{"content": "c", "category": "project", "target": "memory"}, true},
		{"NoContent", map[string]interface{}{"key": "k", "category": "project", "target": "memory"}, true},
		{"NoTarget", map[string]interface{}{"key": "k", "content": "c", "category": "project"}, false}, // target is optional (default: "memory")
		{"NoCategory", map[string]interface{}{"key": "k", "content": "c", "target": "memory"}, true},
		{"EmptyArgs", map[string]interface{}{}, true},
		{"NilArgs", nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateArgs("AddMemory", tc.args, schema)
			if tc.wantErr && err == nil {
				t.Errorf("Validation should have rejected %s but got nil error", tc.name)
			} else if !tc.wantErr && err != nil {
				t.Errorf("Validation unexpectedly rejected %s: %v", tc.name, err)
			}
		})
	}
}

func TestRedTeam_Validation_DefaultsApplied(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	// Test that ApplyDefaults fills in missing optional fields
	args := map[string]interface{}{
		"key":      "test-key",
		"content":  "test content",
		"category": "project",
		"target":   "memory",
		// priority should get default of 3
		// limit and char_limit may or may not get defaults
	}

	ApplyDefaults(args, schema)

	if priority, ok := args["priority"]; !ok {
		t.Log("FINDING: ApplyDefaults does not add default priority")
	} else {
		t.Logf("Default priority: %v", priority)
	}

	if limit, ok := args["limit"]; !ok {
		t.Log("AddMemory schema has no 'limit' property (limit is on SearchMemories)")
	} else {
		t.Logf("Default limit: %v", limit)
	}
}

func TestRedTeam_Validation_NullBytes(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	args := map[string]interface{}{
		"key":      "test\x00key",
		"content":  "content with\x00null bytes",
		"category": "project",
		"target":   "memory",
	}

	err := ValidateArgs("AddMemory", args, schema)
	if err == nil {
		t.Error("FINDING: Validation allows null bytes in key/content/category — they should be rejected")
	} else {
		t.Logf("Validation correctly rejected null bytes: %v", err)
	}
}

func TestRedTeam_Validation_VeryLongInput(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	// 100K character content — should be accepted by validation (store enforces size limit)
	bigContent := strings.Repeat("x", 100000)
	args := map[string]interface{}{
		"key":      "long-input-test",
		"content":  bigContent,
		"category": "project",
		"target":   "memory",
	}

	err := ValidateArgs("AddMemory", args, schema)
	if err != nil {
		t.Errorf("Validation rejected 100K char content: %v (store should handle size limit)", err)
	}

	// 10K character key
	longKey := strings.Repeat("k", 10000)
	args2 := map[string]interface{}{
		"key":      longKey,
		"content":  "test",
		"category": "project",
		"target":   "memory",
	}

	err2 := ValidateArgs("AddMemory", args2, schema)
	if err2 != nil {
		t.Logf("Validation rejected 10K key: %v", err2)
	} else {
		t.Log("FINDING: Validation accepts 10K character key — store may handle this but it's unusual")
	}
}

func TestRedTeam_Validation_UnknownTool(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	// Calling ValidateArgs with a tool name that doesn't match the schema
	args := map[string]interface{}{
		"key":      "k",
		"content":  "c",
		"category": "cat",
		"target":   "memory",
	}

	// This should just validate the schema — tool name doesn't affect validation
	err := ValidateArgs("NonExistentTool", args, schema)
	if err != nil {
		t.Logf("Validation rejected unknown tool: %v", err)
	} else {
		t.Log("Validation passed for unknown tool name (schema validation is tool-agnostic)")
	}
}

func TestRedTeam_Validation_EnumViolation(t *testing.T) {
	t.Parallel()

	schema := AddMemorySchema

	tests := []struct {
		name   string
		target string
		valid  bool
	}{
		{"ValidMemory", "memory", true},
		{"ValidUser", "user", true},
		{"InvalidTarget", "invalid_target", false},
		{"EmptyTarget", "", false},
		{"SQLInjectionTarget", "'; DROP TABLE", false},
		{"NumberTarget", "123", false},
		{"MixedCase", "Memory", false}, // enum is case-sensitive
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]interface{}{
				"key":      "test-key",
				"content":  "test content",
				"category": "project",
				"target":   tc.target,
			}

			err := ValidateArgs("AddMemory", args, schema)
			if tc.valid {
				if err != nil {
					t.Errorf("Validation rejected valid target %q: %v", tc.target, err)
				}
			} else {
				if err == nil {
					t.Errorf("Validation accepted invalid target %q (should reject)", tc.target)
				} else {
					t.Logf("Validation correctly rejected target %q: %v", tc.target, err)
				}
			}
		})
	}
}

func TestRedTeam_CodeExec_Validation(t *testing.T) {
	t.Parallel()

	schema := ExecuteCodeSchema

	// Code must be a string and required
	t.Run("MissingCode", func(t *testing.T) {
		err := ValidateArgs("ExecuteCode", map[string]interface{}{}, schema)
		if err == nil {
			t.Error("Should reject missing code argument")
		}
	})

	t.Run("CodeAsNumber", func(t *testing.T) {
		err := ValidateArgs("ExecuteCode", map[string]interface{}{"code": 42}, schema)
		if err == nil {
			t.Error("Should reject non-string code argument")
		}
	})

	t.Run("ValidCode", func(t *testing.T) {
		err := ValidateArgs("ExecuteCode", map[string]interface{}{"code": "print('hello')"}, schema)
		if err != nil {
			t.Errorf("Should accept valid code: %v", err)
		}
	})

	t.Run("TimeoutAsString", func(t *testing.T) {
		err := ValidateArgs("ExecuteCode", map[string]interface{}{"code": "print()", "timeout": "abc"}, schema)
		if err == nil {
			t.Error("Should reject string timeout")
		}
	})

	t.Run("WorkdirAsNumber", func(t *testing.T) {
		err := ValidateArgs("ExecuteCode", map[string]interface{}{"code": "print()", "workdir": 123}, schema)
		if err == nil {
			t.Error("Should reject non-string workdir")
		}
	})

}