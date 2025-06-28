package logging

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestLogger_LogLevels tests that each log level properly respects the configured level.
func TestLogger_LogLevels(t *testing.T) {
	origLogger := MainLogger
	defer func() { MainLogger = origLogger }()

	// Create a logger with Info level
	MainLogger = NewLogger(InfoLevel, "", NoneLevel, 0, 0, 0)
	MainLogger.SetLastMessagesLimit(10)

	infoMsg := "This info message should appear"
	debugMsg := "This debug message should not appear"

	// Info level should show Info, Warn, Error, Fatal but not Debug or Trace
	Debug(debugMsg)
	Info(infoMsg)

	// Check that Info is in the intercepted messages but Debug is not
	msg := MainLogger.GetLastMessages()
	assert.Contains(t, msg, infoMsg, "Info message was not logged when level is Info")
	assert.NotContains(t, msg, debugMsg, "Debug message was logged when level is Info")

	// Test with a context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Use context in a way similar to job_executor_test.go
	select {
	case <-ctx.Done():
		// This is just to demonstrate context usage like in job_executor_test.go
	default:
		Error("Error message with context")
	}

	// Verify the error message was logged
	msg = MainLogger.GetLastMessages()
	assert.Contains(t, msg, "Error message with context")
}

// TestLogger_WithFields tests the ability to add context fields to loggers.
func TestLogger_WithFields(t *testing.T) {
	MainLogger.SetConsoleLogLevel(InfoLevel)
	MainLogger.SetLastMessagesLimit(10)

	// Test WithFields
	contextLogger := MainLogger.WithFields(map[string]interface{}{
		"user_id": 123,
		"action":  "login",
	})

	contextLogger.Info("User logged in")

	// Check that the message was intercepted
	msg := contextLogger.GetLastMessages()
	assert.Contains(t, msg, "User logged in")
}

// TestLogger_LogMethods tests all the logging methods of Logger.
func TestLogger_LogMethods(t *testing.T) {
	MainLogger.SetConsoleLogLevel(TraceLevel)
	MainLogger.SetLastMessagesLimit(10)

	// Test all logging methods
	Trace("Test trace message: %d", 1)
	Debug("Test debug message: %d", 2)
	Info("Test info message: %d", 3)
	Warn("Test warn message: %d", 4)
	Error("Test error message: %d", 5)
	// Fatal("Test fatal message: %d", 6)

	// Check that all expected messages are present in the in-memory logs
	expectedMessages := []string{
		"Test trace message: 1", // Trace adds "TRACE:" prefix but the expected message is a substring
		"Test debug message: 2",
		"Test info message: 3",
		"Test warn message: 4",
		"Test error message: 5",
	}

	captured := MainLogger.GetLastMessages()
	for _, expected := range expectedMessages {
		if !strings.Contains(captured, expected) {
			t.Fatalf("Expected message '%s' not found in captured logs: %s", expected, captured)
		}
	}
}

// TestForceLogLevel tests that forcing a log level works.
func TestForceLogLevel(t *testing.T) {
	// Save original state
	origLogger := MainLogger
	var origForcedLevel *Level
	if forcedLogLevel != nil {
		levelCopy := *forcedLogLevel
		origForcedLevel = &levelCopy
	}
	defer func() {
		MainLogger = origLogger
		forcedLogLevel = origForcedLevel
	}()

	// Test forcing log level
	MainLogger = NewLogger(InfoLevel, "", ErrorLevel, 0, 0, 0)
	assert.Equal(t, InfoLevel, MainLogger.ConsoleLevel)
	assert.Equal(t, ErrorLevel, MainLogger.FileLevel)

	// Force to debug level
	ForceLogLevel(DebugLevel)
	assert.Equal(t, DebugLevel, MainLogger.ConsoleLevel)
	assert.Equal(t, DebugLevel, MainLogger.FileLevel)

	// Create a new logger - should respect forced level
	NewMainLogger := NewLogger(WarnLevel, "", ErrorLevel, 0, 0, 0)
	assert.Equal(t, DebugLevel, NewMainLogger.ConsoleLevel)
	assert.Equal(t, DebugLevel, NewMainLogger.FileLevel)

	// Force to a different level
	ForceLogLevel(TraceLevel)
	assert.Equal(t, TraceLevel, MainLogger.ConsoleLevel)
	assert.Equal(t, TraceLevel, MainLogger.FileLevel)
}

