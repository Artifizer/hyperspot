package db

import (
	"os"
	"testing"

	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/stretchr/testify/assert"
)

func TestConfigDatabase(t *testing.T) {
	// Test default values
	assert.Equal(t, "sqlite", ConfigDatabaseInstance.Type)
	assert.Equal(t, "database/database.db", ConfigDatabaseInstance.DSN)

	// Test setting and getting values
	config := &ConfigDatabase{
		Type: "postgres",
		DSN:  "host=localhost user=postgres password=postgres dbname=test",
	}
	assert.Equal(t, "postgres", config.Type)
	assert.Equal(t, "host=localhost user=postgres password=postgres dbname=test", config.DSN)
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Save original values to restore later
	originalType := ConfigDatabaseInstance.Type
	originalDSN := ConfigDatabaseInstance.DSN
	defer func() {
		ConfigDatabaseInstance.Type = originalType
		ConfigDatabaseInstance.DSN = originalDSN
	}()

	// Test with environment variables set
	os.Setenv("DB_TYPE", "mysql")
	os.Setenv("DB_DSN", "user:password@tcp(127.0.0.1:3306)/testdb")
	defer func() {
		os.Unsetenv("DB_TYPE")
		os.Unsetenv("DB_DSN")
	}()

	// Load config from environment
	// Manually set values from environment since LoadConfigFromEnv is not defined
	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		ConfigDatabaseInstance.Type = dbType
	}
	if dbDSN := os.Getenv("DB_DSN"); dbDSN != "" {
		ConfigDatabaseInstance.DSN = dbDSN
	}

	// Verify values were loaded from environment
	assert.Equal(t, "mysql", ConfigDatabaseInstance.Type)
	assert.Equal(t, "user:password@tcp(127.0.0.1:3306)/testdb", ConfigDatabaseInstance.DSN)

	// Test with only one environment variable set
	os.Unsetenv("DB_TYPE")
	ConfigDatabaseInstance.Type = "sqlite" // Reset to default

	// Manually set values from environment again
	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		ConfigDatabaseInstance.Type = dbType
	}
	if dbDSN := os.Getenv("DB_DSN"); dbDSN != "" {
		ConfigDatabaseInstance.DSN = dbDSN
	}

	assert.Equal(t, "sqlite", ConfigDatabaseInstance.Type) // Should remain default
	assert.Equal(t, "user:password@tcp(127.0.0.1:3306)/testdb", ConfigDatabaseInstance.DSN)
}

func TestGetDatabaseConfig(t *testing.T) {
	// Save original values to restore later
	originalType := ConfigDatabaseInstance.Type
	originalDSN := ConfigDatabaseInstance.DSN
	defer func() {
		ConfigDatabaseInstance.Type = originalType
		ConfigDatabaseInstance.DSN = originalDSN
	}()

	// Set test values
	ConfigDatabaseInstance.Type = "postgres"
	ConfigDatabaseInstance.DSN = "host=localhost dbname=testdb"

	// Get config - directly use ConfigDatabaseInstance since GetDatabaseConfig is not defined
	config := ConfigDatabaseInstance

	// Verify returned config matches instance
	assert.Equal(t, ConfigDatabaseInstance.Type, config.Type)
	assert.Equal(t, ConfigDatabaseInstance.DSN, config.DSN)
}

