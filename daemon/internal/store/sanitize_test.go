package store

import (
	"testing"
	"unicode/utf8"
)

func TestSanitizeUTF8_ValidPassthrough(t *testing.T) {
	input := "Hello, world! 你好 🌍"
	result := sanitizeUTF8(input)
	if result != input {
		t.Errorf("Valid UTF-8 should pass through unchanged: got %q", result)
	}
}

func TestSanitizeUTF8_EmptyString(t *testing.T) {
	result := sanitizeUTF8("")
	if result != "" {
		t.Errorf("Empty string should return empty, got %q", result)
	}
}

func TestSanitizeUTF8_InvalidBytes(t *testing.T) {
	// Invalid UTF-8: \xff is never a valid lead byte
	input := "Hello\xffWorld"
	result := sanitizeUTF8(input)

	if !utf8.ValidString(result) {
		t.Errorf("Result should be valid UTF-8, got invalid bytes")
	}
	if result != "Hello\uFFFDWorld" {
		t.Errorf("Expected replacement character for invalid byte, got %q", result)
	}
}

func TestSanitizeUTF8_MultipleInvalidBytes(t *testing.T) {
	// Multiple invalid bytes
	input := "A\xfe\x80B\xffc"
	result := sanitizeUTF8(input)

	if !utf8.ValidString(result) {
		t.Errorf("Result should be valid UTF-8, got invalid bytes")
	}
	if result != "A\uFFFD\uFFFDB\uFFFDc" {
		t.Errorf("Expected replacement characters for each invalid byte, got %q", result)
	}
}

func TestSanitizeUTF8_ValidMultiByte(t *testing.T) {
	// Valid multi-byte sequences should be preserved
	input := "émoji 🎉 café"
	result := sanitizeUTF8(input)
	if result != input {
		t.Errorf("Valid multi-byte UTF-8 should pass through: got %q", result)
	}
}

func TestSanitizeUTF8_TruncatedMultiByte(t *testing.T) {
	// Truncated 3-byte sequence: only first byte of a 3-byte char
	input := "start\xe4\xb8end" // \xe4\xb8 is truncated (needs \xe4\xb8\xad for 中)
	result := sanitizeUTF8(input)

	if !utf8.ValidString(result) {
		t.Errorf("Result should be valid UTF-8, got invalid bytes")
	}
}

func TestSanitizeUTF8_MixedValidInvalid(t *testing.T) {
	// Mix of valid Chinese, invalid byte, more valid text
	input := "你好\x00bad世界"
	result := sanitizeUTF8(input)

	if !utf8.ValidString(result) {
		t.Errorf("Result should be valid UTF-8, got invalid bytes")
	}
}