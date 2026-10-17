// This file is part of Vassago.
// See LICENSE for license information.

package daemon

// LoggingConfig configures where and how the daemon writes logs.
type LoggingConfig struct {
	// Path is the log file path. Default: ~/.vassago/daemon.log.
	// Logs are written to both this file AND stderr.
	Path  string `yaml:"path"`
	// Level sets the minimum log level: debug, info, warn, error.
	// Default: info.
	Level string `yaml:"level"`
}
