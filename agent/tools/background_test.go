// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBgTest() {
	bgManager.processes = sync.Map{}
	SetBackgroundProcessConfig(BackgroundProcessConfig{
		Enabled:      true,
		MaxProcesses: 5,
		MaxOutputKB:  64,
		MaxAgeMin:   30,
	})
}

func TestBackgroundProcess_StartValid(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "echo hello",
	})

	assert.False(t, isError, "start should not return error: %s", result)
	assert.Contains(t, result, "Process started")
	assert.Contains(t, result, "Session ID:")

	CleanupBackgroundProcesses()
}

func TestBackgroundProcess_StartDangerousCommand(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "rm -rf /",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "blocked")
}

func TestBackgroundProcess_StartShellInjection(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "ls; rm -rf /",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "blocked")
}

func TestBackgroundProcess_StartMissingCommand(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action": "start",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Missing required parameter: command")
}

func TestBackgroundProcess_PollAfterStart(t *testing.T) {
	setupBgTest()

	startResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "echo test_output",
	})
	require.False(t, isError, "start should succeed: %s", startResult)

	sessionID := extractSessionID(startResult)
	require.NotEmpty(t, sessionID, "should extract session ID from: %s", startResult)

	// Wait for the command to produce output
	time.Sleep(300 * time.Millisecond)

	pollResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "poll",
		"session_id": sessionID,
	})

	assert.False(t, isError, "poll should succeed: %s", pollResult)
	assert.Contains(t, pollResult, sessionID)
	assert.Contains(t, pollResult, "test_output")

	CleanupBackgroundProcesses()
}

func TestBackgroundProcess_PollUnknownSession(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "poll",
		"session_id": "nonexistent",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Unknown session ID")
}

func TestBackgroundProcess_PollMissingSessionID(t *testing.T) {
	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action": "poll",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Missing required parameter")
}

func TestBackgroundProcess_KillRunningProcess(t *testing.T) {
	setupBgTest()

	startResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "sleep 60",
	})
	require.False(t, isError, "start should succeed: %s", startResult)

	sessionID := extractSessionID(startResult)
	require.NotEmpty(t, sessionID)

	killResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "kill",
		"session_id": sessionID,
	})

	assert.False(t, isError, "kill should succeed: %s", killResult)
	assert.Contains(t, killResult, "killed")
}

func TestBackgroundProcess_KillUnknownSession(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "kill",
		"session_id": "nonexistent",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Unknown session ID")
}

func TestBackgroundProcess_WaitExitedProcess(t *testing.T) {
	setupBgTest()

	startResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "echo done",
	})
	require.False(t, isError, "start should succeed: %s", startResult)

	sessionID := extractSessionID(startResult)
	require.NotEmpty(t, sessionID)

	// Wait for it with generous timeout
	waitResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "wait",
		"session_id": sessionID,
		"timeout":    float64(10),
	})

	assert.False(t, isError, "wait should succeed: %s", waitResult)
	assert.Contains(t, waitResult, "exited")

	CleanupBackgroundProcesses()
}

func TestBackgroundProcess_WaitTimeout(t *testing.T) {
	setupBgTest()

	startResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "sleep 300",
	})
	require.False(t, isError, "start should succeed: %s", startResult)

	sessionID := extractSessionID(startResult)
	require.NotEmpty(t, sessionID)

	// Wait with short timeout
	waitResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "wait",
		"session_id": sessionID,
		"timeout":    float64(2),
	})

	assert.True(t, isError, "timeout should return error")
	assert.Contains(t, waitResult, "Timeout reached")

	CleanupBackgroundProcesses()
}

func TestBackgroundProcess_WaitUnknownSession(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":     "wait",
		"session_id": "nonexistent",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Unknown session ID")
}

func TestBackgroundProcess_ListEmpty(t *testing.T) {
	setupBgTest()

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action": "list",
	})

	assert.False(t, isError)
	assert.Contains(t, result, "No background processes")
}

