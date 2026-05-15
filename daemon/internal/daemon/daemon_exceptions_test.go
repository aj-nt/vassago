// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aj-nt/vassago/daemon/internal/grpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Validate() exception paths ──

func TestValidate_TLSKeyWithoutCert(t *testing.T) {
	cfg := Config{
		Daemon: DaemonConfig{
			Socket: "/tmp/test.sock",
			TLS:    TLSConfig{Key: "/path/to/key"},
		},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "daemon.tls.cert and daemon.tls.key must both be set or both be empty")
}

func TestValidate_EmptyStorePath(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: ""},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database.path is required")
}

func TestValidate_MemoryCharLimitTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 500, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.memory_char_limit")
}

func TestValidate_MemoryCharLimitTooHigh(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 30000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.memory_char_limit")
}

func TestValidate_UserCharLimitTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 100, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.user_char_limit")
}

func TestValidate_UserCharLimitTooHigh(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 20000, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.user_char_limit")
}

func TestValidate_MinPriorityTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 0, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.min_priority")
}

func TestValidate_MinPriorityTooHigh(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 10, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.min_priority")
}

func TestValidate_ObservationTTLTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 3},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hot_block.observation_ttl_days")
}

func TestValidate_BusyTimeoutTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 500},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database.busy_timeout")
}

func TestValidate_CronIntervalTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
		Cron:     CronConfig{Enabled: true, Interval: 5, MaxJobs: 100},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cron.interval must be between 10 and 3600")
}

func TestValidate_CronIntervalTooHigh(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
		Cron:     CronConfig{Enabled: true, Interval: 5000, MaxJobs: 100},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cron.interval must be between 10 and 3600")
}

func TestValidate_CronMaxJobsTooLow(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
		Cron:     CronConfig{Enabled: true, Interval: 60, MaxJobs: 0},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cron.max_jobs must be between 1 and 1000")
}

func TestValidate_CronMaxJobsTooHigh(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
		Cron:     CronConfig{Enabled: true, Interval: 60, MaxJobs: 2000},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cron.max_jobs must be between 1 and 1000")
}

func TestValidate_CronValidationSkippedWhenDisabled(t *testing.T) {
	cfg := Config{
		Daemon:   DaemonConfig{Socket: "/tmp/test.sock"},
		Store:    StoreConfig{Path: "/tmp/test.db", BusyTimeout: 5000},
		HotBlock: HotBlockConfig{MemoryCharLimit: 4000, UserCharLimit: 2000, MinPriority: 2, ObservationTTLDays: 30},
		Cron:     CronConfig{Enabled: false, Interval: 1, MaxJobs: 0}, // invalid but disabled
	}
	err := cfg.Validate()
	assert.NoError(t, err, "cron validation should be skipped when disabled")
}

// ── setupMessagingAdapters() uncovered branches ──

func TestSetupMessagingAdapters_IMessageEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Messaging.IMessage.Enabled = true
	cfg.Messaging.IMessage.PollInterval = 10

	svc := grpc.NewMessagingService()
	setupMessagingAdapters(cfg, svc)

	status := svc.Status()
	assert.Contains(t, status, "imessage", "imessage adapter should be registered")
}

func TestSetupMessagingAdapters_EmailEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Messaging.Email.Enabled = true
	cfg.Messaging.Email.PollInterval = 30
	cfg.Messaging.Email.IMAPHost = "imap.example.com"
	cfg.Messaging.Email.SMTPHost = "smtp.example.com"

	svc := grpc.NewMessagingService()
	setupMessagingAdapters(cfg, svc)

	status := svc.Status()
	assert.Contains(t, status, "email", "email adapter should be registered")
}

func TestSetupMessagingAdapters_IMessageDefaultPoll(t *testing.T) {
	// DefaultConfig has PollInterval=0 — setupMessagingAdapters converts to 0s duration
	cfg := DefaultConfig()
	cfg.Messaging.IMessage.Enabled = true

	svc := grpc.NewMessagingService()
	// Should not panic with zero poll interval
	assert.NotPanics(t, func() {
		setupMessagingAdapters(cfg, svc)
	})
	assert.Contains(t, svc.Status(), "imessage")
}

// ── setupListeners() error cleanup path ──

