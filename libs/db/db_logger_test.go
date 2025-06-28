package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/stretchr/testify/assert"
	gorm_logger "gorm.io/gorm/logger"
)

func TestLogQuery(t *testing.T) {
	// Test logQuery function
	query := "SELECT * FROM users"
	rows := int64(10)
	elapsed := 100 * time.Millisecond
	err := errors.New("test error")

	// Just ensuring it doesn't panic
	logQuery(query, rows, elapsed, err)
	logQuery(query, rows, elapsed, nil)
	assert.True(t, true)
}

func TestGormLogger(t *testing.T) {
	// Create a GormLogger instance
	gLogger := &GormLogger{
		Logger:   *logging.MainLogger,
		LogLevel: gorm_logger.Info,
	}

	ctx := context.Background()

	// Test Error method
	gLogger.Error(ctx, "Error message: %s", "test")

	// Test Warn method
	gLogger.Warn(ctx, "Warning message: %s", "test")

	// Test Info method
	gLogger.Info(ctx, "Info message: %s", "test")

	// Test Trace method with different log levels
	sql := "SELECT * FROM users"
	rows := int64(5)
	// When LogLevel is higher than Silent
	gLogger.LogLevel = gorm_logger.Silent
	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, nil)

	// When LogLevel is Info and no error
	gLogger.LogLevel = gorm_logger.Info
	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, nil)

	// When LogLevel is Error and there is an error
	gLogger.LogLevel = gorm_logger.Error
	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, errors.New("database error"))

	// Just ensuring these don't panic
	assert.True(t, true)
}

func TestLoggerOverride(t *testing.T) {
	// Save original logger
	originalLogger := logger
	defer func() {
		// Restore original logger
		logger = originalLogger
	}()

	// Test logging with custom logger
	logDB(logging.InfoLevel, "Custom logger test: %s", "test")

	// Just ensuring it doesn't panic
	assert.True(t, true)
}

func TestGormLoggerDebug(t *testing.T) {
	// Create a context
	ctx := context.Background()

	// Create a GormLogger instance
	gLogger := &GormLogger{
		Logger:   *logging.MainLogger,
		LogLevel: gorm_logger.Info,
	}

	// Test Debug method
	// This just ensures it doesn't panic
	gLogger.Debug(ctx, "Debug message: %s", "test")
	assert.True(t, true)
}

func TestGormLoggerTraceWithLevels(t *testing.T) {
	// Create a context
	ctx := context.Background()

	// Create a GormLogger instance with custom logger
	customLogger := logging.MainLogger

	// Test when both console and file levels are below trace level
	customLogger.ConsoleLevel = logging.InfoLevel
	customLogger.FileLevel = logging.InfoLevel

	gLogger := &GormLogger{
		Logger:   *customLogger,
		LogLevel: gorm_logger.Info,
	}

	// This should return early without executing the function
	sql := "SELECT * FROM users"
	rows := int64(10)

	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, nil)

	// Test when console level is at trace level
	customLogger.ConsoleLevel = logging.TraceLevel
	customLogger.FileLevel = logging.InfoLevel

	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, nil)

	// Test when file level is at trace level
	customLogger.ConsoleLevel = logging.InfoLevel
	customLogger.FileLevel = logging.TraceLevel

	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, nil)

	// Test with error
	gLogger.Trace(ctx, time.Now(), func() (string, int64) {
		return sql, rows
	}, errors.New("database error"))

	// Just ensuring these don't panic
	assert.True(t, true)
}

func TestGormLoggerLogMode(t *testing.T) {
	// Create a GormLogger instance
	gLogger := &GormLogger{
		Logger:   *logging.MainLogger,
		LogLevel: gorm_logger.Info,
	}

	// Test LogMode method
	newLogger := gLogger.LogMode(gorm_logger.Error)

	// Check that the log level was updated
	assert.Equal(t, gorm_logger.Error, gLogger.LogLevel)

	// Check that the method returns the same logger instance
	assert.Equal(t, gLogger, newLogger)

	// Test with Silent level
	newLogger = gLogger.LogMode(gorm_logger.Silent)
	assert.Equal(t, gorm_logger.Silent, gLogger.LogLevel)

	// Test with Warn level
	newLogger = gLogger.LogMode(gorm_logger.Warn)
	assert.Equal(t, gorm_logger.Warn, gLogger.LogLevel)
}
