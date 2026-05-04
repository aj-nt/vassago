// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCodeExecTest() {
	SetCodeExecConfig(DefaultCodeExecConfig())
}

func TestExecuteCode_SimplePrint(t *testing.T) {
	setupCodeExecTest()

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "print('hello from code exec')",
	})

	assert.False(t, isError, "should not be error: %s", result)
	assert.Contains(t, result, "hello from code exec")
}

func TestExecuteCode_MissingCode(t *testing.T) {
	setupCodeExecTest()

	result, isError := ExecuteCodeHandler(map[string]interface{}{})

	assert.True(t, isError)
	assert.Contains(t, result, "requires a 'code' argument")
}

func TestExecuteCode_CodeTooLong(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.MaxCodeChars = 10
	SetCodeExecConfig(cfg)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "print('this is a very long code that exceeds the limit')",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "exceeds maximum length")
}

func TestExecuteCode_InvalidWorkdir(t *testing.T) {
	setupCodeExecTest()

	// Must set allowedDirs for access checks to be enforced
	SetFileConfig([]string{"/tmp"}, nil)
	defer SetFileConfig(nil, nil)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code":    "print('test')",
		"workdir": "/etc",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Access denied")
}

func TestExecuteCode_WorkdirExpansion(t *testing.T) {
	setupCodeExecTest()

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	testDir := filepath.Join(homeDir, "test_code_exec_workdir")
	require.NoError(t, os.MkdirAll(testDir, 0755))
	defer os.RemoveAll(testDir)

	// Set allowed dirs to include home directory
	SetFileConfig([]string{homeDir}, nil)
	defer SetFileConfig(nil, nil)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code":    "import os; print(os.getcwd())",
		"workdir": "~" + strings.TrimPrefix(testDir, homeDir),
	})

	assert.False(t, isError, "should not be error: %s", result)
	assert.Contains(t, result, testDir)
}

func TestExecuteCode_Timeout(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.Timeout = 2
	SetCodeExecConfig(cfg)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "import time; time.sleep(60)",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "timed out")
}

func TestExecuteCode_TimeoutExceedsMax(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.Timeout = 5
	SetCodeExecConfig(cfg)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code":    "print('test')",
		"timeout": float64(100),
	})

	assert.True(t, isError)
	assert.Contains(t, result, "exceeds maximum")
}

func TestExecuteCode_SyntaxError(t *testing.T) {
	setupCodeExecTest()

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "invalid python syntax",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "SyntaxError")
}

func TestExecuteCode_RuntimeError(t *testing.T) {
	setupCodeExecTest()

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "1/0",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "ZeroDivisionError")
}

func TestExecuteCode_OutputTruncation(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.MaxOutputKB = 1
	SetCodeExecConfig(cfg)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "print('x' * 2048)",
	})

	assert.False(t, isError, "output truncation is not an error")
	assert.Contains(t, result, "truncated")
}

func TestExecuteCode_ImportStdlib(t *testing.T) {
	setupCodeExecTest()

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "import json; print(json.dumps({'ok': True}))",
	})

	assert.False(t, isError, "should not be error: %s", result)
	assert.Contains(t, result, `"ok"`)
}

func TestExecuteCode_WorkdirNotAllowed(t *testing.T) {
	setupCodeExecTest()

	// Must set allowedDirs for access checks to be enforced
	SetFileConfig([]string{"/tmp"}, nil)
	defer SetFileConfig(nil, nil)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code":    "print('test')",
		"workdir": "/root",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Access denied")
}

func TestCodeExecConfig_Defaults(t *testing.T) {
	cfg := DefaultCodeExecConfig()
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 300, cfg.Timeout)
	assert.Equal(t, 50, cfg.MaxOutputKB)
	assert.Equal(t, 50000, cfg.MaxCodeChars)
}