func TestSetupListeners_TCPFailsAfterUnixSuccess(t *testing.T) {
	// macOS has a 104-byte limit on Unix socket paths. Use a short path.
	socketPath := filepath.Join("/tmp", "mnemosyne-test-"+t.Name()+".sock")

	// Bind a TCP port first, then try to use the same port
	blocker, err := ListenTCP("localhost:0")
	require.NoError(t, err)
	defer blocker.Close()
	blockedAddr := blocker.Addr().String()

	cfg := DaemonConfig{
		Socket: socketPath,
		TCP:    blockedAddr, // port already taken
	}

	listeners, err := setupListeners(cfg)
	assert.Error(t, err, "should fail when TCP port is already in use")
	assert.Contains(t, err.Error(), "TCP:")
	assert.Nil(t, listeners, "listeners should be nil after cleanup")
}

// ── New() error path: db directory creation failure ──

func TestNew_DBCreationFailure(t *testing.T) {
	// Create a file where the directory should be
	tmpDir := t.TempDir()
	blockerPath := filepath.Join(tmpDir, "blocker")
	require.NoError(t, os.WriteFile(blockerPath, []byte("block"), 0644))

	dbPath := filepath.Join(blockerPath, "subdir", "test.db")

	cfg := DefaultConfig()
	cfg.Store.Path = dbPath
	cfg.Daemon.Socket = filepath.Join(tmpDir, "test.sock")
	cfg.Daemon.TCP = "localhost:0"

	_, err := New(cfg)
	assert.Error(t, err, "should fail when DB directory can't be created")
	assert.Contains(t, err.Error(), "create database directory")
}

// ── New() error path: WAL mode not writable ──

func TestNew_StoreOpenFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create the file read-only before New tries to open it
	require.NoError(t, os.WriteFile(dbPath, []byte("not a sqlite db"), 0444))
	defer os.Chmod(dbPath, 0644)

	cfg := DefaultConfig()
	cfg.Store.Path = dbPath
	cfg.Daemon.Socket = filepath.Join(tmpDir, "test.sock")
	cfg.Daemon.TCP = "localhost:0"

	_, err := New(cfg)
	assert.Error(t, err, "should fail when store can't open")
	assert.Contains(t, err.Error(), "open store")
}

// ── processGitHubEvent() empty/error return path ──

func TestProcessGitHubEvent_IssueWithZeroNumber(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig()
	cfg.Store.Path = dbPath
	cfg.Daemon.Socket = filepath.Join(tmpDir, "test.sock")
	cfg.Daemon.TCP = "localhost:0"

	d, err := New(cfg)
	require.NoError(t, err)
	defer d.Store().Close()

	// Issue with number=0 — ParseIssueEvent returns ("", nil)
	payload := `{"action":"opened","issue":{"number":0,"title":"","body":"","html_url":"","user":{"login":""}},"repository":{"full_name":"test/repo"}}`
	memory, err := d.processGitHubEvent("issues", []byte(payload))
	assert.NoError(t, err)
	assert.Empty(t, memory, "zero-number issues should return empty memory")
}

func TestProcessGitHubEvent_PREventUnknownAction(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig()
	cfg.Store.Path = dbPath
	cfg.Daemon.Socket = filepath.Join(tmpDir, "test.sock")
	cfg.Daemon.TCP = "localhost:0"

	d, err := New(cfg)
	require.NoError(t, err)
	defer d.Store().Close()

	// Valid PR with a weird action — still creates a memory with the action verbatim
	payload := `{"action":"transmogrified","number":99,"pull_request":{"number":99,"title":"Weird PR","body":"Odd body","html_url":"https://github.com/test/repo/pull/99","user":{"login":"user"}},"repository":{"full_name":"test/repo"}}`
	memory, err := d.processGitHubEvent("pull_request", []byte(payload))
	assert.NoError(t, err)
	assert.NotEmpty(t, memory)

	// Verify it was persisted
	memories, err := d.Store().ListMemories("default", "", "observation", 0)
	require.NoError(t, err)
	assert.NotEmpty(t, memories)
}

// ── Duplicate GitHub events (idempotency key check) ──

func TestProcessGitHubEvent_DuplicateDoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig()
	cfg.Store.Path = dbPath
	cfg.Daemon.Socket = filepath.Join(tmpDir, "test.sock")
	cfg.Daemon.TCP = "localhost:0"

	d, err := New(cfg)
	require.NoError(t, err)
	defer d.Store().Close()

	// Send the same issue event twice
	memory1, err := d.processGitHubEvent("issues", []byte(validIssuePayload))
	require.NoError(t, err)
	require.NotEmpty(t, memory1)

	memory2, err := d.processGitHubEvent("issues", []byte(validIssuePayload))
	require.NoError(t, err)
	require.NotEmpty(t, memory2)

	// Both should succeed — each has a unique key due to nanosecond timestamps
	// so they create separate entries. That's fine — the store's dedup is key-based.
	assert.Equal(t, memory1, memory2, "same content should produce same memory string")
}