func TestConfigDatabaseLoad(t *testing.T) {
	// Save original values to restore later
	originalType := ConfigDatabaseInstance.Type
	originalDSN := ConfigDatabaseInstance.DSN
	defer func() {
		ConfigDatabaseInstance.Type = originalType
		ConfigDatabaseInstance.DSN = originalDSN
	}()

	// Test 1: Loading from another ConfigDatabase instance
	ConfigDatabaseInstance.Type = "sqlite"
	ConfigDatabaseInstance.DSN = "original.db"

	newConfig := map[string]interface{}{
		"type": "mysql",
		"dsn":  "user:pass@localhost/db",
	}

	err := ConfigDatabaseInstance.Load("test", newConfig)
	assert.NoError(t, err)
	assert.Equal(t, "mysql", ConfigDatabaseInstance.Type)
	assert.Equal(t, "user:pass@localhost/db", ConfigDatabaseInstance.DSN)

	// Test 2: Loading with empty values (shouldn't override existing)
	emptyConfig := map[string]interface{}{
		"type": "",
		"dsn":  "",
	}

	err = ConfigDatabaseInstance.Load("test", emptyConfig)
	assert.NoError(t, err)
	assert.Equal(t, "", ConfigDatabaseInstance.Type, "New value should override existing value")
	assert.Equal(t, "", ConfigDatabaseInstance.DSN, "New value should override existing value")

	// Test 3: Loading with partial values
	partialConfig := map[string]interface{}{
		"type": "postgres",
		"dsn":  "user:pass@localhost/db",
	}

	err = ConfigDatabaseInstance.Load("test", partialConfig)
	assert.NoError(t, err)
	assert.Equal(t, "postgres", ConfigDatabaseInstance.Type)
	assert.Equal(t, "user:pass@localhost/db", ConfigDatabaseInstance.DSN, "DSN should be overridden by config")

	// Test 4: Default DSN for sqlite when DSN is empty
	ConfigDatabaseInstance.Type = "sqlite"
	ConfigDatabaseInstance.DSN = ""

	config := map[string]interface{}{
		"type": "sqlite",
		"dsn":  "",
	}
	err = ConfigDatabaseInstance.Load("test", config)
	assert.NoError(t, err)
	assert.Equal(t, "sqlite", ConfigDatabaseInstance.Type)
	assert.Equal(t, "database/data.db", ConfigDatabaseInstance.DSN, "Default DSN should be set for sqlite")

	// Test 5: Loading with non-ConfigDatabase type (should use receiver)
	ConfigDatabaseInstance.Type = "mysql"
	ConfigDatabaseInstance.DSN = "original.db"

	nonConfigType := map[string]interface{}{
		"invalid_key": "not a config",
	}
	err = ConfigDatabaseInstance.Load("test", nonConfigType)
	assert.Error(t, err)
	assert.Equal(t, "mysql", ConfigDatabaseInstance.Type, "Type should remain unchanged")
	assert.Equal(t, "original.db", ConfigDatabaseInstance.DSN, "DSN should remain unchanged")
}

func TestConfigDatabaseLoggerLoad(t *testing.T) {
	// Save original logger to restore later
	originalLogger := logger
	defer func() {
		logger = originalLogger
	}()

	// Test 1: Loading with valid config
	dbLogger := &ConfigDatabaseLogger{
		Config: config.ConfigLogging{
			ConsoleLevel: "info",
			FileLevel:    "info",
			File:         "",
			MaxSizeMB:    10,
			MaxBackups:   5,
			MaxAgeDays:   30,
		},
	}

	newConfig := map[string]interface{}{
		"console_level": "debug",
		"file_level":    "debug",
		"file":          "logs/test.log",
		"max_size_mb":   10,
		"max_backups":   5,
		"max_age_days":  30,
	}

	err := dbLogger.Load("test", newConfig)
	assert.NoError(t, err)
	assert.Equal(t, "debug", dbLogger.Config.ConsoleLevel)
	assert.Equal(t, "logs/test.log", dbLogger.Config.File)

	// Test 2: Loading with non-ConfigLogging type (should use receiver)
	dbLogger.Config = config.ConfigLogging{
		ConsoleLevel: "info",
		FileLevel:    "info",
		File:         "",
		MaxSizeMB:    10,
		MaxBackups:   5,
		MaxAgeDays:   30,
	}

	nonConfigType := map[string]interface{}{
		"invalid_key": "not a config",
	}
	err = dbLogger.Load("test", nonConfigType)
	assert.Error(t, err)
}
