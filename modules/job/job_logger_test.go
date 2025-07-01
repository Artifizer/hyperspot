package job

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/hypernetix/hyperspot/libs/logging"
)

// captureOutput temporarily redirects the given writer (stdout or stderr) while f executes,
// then returns everything written as a string.
func captureOutput(getWriter func() *os.File, setWriter func(*os.File), f func()) string {
	// Save the original writer.
	orig := getWriter()
	// Create a pipe to capture output.
	r, w, _ := os.Pipe()
	setWriter(w)

	// Execute the function.
	f()

	// Close the writer and restore the original.
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	setWriter(orig)
	return buf.String()
}

// captureStdout captures writes to os.Stdout during execution of f.
// This works with zap-based loggers because zap's console encoder writes to os.Stdout.
// The function returns all captured output as a string.
func captureStdout(f func()) string {
	return captureOutput(
		func() *os.File { return os.Stdout },
		func(new *os.File) { os.Stdout = new },
		f,
	)
}

// captureStderr captures writes to os.Stderr during execution of f.
func captureStderr(f func()) string {
	return captureOutput(
		func() *os.File { return os.Stderr },
		func(new *os.File) { os.Stderr = new },
		f,
	)
}

// interceptedMessages holds all log messages intercepted by our mock logger.
var interceptedMessages []string

// mockCore implements zapcore.Core so that every Write call appends the message to interceptedMessages.
type mockCore struct {
	level zapcore.Level
}

func (m *mockCore) Enabled(lvl zapcore.Level) bool {
	return lvl >= m.level
}

func (m *mockCore) With(fields []zap.Field) zapcore.Core {
	// In this simple mock, we ignore fields.
	return m
}

func (m *mockCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if m.Enabled(ent.Level) {
		return ce.AddCore(ent, m)
	}
	return ce
}

func (m *mockCore) Write(ent zapcore.Entry, fields []zap.Field) error {
	// Append the level (capitalized) and message to interceptedMessages.
	interceptedMessages = append(interceptedMessages, fmt.Sprintf("%s: %s", ent.Level.CapitalString(), ent.Message))
	return nil
}

func (m *mockCore) Sync() error {
	return nil
}

// TestJobLogger_LogInfo tests that LogInfo writes the expected output to stdout.
func TestJobLogger_Logging(t *testing.T) {
	// Create a dummy Job for logging.
	j := &JobObj{}

	logger.SetConsoleLogLevel(logging.InfoLevel)
	logger.SetLastMessagesLimit(10)

	j.LogTrace("Test log trace: %d", 41)
	j.LogDebug("Test log debug: %d", 42)
	j.LogInfo("Test log info: %d", 43)
	j.LogWarn("Test log warn: %d", 44)
	j.LogError("Test log error: %d", 45)

	msg := logger.GetLastMessages()
	assert.NotContains(t, msg, "Test log trace: 41")
	assert.NotContains(t, msg, "Test log debug: 42")
	assert.Contains(t, msg, "Test log info: 43")
	assert.Contains(t, msg, "Test log warn: 44")
	assert.Contains(t, msg, "Test log error: 45")
}
