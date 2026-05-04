// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Non-existent file should return defaults
	config, err := LoadConfig("/nonexistent/path.yaml")
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig(), config)
}

func TestLoadConfig_MergePartial(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a minimal YAML config
	yamlContent := `
daemon:
  socket: /tmp/custom.sock
  tcp: localhost:60000

database:
  path: /tmp/custom.db
  wal_mode: false
  busy_timeout: 3000

hot_block:
  memory_char_limit: 5000
  user_char_limit: 3000
  min_priority: 1
  observation_ttl_days: 60
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	config, err := LoadConfig(configPath)
	require.NoError(t, err)

	// Check that custom values are applied
	assert.Equal(t, "/tmp/custom.sock", config.Daemon.Socket)
	assert.Equal(t, "localhost:60000", config.Daemon.TCP)
	assert.Equal(t, "/tmp/custom.db", config.Store.Path)
	assert.Equal(t, false, config.Store.WALMode)
	assert.Equal(t, 3000, config.Store.BusyTimeout)
	assert.Equal(t, int32(5000), config.HotBlock.MemoryCharLimit)
	assert.Equal(t, int32(3000), config.HotBlock.UserCharLimit)
	assert.Equal(t, int32(1), config.HotBlock.MinPriority)
	assert.Equal(t, 60, config.HotBlock.ObservationTTLDays)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create invalid YAML
	invalidYAML := `daemon:
  socket: /tmp/custom.sock
  invalid_field: "this will break parsing"
  tls:
    cert: /path/to/cert
    key: /path/to/key
hot_block:
  memory_char_limit: invalid_value
`
	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	_, err = LoadConfig(configPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse config YAML")
}