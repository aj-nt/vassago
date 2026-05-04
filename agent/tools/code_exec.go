// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodeExecConfig configures the ExecuteCode tool.
type CodeExecConfig struct {
	Enabled      bool `yaml:"enabled"`
	Timeout      int  `yaml:"timeout"`        // max seconds for execution (default 300 = 5 min)
	MaxOutputKB  int  `yaml:"max_output_kb"`  // max output in KB (default 50)
	MaxCodeChars int  `yaml:"max_code_chars"` // max code length in chars (default 50000)
}

// DefaultCodeExecConfig returns a config with sensible defaults.
func DefaultCodeExecConfig() CodeExecConfig {
	return CodeExecConfig{
		Enabled:      true,
		Timeout:      300,
		MaxOutputKB:  50,
		MaxCodeChars: 50000,
	}
}

var codeExecConfig = DefaultCodeExecConfig()

// SetCodeExecConfig configures the global code execution settings.
func SetCodeExecConfig(cfg CodeExecConfig) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 300
	}
	if cfg.MaxOutputKB <= 0 {
		cfg.MaxOutputKB = 50
	}
	if cfg.MaxCodeChars <= 0 {
		cfg.MaxCodeChars = 50000
	}
	codeExecConfig = cfg
}

// RegisterCodeExecTools adds the ExecuteCode tool to the registry.
func RegisterCodeExecTools(registry *ToolRegistry, cfg CodeExecConfig) {
	if !cfg.Enabled {
		return
	}
	SetCodeExecConfig(cfg)
	registry.Register("ExecuteCode", ExecuteCodeHandler, ExecuteCodeSchema)
}

// ExecuteCodeSchema defines the tool schema for the LLM.
var ExecuteCodeSchema = map[string]interface{}{
	"name":        "ExecuteCode",
	"description": "Run a Python script that can call Mnemosyne tools programmatically. Use this when you need 3+ tool calls with processing logic between them, need to filter/reduce large tool outputs before they enter your context, need conditional branching (if X then Y else Z), or need to loop (fetch N pages, process N files, retry on failure). Use normal tool calls instead when: single tool call with no processing, you need to see the full result and apply complex reasoning, or the task requires interactive user input.",
	"parameters": map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"code": map[string]interface{}{
				"type":        "string",
				"description": "Python code to execute",
			},
			"workdir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for script execution (must be under allowed dirs)",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Max seconds to wait (default from config, cannot exceed config max)",
			},
		},
		"required": []string{"code"},
	},
}

// ExecuteCodeHandler runs Python code and returns its output.
func ExecuteCodeHandler(args map[string]interface{}) (string, bool) {
	code, _ := args["code"].(string)
	if code == "" {
		return "ExecuteCode requires a 'code' argument", true
	}

	// Validate code length
	if len(code) > codeExecConfig.MaxCodeChars {
		return fmt.Sprintf("Code exceeds maximum length (%d chars, limit %d)", len(code), codeExecConfig.MaxCodeChars), true
	}

	workdir, _ := args["workdir"].(string)

	// Validate workdir if provided
	if workdir != "" {
		// Expand ~ to home directory
		if strings.HasPrefix(workdir, "~") {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Sprintf("Error expanding home directory: %v", err), true
			}
			workdir = filepath.Join(homeDir, workdir[1:])
		}
		workdir = filepath.Clean(workdir)

		// Only enforce allowed dirs when configured (same as FileRead/FileWrite).
		// When allowedDirs is empty (dev mode), any workdir is allowed.
		if len(allowedDirs) > 0 && !IsUnderAllowedDir(workdir) {
			return fmt.Sprintf("Access denied: working directory %q is not in an allowed directory", workdir), true
		}
	}

	// Get timeout (user can request less than config max, not more)
	timeout := codeExecConfig.Timeout
	if t, ok := args["timeout"].(float64); ok && int(t) > 0 {
		requested := int(t)
		if requested > codeExecConfig.Timeout {
			return fmt.Sprintf("Timeout %ds exceeds maximum %ds", requested, codeExecConfig.Timeout), true
		}
		timeout = requested
	}

	// Write code to temp file
	tmpFile, err := os.CreateTemp("", "mnemosyne_code_*.py")
	if err != nil {
		return fmt.Sprintf("Failed to create temp file: %v", err), true
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(code); err != nil {
		tmpFile.Close()
		return fmt.Sprintf("Failed to write code to temp file: %v", err), true
	}
	tmpFile.Close()

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", tmpPath)
	if workdir != "" {
		cmd.Dir = workdir
	}

	output, err := cmd.CombinedOutput()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		outputStr := string(output)
		if outputStr == "" {
			outputStr = "(no output before timeout)"
		}
		return fmt.Sprintf("Code execution timed out after %ds\nOutput: %s", timeout, outputStr), true
	}

	result := string(output)
	maxBytes := codeExecConfig.MaxOutputKB * 1024
	if len(result) > maxBytes {
		result = truncateOutput(result, maxBytes)
	}

	if err != nil {
		// Non-zero exit code or other execution error
		return result, true
	}

	return result, false
}