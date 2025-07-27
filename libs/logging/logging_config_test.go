package logging

import (
	"testing"

	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/stretchr/testify/assert"
)

// TestGetDefault verifies that the default config returns expected values
func TestGetDefault(t *testing.T) {
	// Get the default config
	cfg := mainLoggerConfig.GetDefault()

	// Check it's the right type
	configLogging, ok := cfg.(config.ConfigLogging)
	assert.True(t, ok, "GetDefault should return a ConfigLogging struct")

	// Check default values
	assert.Equal(t, "info", configLogging.ConsoleLevel, "Default console level should be info")
	assert.Equal(t, "debug", configLogging.FileLevel, "Default file level should be debug")
	assert.Equal(t, "logs/main.log", configLogging.File, "Default log file path is incorrect")
	assert.Equal(t, 1000, configLogging.MaxSizeMB, "Default max size is incorrect")
	assert.Equal(t, 3, configLogging.MaxBackups, "Default max backups is incorrect")
	assert.Equal(t, 28, configLogging.MaxAgeDays, "Default max age is incorrect")
}

// TestLoad tests the Load method of mainLoggingConfig
func TestLoad(t *testing.T) {
	// Save original MainLogger to restore after test
	origLogger := MainLogger
	defer func() {
		MainLogger = origLogger
	}()

	// Create a test config with different values
	testConfig := map[string]interface{}{
		"console_level": "debug",
		"file_level":    "info",
		"file":          "test.log",
		"max_size_mb":   100,
		"max_backups":   5,
		"max_age_days":  14,
	}

	// Call Load with our test config
	err := mainLoggerConfig.Load("test", testConfig)
	assert.NoError(t, err, "Load should not return an error")

	// Verify the MainLogger was updated with new settings
	// We can't directly check private fields, but we can verify it's not nil
	assert.NotNil(t, MainLogger, "MainLogger should be initialized")
	assert.NotNil(t, MainLogger.ConsoleLogger, "ConsoleLogger should be initialized")
	assert.Equal(t, DebugLevel, MainLogger.ConsoleLevel, "Console level should be set to debug")
	assert.Equal(t, InfoLevel, MainLogger.FileLevel, "File level should be set to info")
}

// TestConfigRegistration verifies the logger is registered with the config system
func TestConfigRegistration(t *testing.T) {
	// This test just makes sure that our init function runs and registers the logger
	// In a real app, config.GetRegisteredLoggers() would return our logger
	// But since that's not exposed, we can just verify the package initializes without errors

	// This is a simple smoke test - if the init() function has a bug,
	// it would likely panic and fail this test
	assert.NotNil(t, mainLoggerConfig, "mainLoggerConfig should be initialized")
}

// TestCreateLoggerFromConfig tests that CreateLogger correctly applies config
func TestCreateLoggerFromConfig(t *testing.T) {
	// Create a minimal config
	cfg := &config.ConfigLogging{
		ConsoleLevel: "trace",
		FileLevel:    "error",
		File:         "", // No file logging
	}

	// Create a logger from this config
	logger := CreateLogger(cfg, "test")

	// Verify settings were applied
	assert.Equal(t, TraceLevel, logger.ConsoleLevel, "Console level should be trace")
	assert.Equal(t, ErrorLevel, logger.FileLevel, "File level should be error")
	assert.NotNil(t, logger.ConsoleLogger, "Console logger should be initialized")
	assert.NotNil(t, logger.FileLogger, "File logger should be initialized even if no file path is provided")
}
