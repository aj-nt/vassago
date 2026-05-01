package tools

import (
	"testing"
)

func TestValidateArgs_RequiredField(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
			"required": []string{"query"},
		},
	}

	err := ValidateArgs("TestTool", map[string]interface{}{}, schema)
	if err == nil {
		t.Error("Expected validation error for missing required field")
	}
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Expected ValidationError, got %T", err)
	}
	if verr.Field != "query" {
		t.Errorf("Expected field 'query', got %q", verr.Field)
	}
}

func TestValidateArgs_ValidRequired(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
			"required": []string{"query"},
		},
	}

	err := ValidateArgs("TestTool", map[string]interface{}{
		"query": "hello",
	}, schema)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestValidateArgs_TypeMismatch(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"count": map[string]interface{}{"type": "integer"},
			},
		},
	}

	// Pass a string where integer is expected
	err := ValidateArgs("TestTool", map[string]interface{}{
		"count": "not-a-number",
	}, schema)
	if err == nil {
		t.Error("Expected validation error for type mismatch")
	}
	verr, _ := err.(*ValidationError)
	if verr.Field != "count" {
		t.Errorf("Expected field 'count', got %q", verr.Field)
	}
}

func TestValidateArgs_EnumValidation(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "AddMemory",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target": map[string]interface{}{"type": "string", "enum": []string{"memory", "user"}},
			},
		},
	}

	// Valid enum value
	err := ValidateArgs("AddMemory", map[string]interface{}{
		"target": "memory",
	}, schema)
	if err != nil {
		t.Errorf("Expected no error for valid enum, got %v", err)
	}

	// Invalid enum value
	err = ValidateArgs("AddMemory", map[string]interface{}{
		"target": "invalid",
	}, schema)
	if err == nil {
		t.Error("Expected validation error for invalid enum value")
	}
}

func TestValidateArgs_MinimumMaximum(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"priority": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 5},
			},
		},
	}

	// Valid value
	err := ValidateArgs("TestTool", map[string]interface{}{
		"priority": 3,
	}, schema)
	if err != nil {
		t.Errorf("Expected no error for valid value, got %v", err)
	}

	// Below minimum
	err = ValidateArgs("TestTool", map[string]interface{}{
		"priority": 0,
	}, schema)
	if err == nil {
		t.Error("Expected error for value below minimum")
	}

	// Above maximum
	err = ValidateArgs("TestTool", map[string]interface{}{
		"priority": 6,
	}, schema)
	if err == nil {
		t.Error("Expected error for value above maximum")
	}
}

func TestValidateArgs_FloatFromJSON(t *testing.T) {
	// JSON numbers unmarshal to float64 — integer validation should accept 3.0
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"priority": map[string]interface{}{"type": "integer"},
			},
		},
	}

	err := ValidateArgs("TestTool", map[string]interface{}{
		"priority": float64(3), // JSON number unmarshals to float64
	}, schema)
	if err != nil {
		t.Errorf("Expected float64(3) to pass integer validation, got %v", err)
	}
}

func TestValidateArgs_UnknownFieldsAllowed(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
		},
	}

	// Extra fields should be allowed (forward compatibility)
	err := ValidateArgs("TestTool", map[string]interface{}{
		"query":         "hello",
		"unknown_field": "whatever",
	}, schema)
	if err != nil {
		t.Errorf("Expected no error for unknown field, got %v", err)
	}
}

func TestValidateArgs_NoSchema(t *testing.T) {
	// Empty schema — anything goes
	err := ValidateArgs("TestTool", map[string]interface{}{
		"whatever": "ok",
	}, nil)
	if err != nil {
		t.Errorf("Expected no error with nil schema, got %v", err)
	}
}

func TestApplyDefaults(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "SearchMemories",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":  map[string]interface{}{"type": "string"},
				"target": map[string]interface{}{"type": "string", "default": "memory"},
				"limit":  map[string]interface{}{"type": "integer", "default": 20},
			},
			"required": []string{"query"},
		},
	}

	args := map[string]interface{}{
		"query": "test",
	}
	applied := ApplyDefaults(args, schema)

	if applied != 2 {
		t.Errorf("Expected 2 defaults applied, got %d", applied)
	}
	if args["target"] != "memory" {
		t.Errorf("Expected target='memory', got %v", args["target"])
	}
	if args["limit"] != 20 {
		t.Errorf("Expected limit=20, got %v", args["limit"])
	}
	// Existing values should not be overwritten
	args2 := map[string]interface{}{
		"query":  "test",
		"target": "user",
		"limit":  5,
	}
	applied2 := ApplyDefaults(args2, schema)
	if applied2 != 0 {
		t.Errorf("Expected 0 defaults applied when all values present, got %d", applied2)
	}
}

func TestApplyDefaults_NoDefaults(t *testing.T) {
	schema := map[string]interface{}{
		"name":       "TestTool",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
			"required": []string{"query"},
		},
	}

	args := map[string]interface{}{
		"query": "test",
	}
	applied := ApplyDefaults(args, schema)
	if applied != 0 {
		t.Errorf("Expected 0 defaults, got %d", applied)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{Tool: "AddMemory", Field: "target", Issue: "value \"invalid\" not in enum [memory, user]"}
	expected := `AddMemory.target: value "invalid" not in enum [memory, user]`
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestValidationError_ToolResult(t *testing.T) {
	err := &ValidationError{Tool: "AddMemory", Field: "key", Issue: "required field missing"}
	result := err.ToolResult()
	if result != "Invalid argument for AddMemory: required field missing" {
		t.Errorf("Unexpected tool result: %q", result)
	}
}

func TestValidateArgs_AddMemorySchema(t *testing.T) {
	// Test against the actual AddMemory schema
	err := ValidateArgs("AddMemory", map[string]interface{}{
		"key":      "test-key",
		"content":  "test content",
		"target":   "memory",
		"category": "environment",
		"priority": 3,
	}, AddMemorySchema)
	if err != nil {
		t.Errorf("Expected valid AddMemory args to pass, got %v", err)
	}

	// Missing required field
	err = ValidateArgs("AddMemory", map[string]interface{}{
		"content": "test content",
	}, AddMemorySchema)
	if err == nil {
		t.Error("Expected error for missing 'key' field")
	}

	// Invalid enum
	err = ValidateArgs("AddMemory", map[string]interface{}{
		"key":      "test",
		"content":  "test",
		"target":   "invalid",
	}, AddMemorySchema)
	if err == nil {
		t.Error("Expected error for invalid target enum")
	}

	// Priority out of range
	err = ValidateArgs("AddMemory", map[string]interface{}{
		"key":      "test",
		"content":  "test",
		"priority": 10,
	}, AddMemorySchema)
	if err == nil {
		t.Error("Expected error for priority > 5")
	}
}