func TestCodeExecConfig_SetCodeExecConfig(t *testing.T) {
	cfg := CodeExecConfig{
		Enabled:      true,
		Timeout:      0,
		MaxOutputKB:  0,
		MaxCodeChars: 0,
	}
	SetCodeExecConfig(cfg)

	assert.Equal(t, 300, codeExecConfig.Timeout)
	assert.Equal(t, 50, codeExecConfig.MaxOutputKB)
	assert.Equal(t, 50000, codeExecConfig.MaxCodeChars)

	// Reset
	SetCodeExecConfig(DefaultCodeExecConfig())
}

func TestCodeExecConfig_NegativeValues(t *testing.T) {
	cfg := CodeExecConfig{
		Enabled:      true,
		Timeout:      -5,
		MaxOutputKB:  -1,
		MaxCodeChars: -100,
	}
	SetCodeExecConfig(cfg)

	assert.Equal(t, 300, codeExecConfig.Timeout)
	assert.Equal(t, 50, codeExecConfig.MaxOutputKB)
	assert.Equal(t, 50000, codeExecConfig.MaxCodeChars)

	// Reset
	SetCodeExecConfig(DefaultCodeExecConfig())
}

func TestRegisterCodeExecTools_Disabled(t *testing.T) {
	cfg := DefaultCodeExecConfig()
	cfg.Enabled = false
	registry := NewToolRegistry()
	RegisterCodeExecTools(registry, cfg)

	assert.Equal(t, 0, len(registry.List()))
}

func TestRegisterCodeExecTools_Enabled(t *testing.T) {
	cfg := DefaultCodeExecConfig()
	cfg.Enabled = true
	registry := NewToolRegistry()
	RegisterCodeExecTools(registry, cfg)

	names := registry.List()
	assert.Equal(t, 1, len(names))
	assert.Contains(t, names, "ExecuteCode")
}

func TestExecuteCode_CustomTimeout(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.Timeout = 30
	SetCodeExecConfig(cfg)

	// Request a shorter timeout than max — should work
	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code":    "print('quick')",
		"timeout": float64(5),
	})

	assert.False(t, isError, "should not be error: %s", result)
	assert.Contains(t, result, "quick")
}

// --- Red Team Tests ---

func TestRedTeam_CodeExec_ShellInjection(t *testing.T) {
	setupCodeExecTest()

	// os.system is allowed — we rely on timeout/output limits, not Python sandboxing
	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "import os; os.system('echo test')",
	})

	// This should execute (not an error from our perspective)
	// The echo output goes to the subprocess stdout, not captured by os.system
	assert.False(t, isError, "shell echo should succeed: %s", result)
}

func TestRedTeam_CodeExec_FileExfiltration(t *testing.T) {
	setupCodeExecTest()

	// Python can read system files since we don't sandbox Python itself.
	// We rely on workdir validation and the host system's permissions.
	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "import os; print(os.path.exists('/etc/passwd'))",
	})

	assert.False(t, isError, "should execute: %s", result)
	assert.Contains(t, result, "True") // File exists on Unix systems
}

func TestRedTeam_CodeExec_InfiniteLoop(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.Timeout = 2
	SetCodeExecConfig(cfg)

	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "while True: pass",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "timed out")
}

func TestRedTeam_CodeExec_ImportOs(t *testing.T) {
	setupCodeExecTest()

	// os.system is allowed — no Python-level sandboxing
	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "import os; os.system('echo pwned')",
	})

	assert.False(t, isError, "os.system echo should execute: %s", result)
}

func TestRedTeam_CodeExec_ProcessSpawning(t *testing.T) {
	setupCodeExecTest()

	cfg := DefaultCodeExecConfig()
	cfg.Timeout = 3
	SetCodeExecConfig(cfg)

	// Simulated process spawning — limited by timeout
	// Don't do actual fork bomb, just subprocess creation
	result, isError := ExecuteCodeHandler(map[string]interface{}{
		"code": "import subprocess; result = subprocess.run(['echo', 'spawned'], capture_output=True, text=True); print(result.stdout)",
	})

	assert.False(t, isError, "subprocess should work: %s", result)
	assert.Contains(t, result, "spawned")
}