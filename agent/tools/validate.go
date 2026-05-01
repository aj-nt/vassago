package tools

import (
	"fmt"
	"strings"
)

// ValidateArgs validates tool call arguments against the tool's JSON Schema.
// Returns nil if valid, or an error describing the validation failure.
// This is a lightweight validator covering the JSON Schema subset used by Mnemosyne tools:
// required, type, enum, minimum/maximum, default.
func ValidateArgs(toolName string, args map[string]interface{}, schema map[string]interface{}) error {
	if args == nil {
		args = map[string]interface{}{}
	}

	params, ok := schema["parameters"].(map[string]interface{})
	if !ok {
		return nil // no parameters defined = anything goes
	}

	// Check required fields
	required, _ := params["required"].([]string)
	for _, req := range required {
		if _, exists := args[req]; !exists {
			return &ValidationError{
				Tool:  toolName,
				Field: req,
				Issue: "required field missing",
			}
		}
	}

	// Validate each provided field against the schema
	properties, _ := params["properties"].(map[string]interface{})
	for key, value := range args {
		propSchema, hasSchema := properties[key]
		if !hasSchema {
			continue // unknown fields are allowed (forward compatibility)
		}
		propDef, _ := propSchema.(map[string]interface{})
		if propDef == nil {
			continue
		}

		if err := validateField(toolName, key, value, propDef); err != nil {
			return err
		}
	}

	return nil
}

// ApplyDefaults fills in default values from the schema for missing arguments.
// Modifies args in place and returns the number of defaults applied.
func ApplyDefaults(args map[string]interface{}, schema map[string]interface{}) int {
	if args == nil {
		args = map[string]interface{}{}
	}

	params, ok := schema["parameters"].(map[string]interface{})
	if !ok {
		return 0
	}

	properties, _ := params["properties"].(map[string]interface{})
	applied := 0
	for key, propDef := range properties {
		prop, _ := propDef.(map[string]interface{})
		if prop == nil {
			continue
		}
		if _, exists := args[key]; !exists {
			if defaultVal, hasDefault := prop["default"]; hasDefault {
				args[key] = defaultVal
				applied++
			}
		}
	}
	return applied
}

// validateField checks a single field value against its schema definition.
func validateField(tool, key string, value interface{}, schema map[string]interface{}) error {
	// Type check
	expectedType, _ := schema["type"].(string)
	if expectedType != "" {
		if err := checkType(key, value, expectedType); err != nil {
			return &ValidationError{
				Tool:  tool,
				Field: key,
				Issue: err.Error(),
			}
		}
	}

	// Enum check
	if enumVals, ok := schema["enum"].([]string); ok && len(enumVals) > 0 {
		strVal, _ := value.(string)
		if !enumContains(enumVals, strVal) {
			return &ValidationError{
				Tool:  tool,
				Field: key,
				Issue: fmt.Sprintf("value %q not in enum %s", strVal, formatEnum(enumVals)),
			}
		}
	}
	// Also handle enum as []interface{} (JSON unmarshaling)
	if enumRaw, ok := schema["enum"].([]interface{}); ok && len(enumRaw) > 0 {
		enumVals := make([]string, len(enumRaw))
		for i, v := range enumRaw {
			enumVals[i], _ = v.(string)
		}
		strVal, _ := value.(string)
		if !enumContains(enumVals, strVal) {
			return &ValidationError{
				Tool:  tool,
				Field: key,
				Issue: fmt.Sprintf("value %q not in enum %s", strVal, formatEnum(enumVals)),
			}
		}
	}

	// Minimum check (for integers and floats)
	minVal, hasMin := toFloatFromSchema(schema["minimum"])
	if hasMin {
		if val, ok := toFloat(value); ok && val < minVal {
			return &ValidationError{
				Tool:  tool,
				Field: key,
				Issue: fmt.Sprintf("value %v is below minimum %v", val, minVal),
			}
		}
	}

	// Maximum check (for integers and floats)
	maxVal, hasMax := toFloatFromSchema(schema["maximum"])
	if hasMax {
		if val, ok := toFloat(value); ok && val > maxVal {
			return &ValidationError{
				Tool:  tool,
				Field: key,
				Issue: fmt.Sprintf("value %v exceeds maximum %v", val, maxVal),
			}
		}
	}

	return nil
}

// checkType validates that a value matches the expected JSON Schema type.
func checkType(key string, value interface{}, expected string) error {
	switch expected {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "integer":
		switch value.(type) {
		case int, int32, int64, float64:
			// JSON numbers unmarshal to float64, check if it's a whole number
			if f, ok := value.(float64); ok && f != float64(int64(f)) {
				return fmt.Errorf("expected integer, got float")
			}
		default:
			return fmt.Errorf("expected integer, got %T", value)
		}
	case "number":
		switch value.(type) {
		case int, int32, int64, float64:
			// ok
		default:
			return fmt.Errorf("expected number, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	}
	return nil
}

// enumContains checks if a string is in a slice.
func enumContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// formatEnum formats enum values for error messages.
func formatEnum(vals []string) string {
	return "[" + strings.Join(vals, ", ") + "]"
}

// toFloat converts any numeric type to float64 for comparison.
// Returns (value, true) for numeric types, (0, false) otherwise.
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

// toFloatFromSchema extracts a numeric value from schema metadata.
// YAML/JSON numbers parse to float64, but Go literals may be int.
func toFloatFromSchema(v interface{}) (float64, bool) {
	return toFloat(v)
}

// ValidationError describes a tool argument validation failure.
type ValidationError struct {
	Tool  string // Tool name
	Field string // Argument name
	Issue string // What went wrong
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s.%s: %s", e.Tool, e.Field, e.Issue)
}

// ToolResult creates a tool result string from a validation error.
func (e *ValidationError) ToolResult() string {
	return fmt.Sprintf("Invalid argument for %s: %s", e.Tool, e.Issue)
}