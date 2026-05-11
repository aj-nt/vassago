// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package agentlog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// testLogger returns a logger that writes to buf and the buffer, so tests can
// inspect structured output.
func testLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})
	return slog.New(h), buf
}

func TestInitSetsGlobalLogger(t *testing.T) {
	buf := new(bytes.Buffer)
	l := slog.New(slog.NewJSONHandler(buf, nil))
	Init(l)

	Info("test message", "key", "value")

	// Verify the custom logger was used
	if buf.Len() == 0 {
		t.Fatal("expected output from custom logger, got nothing")
	}
}

func TestInfoLogsStructuredJSON(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	Info("agent started", "model", "gpt-4o", "provider", "openai")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}

	if entry["msg"] != "agent started" {
		t.Errorf("msg = %q, want %q", entry["msg"], "agent started")
	}
	if entry["model"] != "gpt-4o" {
		t.Errorf("model = %q, want %q", entry["model"], "gpt-4o")
	}
	if entry["provider"] != "openai" {
		t.Errorf("provider = %q, want %q", entry["provider"], "openai")
	}
	if entry["level"] != "INFO" {
		t.Errorf("level = %q, want %q", entry["level"], "INFO")
	}
}

func TestErrorLogsAtErrorLevel(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo) // all levels pass through Info and above
	Init(l)

	Error("pipeline failed", "error", "timeout", "iteration", 5)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}

	if entry["level"] != "ERROR" {
		t.Errorf("level = %q, want %q", entry["level"], "ERROR")
	}
	if entry["error"] != "timeout" {
		t.Errorf("error = %q, want %q", entry["error"], "timeout")
	}
}

func TestWarnLogsAtWarnLevel(t *testing.T) {
	l, buf := testLogger(slog.LevelWarn)
	Init(l)

	Warn("retry attempt", "attempt", 3, "max", 5)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}

	if entry["level"] != "WARN" {
		t.Errorf("level = %q, want %q", entry["level"], "WARN")
	}
}

func TestDebugIsFilteredAtInfoLevel(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo) // Debug < Info
	Init(l)

	Debug("verbose detail", "data", "secret")

	if buf.Len() != 0 {
		t.Errorf("expected no output at Info level, got: %s", buf.String())
	}
}

func TestDebugAppearsAtDebugLevel(t *testing.T) {
	l, buf := testLogger(slog.LevelDebug)
	Init(l)

	Debug("verbose detail", "data", "secret")

	if buf.Len() == 0 {
		t.Fatal("expected output at Debug level, got nothing")
	}
}

func TestPrintfBridgeProducesJSON(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	Printf("agent started with model %s on provider %s", "gpt-4o", "openai")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s | err: %v", buf.String(), err)
	}

	if entry["level"] != "INFO" {
		t.Errorf("level = %q, want %q", entry["level"], "INFO")
	}

	msg, ok := entry["msg"].(string)
	if !ok {
		t.Fatalf("msg is not a string: %T", entry["msg"])
	}
	if !strings.Contains(msg, "gpt-4o") || !strings.Contains(msg, "openai") {
		t.Errorf("msg = %q, want it to contain gpt-4o and openai", msg)
	}
}

func TestPrintfEmptyFormat(t *testing.T) {
	l, buf := testLogger(slog.LevelInfo)
	Init(l)

	Printf("static message with no format verbs")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON, got: %s", buf.String())
	}
	if entry["msg"] != "static message with no format verbs" {
		t.Errorf("msg = %q", entry["msg"])
	}
}

func TestFatalfPanics(t *testing.T) {
	l, _ := testLogger(slog.LevelInfo)
	Init(l)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected Fatalf to panic, but it didn't")
		}
	}()

	Fatalf("unrecoverable: %s", "database connection refused")
}

func TestConcurrentSafety(t *testing.T) {
	l, _ := testLogger(slog.LevelInfo)
	Init(l)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				Info("concurrent log", "goroutine", id, "iteration", j)
				Printf("concurrent printf %d/%d", id, j)
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// If we get here without race detector complaints, it passes.
}
