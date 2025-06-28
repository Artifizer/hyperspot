package db

import (
	"os"
	"testing"

	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	// Run tests
	exitCode := m.Run()

	// Clean up
	os.Exit(exitCode)
}

func TestInitSQLite(t *testing.T) {
	// Setup temporary database file
	tempFile := "test_db.sqlite"
	defer os.Remove(tempFile)

	// Test with valid config
	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "sqlite",
		DSN:  tempFile,
	}

	gormConfig := &gorm.Config{}
	db, err := initSQLite(gormConfig)
	assert.NoError(t, err)
	assert.NotNil(t, db)

	// Test with nil logger
	oldLogger := logger
	logger = nil
	_, err = initSQLite(gormConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger not initialized")
	logger = oldLogger
}

func TestNewDB(t *testing.T) {
	// Test SQLite
	tempFile := "test_db.sqlite"
	defer os.Remove(tempFile)

	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "sqlite",
		DSN:  tempFile,
	}

	db, err := newDB()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	// Test in-memory database
	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "memory",
	}

	db, err = newDB()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	// Test unsupported database type
	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "unsupported",
	}

	_, err = newDB()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestHealthCheck(t *testing.T) {
	// Setup in-memory database
	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "memory",
	}

	db, err := newDB()
	assert.NoError(t, err)

	// Test health check
	err = healthCheck(db)
	assert.NoError(t, err)
}

func TestStartDBServer(t *testing.T) {
	// Test with valid config
	tempFile := "test_db.sqlite"
	defer os.Remove(tempFile)

	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "sqlite",
		DSN:  tempFile,
	}

	_, err := StartDBServer()
	assert.NoError(t, err)
	assert.NotNil(t, DB)

	// Test with invalid config
	ConfigDatabaseInstance = &ConfigDatabase{
		Type: "unsupported",
	}

	_, err = StartDBServer()
	assert.Error(t, err)
}

func TestLogDB(t *testing.T) {
	// Test logging at different levels
	logDB(logging.DebugLevel, "Debug message: %s", "test")
	logDB(logging.InfoLevel, "Info message: %s", "test")
	logDB(logging.WarnLevel, "Warn message: %s", "test")
	logDB(logging.ErrorLevel, "Error message: %s", "test")

	// Just ensuring these don't panic
	assert.True(t, true)
}
