package api_client

import (
	"testing"

	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/stretchr/testify/assert"
)

func TestUpstreamLoggerConfig_GetDefault(t *testing.T) {
	// Expected default logging config as defined in production.
	expected := config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "debug",
		File:         "logs/upstream.log",
		MaxSizeMB:    1000,
		MaxBackups:   3,
		MaxAgeDays:   28,
	}

	// Get the default config from the instance.
	got := upstreamLoggerConfigInstance.GetDefault()

	// Because GetDefault returns an interface{}, we assert that it is of type config.ConfigLogging.
	defaultCfg, ok := got.(config.ConfigLogging)
	assert.True(t, ok, "GetDefault() should return a config.ConfigLogging value")
	assert.Equal(t, expected, defaultCfg, "Default config does not match expected values")
}

func TestUpstreamLoggerConfig_Load_Valid(t *testing.T) {
	// Create a custom configuration.
	customConfig := map[string]interface{}{
		"console_level": "debug",
		"file_level":    "error",
		"file":          "logs/custom.log",
		"max_size_mb":   500,
		"max_backups":   5,
		"max_age_days":  7,
	}

	assert.Equal(t, logger, logging.MainLogger, "Logger should be the main logger")

	// Call Load with the custom config.
	err := upstreamLoggerConfigInstance.Load("test", customConfig)
	assert.NoError(t, err, "Load() should not return an error when provided a valid config")

	assert.NotNil(t, logger, "Logger should not be nil")
	assert.NotEqual(t, logger, logging.MainLogger, "Logger should not be the main logger")

	// Optionally log a debug message (this message should be emitted via the newly created logger).
	// Note: In your production logger, this will write to the ConsoleLogger.
	logger.ConsoleLogger.Debug("Test debug message: valid config loaded")
}

func TestUpstreamLoggerConfig_Load_Invalid(t *testing.T) {
	// Pass an invalid config (not of type *config.ConfigLogging); Load() should fall back to the default.
	invalidConfig := map[string]interface{}{
		"invalid_key": "invalid_value",
	}
	err := upstreamLoggerConfigInstance.Load("test", invalidConfig)
	assert.Error(t, err, "Load() should return an error when provided an invalid config type; it should fall back to the default")
}