// TestStringToLevel tests the string to level conversion.
func TestStringToLevel(t *testing.T) {
	testCases := []struct {
		input    string
		expected Level
	}{
		{"trace", TraceLevel},
		{"debug", DebugLevel},
		{"info", InfoLevel},
		{"warn", WarnLevel},
		{"error", ErrorLevel},
		{"fatal", FatalLevel},
		{"invalid", InfoLevel}, // Default is Info
		{"", InfoLevel},        // Default is Info
	}

	for _, tc := range testCases {
		result := stringToLevel(tc.input)
		if result != tc.expected {
			t.Fatalf("stringToLevel(%q) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

// TestLogger_With tests the With method
func TestLogger_With(t *testing.T) {
	// Create a base logger
	logger := NewLogger(DebugLevel, "", DebugLevel, 0, 0, 0)

	// Test with nil logger
	var nilLogger *Logger
	result := nilLogger.With(zap.String("key", "value"))
	assert.Nil(t, result, "With() on nil logger should return nil")

	// Test with valid logger
	field := zap.String("testKey", "testValue")
	newLogger := logger.With(field)

	assert.NotNil(t, newLogger, "With() should return a non-nil logger")
	assert.Equal(t, logger.ConsoleLevel, newLogger.ConsoleLevel, "ConsoleLevel should be preserved")
	assert.Equal(t, logger.FileLevel, newLogger.FileLevel, "FileLevel should be preserved")
	assert.Equal(t, logger.FileName, newLogger.FileName, "FileName should be preserved")
}

// TestLogger_WithField tests the WithField method
func TestLogger_WithField(t *testing.T) {
	// Create a base logger
	logger := NewLogger(DebugLevel, "", DebugLevel, 0, 0, 0)

	// Test with nil logger
	var nilLogger *Logger
	result := nilLogger.WithField("key", "value")
	assert.Nil(t, result, "WithField() on nil logger should return nil")

	// Test with valid logger
	newLogger := logger.WithField("testKey", "testValue")

	assert.NotNil(t, newLogger, "WithField() should return a non-nil logger")
	assert.Equal(t, logger.ConsoleLevel, newLogger.ConsoleLevel, "ConsoleLevel should be preserved")
	assert.Equal(t, logger.FileLevel, newLogger.FileLevel, "FileLevel should be preserved")
	assert.Equal(t, logger.FileName, newLogger.FileName, "FileName should be preserved")
}

// TestNewLogger_NoneLevel tests creating a logger with NoneLevel for console
func TestNewLogger_NoneLevel(t *testing.T) {
	// Create a logger with NoneLevel for console
	logger := NewLogger(NoneLevel, "", InfoLevel, 0, 0, 0)

	// Verify the console level is set correctly
	assert.Equal(t, NoneLevel, logger.ConsoleLevel, "ConsoleLevel should be NoneLevel")
	assert.Equal(t, InfoLevel, logger.FileLevel, "FileLevel should be InfoLevel")

	// Set up message capture
	logger.SetLastMessagesLimit(10)

	// Log a message at various levels
	logger.Info("This info message should not appear in console")
	logger.Error("This error message should not appear in console")

	// Verify no messages were logged to console
	msg := logger.GetLastMessages()
	assert.Empty(t, msg, "No messages should be logged when ConsoleLevel is NoneLevel")

	// Test with both console and file as NoneLevel
	logger = NewLogger(NoneLevel, "", NoneLevel, 0, 0, 0)
	logger.SetLastMessagesLimit(10)
	logger.Info("This should not be logged anywhere")
	msg = logger.GetLastMessages()
	assert.Empty(t, msg, "No messages should be logged when both levels are NoneLevel")
}

// TestSetFileLogLevel tests the SetFileLogLevel method
func TestSetFileLogLevel(t *testing.T) {
	// Create a logger with different console and file levels
	logger := NewLogger(InfoLevel, "test_log.txt", ErrorLevel, 1, 1, 1)

	// Verify initial levels
	assert.Equal(t, InfoLevel, logger.ConsoleLevel, "Initial ConsoleLevel should be InfoLevel")
	assert.Equal(t, ErrorLevel, logger.FileLevel, "Initial FileLevel should be ErrorLevel")

	// Change file log level
	logger.SetFileLogLevel(DebugLevel)

	// Verify the level was changed
	assert.Equal(t, DebugLevel, logger.FileLevel, "FileLevel should be changed to DebugLevel")

	// Change to NoneLevel
	logger.SetFileLogLevel(NoneLevel)
	assert.Equal(t, NoneLevel, logger.FileLevel, "FileLevel should be changed to NoneLevel")

	// Ensure console level wasn't affected
	assert.Equal(t, InfoLevel, logger.ConsoleLevel, "ConsoleLevel should remain unchanged")
}

// TestLogger_LogMethods tests all the logging methods (Trace, Debug, Info, Warn, Error, Fatal, Log)
func TestLogger_LogMethods2(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(l *Logger, msg string, args ...interface{})
		level    Level
		msg      string
		args     []interface{}
		expected string
	}{
		{
			name: "Trace",
			logFunc: func(l *Logger, msg string, args ...interface{}) {
				l.Trace(msg, args...)
			},
			level:    TraceLevel,
			msg:      "Trace message: %s",
			args:     []interface{}{"test"},
			expected: "Trace message: test",
		},
		{
			name: "Debug",
			logFunc: func(l *Logger, msg string, args ...interface{}) {
				l.Debug(msg, args...)
			},
			level:    DebugLevel,
			msg:      "Debug message: %d",
			args:     []interface{}{42},
			expected: "Debug message: 42",
		},
		{
			name: "Info",
			logFunc: func(l *Logger, msg string, args ...interface{}) {
				l.Info(msg, args...)
			},
			level:    InfoLevel,
			msg:      "Info message: %s",
			args:     []interface{}{"info"},
			expected: "Info message: info",
		},
		{
			name: "Warn",
			logFunc: func(l *Logger, msg string, args ...interface{}) {
				l.Warn(msg, args...)
			},
			level:    WarnLevel,
			msg:      "Warning: %s %d",
			args:     []interface{}{"code", 123},
			expected: "Warning: code 123",
		},
		{
			name: "Error",
			logFunc: func(l *Logger, msg string, args ...interface{}) {
				l.Error(msg, args...)
			},
			level:    ErrorLevel,
			msg:      "Error occurred: %v",
			args:     []interface{}{"test error"},
			expected: "Error occurred: test error",
		},
		{
			name: "Log",
			logFunc: func(l *Logger, msg string, args ...interface{}) {
				l.Log(InfoLevel, msg, args...)
			},
			level:    InfoLevel,
			msg:      "Generic log: %s",
			args:     []interface{}{"message"},
			expected: "Generic log: message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a logger that captures logs to in-memory buffer
			logger := NewLogger(tt.level, "", tt.level, 0, 0, 0)
			logger.SetLastMessagesLimit(10)

			// Skip testing Fatal as it would terminate the program
			if tt.level != FatalLevel {
				tt.logFunc(logger, tt.msg, tt.args...)
				assert.Contains(t, logger.GetLastMessages(), tt.expected, "Message should be formatted correctly")
			}
		})
	}
}
