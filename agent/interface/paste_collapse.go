// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package iface

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PasteCollapse manages large paste detection and file storage.
// When the user pastes text with 5+ lines, it saves the text to
// ~/.mnemosyne/pastes/ and replaces the input with a compact reference.
//
// This is inspired by Hermes' paste-collapse feature: instead of showing
// hundreds of lines of pasted code in the input field, the user sees:
//
//	[Pasted text #3: 47 lines → ~/.mnemosyne/pastes/paste_3_143052.txt]
type PasteCollapse struct {
	mu      sync.Mutex
	counter int
	dir     string
	enabled bool

	// PasteThresholdLines is the minimum number of lines to trigger collapse.
	PasteThresholdLines int
	// PasteThresholdChars is the minimum number of characters to trigger collapse.
	PasteThresholdChars int
}

// NewPasteCollapse creates a new PasteCollapse manager.
// It creates the paste directory if it doesn't exist.
func NewPasteCollapse() *PasteCollapse {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".mnemosyne", "pastes")
	return &PasteCollapse{
		dir:                 dir,
		PasteThresholdLines: 5,
		PasteThresholdChars: 8000,
		enabled:             true,
	}
}

// ShouldCollapse returns true if the given text meets the threshold for collapsing.
// Slash commands (starting with /) are never collapsed.
func (pc *PasteCollapse) ShouldCollapse(text string) bool {
	if !pc.enabled {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(text), "/") {
		return false
	}
	lines := strings.Count(text, "\n") + 1
	return lines >= pc.PasteThresholdLines || len(text) >= pc.PasteThresholdChars
}

// Collapse saves the pasted text to a file and returns a compact reference string.
// The reference includes the paste number, line count, and file path.
// Returns the original text if saving fails (graceful degradation).
func (pc *PasteCollapse) Collapse(text string) string {
	if !pc.ShouldCollapse(text) {
		return text
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.counter++
	lineCount := strings.Count(text, "\n") + 1

	// Ensure paste directory exists
	if err := os.MkdirAll(pc.dir, 0700); err != nil {
		// Graceful degradation: return original text if we can't save
		return text
	}

	// Save to file: paste_{counter}_{timestamp}.txt
	timestamp := time.Now().Format("150405")
	filename := fmt.Sprintf("paste_%d_%s.txt", pc.counter, timestamp)
 filepath := filepath.Join(pc.dir, filename)

	if err := os.WriteFile(filepath, []byte(text), 0600); err != nil {
		// Graceful degradation: return original text if we can't save
		return text
	}

	// Return compact reference
	home, _ := os.UserHomeDir()
	displayPath := filepath
	if home != "" && strings.HasPrefix(filepath, home) {
		displayPath = "~" + strings.TrimPrefix(filepath, home)
	}

	return fmt.Sprintf("[Pasted text #%d: %d lines → %s]", pc.counter, lineCount, displayPath)
}

// EnableBracketedPaste enables terminal bracketed paste mode.
// When enabled, the terminal wraps pastes in \x1b[2004~ ... \x1b[2005~ sequences,
// allowing the application to detect pastes vs typed input.
func EnableBracketedPaste() {
	fmt.Print("\x1b[?2004h")
}

// DisableBracketedPaste disables terminal bracketed paste mode.
func DisableBracketedPaste() {
	fmt.Print("\x1b[?2004l")
}

// ReadInput reads a line of input from stdin, handling bracketed paste sequences.
// When a paste is detected (bracketed paste markers), the entire paste is
// collected and then collapsed if it exceeds the threshold.
// Returns the (possibly collapsed) input and whether this was a paste.
// Returns empty string and false on EOF.
func ReadInput(scanner *bufio.Scanner, pc *PasteCollapse) (string, bool) {
	if !scanner.Scan() {
		return "", false
	}
	text := scanner.Text()

	// Check for bracketed paste start: \x1b[200~ ... \x1b[201~
	if strings.HasPrefix(text, "\x1b[200~") {
		// This is a paste — collect all lines until the end marker \x1b[201~
		paste := strings.TrimPrefix(text, "\x1b[200~")
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "\x1b[201~") {
				// Strip the end marker
				paste += "\n" + strings.TrimSuffix(line, "\x1b[201~")
				break
			}
			paste += "\n" + line
		}

		// Collapse if needed
		if pc.ShouldCollapse(paste) {
			return pc.Collapse(paste), true
		}
		return strings.TrimRight(paste, "\n"), true
	}

	// Regular single-line input
	return text, false
}

// ReadMultilineInput reads multiple lines until a blank line or EOF.
// Used for collecting multi-line user input (like the /edit command).
// Returns the collected text and whether it was collapsed.
func ReadMultilineInput(scanner *bufio.Scanner, pc *PasteCollapse) (string, bool) {
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", false
	}
	text := strings.Join(lines, "\n")

	if pc.ShouldCollapse(text) {
		return pc.Collapse(text), true
	}
	return text, false
}