func TestBackgroundProcess_ListWithProcess(t *testing.T) {
	setupBgTest()

	startResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "sleep 10",
	})
	require.False(t, isError)

	sessionID := extractSessionID(startResult)
	require.NotEmpty(t, sessionID)

	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action": "list",
	})

	assert.False(t, isError)
	assert.Contains(t, result, sessionID)

	CleanupBackgroundProcesses()
}

func TestBackgroundProcess_MaxProcessLimit(t *testing.T) {
	bgManager.processes = sync.Map{}
	SetBackgroundProcessConfig(BackgroundProcessConfig{
		Enabled:      true,
		MaxProcesses: 2,
		MaxOutputKB:  64,
		MaxAgeMin:   30,
	})

	_, e1 := BackgroundProcessHandler(map[string]interface{}{"action": "start", "command": "sleep 60"})
	require.False(t, e1)
	_, e2 := BackgroundProcessHandler(map[string]interface{}{"action": "start", "command": "sleep 60"})
	require.False(t, e2)

	// Third should fail
	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "echo hello",
	})
	assert.True(t, isError)
	assert.Contains(t, result, "Maximum processes limit")

	CleanupBackgroundProcesses()
	// Reset to default config
	SetBackgroundProcessConfig(BackgroundProcessConfig{
		Enabled:      true,
		MaxProcesses: 5,
		MaxOutputKB:  64,
		MaxAgeMin:   30,
	})
}

func TestBackgroundProcess_InvalidAction(t *testing.T) {
	result, isError := BackgroundProcessHandler(map[string]interface{}{
		"action": "invalid",
	})

	assert.True(t, isError)
	assert.Contains(t, result, "Invalid action")
}

func TestBackgroundProcess_CleanupOldProcesses(t *testing.T) {
	setupBgTest()

	startResult, isError := BackgroundProcessHandler(map[string]interface{}{
		"action":  "start",
		"command": "echo done",
	})
	require.False(t, isError)

	sessionID := extractSessionID(startResult)
	require.NotEmpty(t, sessionID)

	// Wait for it to finish
	time.Sleep(300 * time.Millisecond)

	// Manually set its EndTime to old
	val, ok := bgManager.processes.Load(sessionID)
	require.True(t, ok)
	proc := val.(*BackgroundProc)
	proc.mu.Lock()
	proc.EndTime = time.Now().Add(-60 * time.Minute) // 60 min ago
	proc.mu.Unlock()

	// Cleanup with 30 min cutoff should remove it
	bgManager.cleanupOld(30)

	_, exists := bgManager.processes.Load(sessionID)
	assert.False(t, exists, "old process should be cleaned up")
}

func TestRingBuffer_WriteRead(t *testing.T) {
	buf := NewRingBuffer(16)

	n, err := buf.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)

	data := buf.ReadFrom(0)
	assert.Equal(t, "hello", string(data))
	assert.Equal(t, 5, buf.TotalWritten())

	data = buf.ReadFrom(5)
	assert.Equal(t, 0, len(data))
}

func TestRingBuffer_Overwrite(t *testing.T) {
	buf := NewRingBuffer(8)

	buf.Write([]byte("hello world")) // 11 bytes > 8 capacity

	data := buf.ReadFrom(0)
	assert.Equal(t, 8, len(data))
	assert.Equal(t, "lo world", string(data))
	assert.Equal(t, 11, buf.TotalWritten())
}

func TestRingBuffer_IncrementalPoll(t *testing.T) {
	buf := NewRingBuffer(256)

	buf.Write([]byte("first\n"))
	totalAfter1 := buf.TotalWritten()

	data := buf.ReadFrom(0)
	assert.Equal(t, "first\n", string(data))

	buf.Write([]byte("second\n"))
	data = buf.ReadFrom(totalAfter1)
	assert.Equal(t, "second\n", string(data))
	assert.Equal(t, totalAfter1+7, buf.TotalWritten())
}