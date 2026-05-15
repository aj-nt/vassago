// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDaemonConfigDeep(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid daemon config", func(t *testing.T) {
		path := filepath.Join(tmpDir, "valid.yaml")
		os.WriteFile(path, []byte(`daemon:
  socket: /tmp/test.sock
  tcp: localhost:50051
database:
  path: /tmp/test.db
  busy_timeout: 5000
hot_block:
  memory_char_limit: 4000
  user_char_limit: 2000
  min_priority: 2
  observation_ttl_days: 30
`), 0644)
		issues := validateDaemonConfigDeep(path)
		errors := filterIssues(issues, "error")
		if len(errors) > 0 {
			t.Errorf("Valid config should have no errors, got: %v", errors)
		}
	})

	t.Run("missing socket and tcp", func(t *testing.T) {
		path := filepath.Join(tmpDir, "no-addr.yaml")
		os.WriteFile(path, []byte(`daemon: {}
database:
  path: /tmp/test.db
`), 0644)
		issues := validateDaemonConfigDeep(path)
		if !hasIssue(issues, "error", "daemon.socket or daemon.tcp") {
			t.Error("Should report missing socket/tcp")
		}
	})

	t.Run("invalid hot block limits", func(t *testing.T) {
		path := filepath.Join(tmpDir, "bad-hotblock.yaml")
		os.WriteFile(path, []byte(`daemon:
  tcp: localhost:50051
database:
  path: /tmp/test.db
hot_block:
  memory_char_limit: 50
  user_char_limit: 10
  min_priority: 99
  observation_ttl_days: 2
`), 0644)
		issues := validateDaemonConfigDeep(path)
		if !hasIssue(issues, "error", "memory_char_limit") {
			t.Error("Should report memory_char_limit out of range")
		}
		if !hasIssue(issues, "error", "user_char_limit") {
			t.Error("Should report user_char_limit out of range")
		}
		if !hasIssue(issues, "error", "min_priority") {
			t.Error("Should report min_priority out of range")
		}
		if !hasIssue(issues, "error", "observation_ttl_days") {
			t.Error("Should report observation_ttl_days out of range")
		}
	})

	t.Run("tls cert without key", func(t *testing.T) {
		path := filepath.Join(tmpDir, "tls-half.yaml")
		os.WriteFile(path, []byte(`daemon:
  tcp: localhost:50051
  tls:
    cert: /path/to/cert
database:
  path: /tmp/test.db
`), 0644)
		issues := validateDaemonConfigDeep(path)
		if !hasIssue(issues, "error", "tls.cert and daemon.tls.key") {
			t.Error("Should report TLS cert without key")
		}
	})

	t.Run("bad cron interval", func(t *testing.T) {
		path := filepath.Join(tmpDir, "bad-cron.yaml")
		os.WriteFile(path, []byte(`daemon:
  tcp: localhost:50051
database:
  path: /tmp/test.db
cron:
  enabled: true
  interval: 5
  max_jobs: 0
`), 0644)
		issues := validateDaemonConfigDeep(path)
		if !hasIssue(issues, "error", "cron.interval") {
			t.Error("Should report cron.interval out of range")
		}
	})

	t.Run("non-standard log level", func(t *testing.T) {
		path := filepath.Join(tmpDir, "bad-log.yaml")
		os.WriteFile(path, []byte(`daemon:
  tcp: localhost:50051
database:
  path: /tmp/test.db
logging:
  level: verbose
`), 0644)
		issues := validateDaemonConfigDeep(path)
		if !hasIssue(issues, "warning", "logging.level") {
			t.Error("Should warn about non-standard log level")
		}
	})
}

// Red Team: malicious config values
func TestDaemonConfigInjection(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("path traversal in db path", func(t *testing.T) {
		path := filepath.Join(tmpDir, "traversal.yaml")
		os.WriteFile(path, []byte(`daemon:
  tcp: localhost:50051
database:
  path: ../../etc/passwd
`), 0644)
		issues := validateDaemonConfigDeep(path)
		// The validator should at least not crash on path traversal
		_ = issues
	})

	t.Run("empty config", func(t *testing.T) {
		path := filepath.Join(tmpDir, "empty.yaml")
		os.WriteFile(path, []byte(`{}`), 0644)
		issues := validateDaemonConfigDeep(path)
		errors := filterIssues(issues, "error")
		if len(errors) == 0 {
			t.Error("Empty config should have validation errors")
		}
	})
}

// Helpers

func filterIssues(issues []ValidationIssue, severity string) []ValidationIssue {
	var filtered []ValidationIssue
	for _, i := range issues {
		if i.Severity == severity {
			filtered = append(filtered, i)
		}
	}
	return filtered
}

func hasIssue(issues []ValidationIssue, severity, substring string) bool {
	for _, i := range issues {
		if i.Severity == severity && contains(i.Message, substring) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
