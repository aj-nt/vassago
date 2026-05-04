// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasteCollapse_ShouldCollapse(t *testing.T) {
	pc := NewPasteCollapse()

	t.Run("short text is not collapsed", func(t *testing.T) {
		if pc.ShouldCollapse("hello") {
			t.Error("Single line should not be collapsed")
		}
	})

	t.Run("slash commands are never collapsed", func(t *testing.T) {
		longCmd := "/edit " + strings.Repeat("x", 10000)
		if pc.ShouldCollapse(longCmd) {
			t.Error("Slash commands should never be collapsed")
		}
	})

	t.Run("5 lines triggers collapse", func(t *testing.T) {
		text := "line1\nline2\nline3\nline4\nline5"
		if !pc.ShouldCollapse(text) {
			t.Error("5 lines should trigger collapse")
		}
	})

	t.Run("4 lines does not trigger collapse", func(t *testing.T) {
		text := "line1\nline2\nline3\nline4"
		if pc.ShouldCollapse(text) {
			t.Error("4 lines should not trigger collapse")
		}
	})

	t.Run("large character count triggers collapse", func(t *testing.T) {
		text := strings.Repeat("x", 8001)
		if !pc.ShouldCollapse(text) {
			t.Error("8000+ chars should trigger collapse")
		}
	})

	t.Run("below character threshold does not trigger collapse", func(t *testing.T) {
		text := strings.Repeat("x", 7999)
		if pc.ShouldCollapse(text) {
			t.Error("7999 chars should not trigger collapse")
		}
	})

	t.Run("disabled does not collapse", func(t *testing.T) {
		pc := NewPasteCollapse()
		pc.enabled = false
		text := "line1\nline2\nline3\nline4\nline5"
		if pc.ShouldCollapse(text) {
			t.Error("Disabled PasteCollapse should not collapse")
		}
	})
}

func TestPasteCollapse_Collapse(t *testing.T) {
	pc := NewPasteCollapse()
	pc.dir = t.TempDir() // Use temp dir for tests

	t.Run("short text is returned as-is", func(t *testing.T) {
		result := pc.Collapse("hello")
		if result != "hello" {
			t.Errorf("Expected 'hello', got %q", result)
		}
	})

	t.Run("long text is collapsed to reference", func(t *testing.T) {
		text := "line1\nline2\nline3\nline4\nline5"
		result := pc.Collapse(text)

		// Should contain the line count and file path
		if !strings.Contains(result, "5 lines") {
			t.Errorf("Expected reference to contain '5 lines', got %q", result)
		}
		if !strings.Contains(result, "paste_") {
			t.Errorf("Expected reference to contain 'paste_', got %q", result)
		}
		if !strings.Contains(result, ".txt") {
			t.Errorf("Expected reference to contain '.txt', got %q", result)
		}
	})

	t.Run("collapsed text is saved to file", func(t *testing.T) {
		pc := NewPasteCollapse()
		pc.dir = t.TempDir()

		text := "line1\nline2\nline3\nline4\nline5\nline6"
		result := pc.Collapse(text)

		// Extract file path from reference
		// Format: [Pasted text #N: M lines → path]
		start := strings.Index(result, "→ ") + len("→ ")
		end := strings.Index(result, "]")
		if start < 0 || end < 0 || end <= start {
			t.Fatalf("Could not parse file path from result: %q", result)
		}
		filePath := result[start:end]

		// Expand ~ if present
		if strings.HasPrefix(filePath, "~") {
			home, _ := os.UserHomeDir()
			filePath = filepath.Join(home, filePath[1:])
		}

		// Verify the file exists and contains the original text
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Paste file not found at %q: %v", filePath, err)
		}
		if string(data) != text {
			t.Errorf("Paste file content mismatch:\ngot:  %q\nwant: %q", string(data), text)
		}
	})

	t.Run("paste counter increments", func(t *testing.T) {
		pc := NewPasteCollapse()
		pc.dir = t.TempDir()

		text := "line1\nline2\nline3\nline4\nline5"
		result1 := pc.Collapse(text)
		result2 := pc.Collapse(text)

		if !strings.Contains(result1, "#1") {
			t.Errorf("First paste should be #1, got %q", result1)
		}
		if !strings.Contains(result2, "#2") {
			t.Errorf("Second paste should be #2, got %q", result2)
		}
	})

	t.Run("slash command text is returned as-is", func(t *testing.T) {
		pc := NewPasteCollapse()
		pc.dir = t.TempDir()

		text := "/edit " + strings.Repeat("x", 10000)
		result := pc.Collapse(text)
		if result != text {
			t.Error("Slash commands should not be collapsed")
		}
	})
}

func TestBracketedPaste(t *testing.T) {
	t.Run("enable and disable produce correct escape sequences", func(t *testing.T) {
		// Just verify the functions don't panic
		EnableBracketedPaste()
		DisableBracketedPaste()
	})
}