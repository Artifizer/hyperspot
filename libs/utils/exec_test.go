package utils

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExec(t *testing.T) {
	// Test successful command execution
	var cmd []string
	var expectedOutput string

	// Platform-specific tests
	switch runtime.GOOS {
	case "windows":
		cmd = []string{"cmd", "/c", "echo Hello World"}
		expectedOutput = "Hello World"
	default:
		cmd = []string{"echo", "Hello World"}
		expectedOutput = "Hello World"
	}

	ctx := context.Background()
	result := Exec(ctx, cmd, "")
	assert.Equal(t, 0, result.ExitCode, "Command should execute successfully")
	assert.Greater(t, len(result.Stdout), 0, "Should have output")
	if len(result.Stdout) > 0 {
		assert.Contains(t, result.Stdout[0], expectedOutput, "Output should contain expected text")
	}
	assert.Greater(t, result.Duration, time.Duration(0), "Duration should be positive")

	// Test command with empty args
	result = Exec(ctx, []string{}, "")
	assert.Equal(t, 1, result.ExitCode, "Empty command should fail")
	assert.Contains(t, result.Stderr[0], "no command provided", "Should report no command provided")

	// Test non-existent binary
	result = Exec(ctx, []string{"non_existent_binary_12345"}, "")
	assert.NotEqual(t, 0, result.ExitCode, "Non-existent binary should fail")
	assert.NotEmpty(t, result.Stderr, "Should have error output")

	// Test with invalid parameters to a valid command
	if runtime.GOOS == "windows" {
		result = Exec(ctx, []string{"type", "non_existent_file_12345.txt"}, "")
	} else {
		result = Exec(ctx, []string{"ls", "--invalid-flag-that-doesnt-exist"}, "")
	}
	assert.NotEqual(t, 0, result.ExitCode, "Command with invalid parameters should fail")
	assert.NotEmpty(t, result.Stderr, "Should have error output")

	// Test timeout/cancellation
	ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if runtime.GOOS == "windows" {
		result = Exec(ctxWithTimeout, []string{"ping", "-n", "10", "127.0.0.1"}, "")
	} else {
		result = Exec(ctxWithTimeout, []string{"sleep", "5"}, "")
	}
	assert.NotEqual(t, 0, result.ExitCode, "Timed out command should fail")
	assert.NotEmpty(t, result.Stderr, "Should have timeout error")
	assert.Contains(t, result.Stderr[0], context.DeadlineExceeded.Error(), "Should report context deadline exceeded error")
}
