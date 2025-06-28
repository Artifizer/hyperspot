package db

import (
	"context"
	"fmt"
	"time"

	"github.com/hypernetix/hyperspot/libs/logging"
	"go.uber.org/zap"
	gorm_logger "gorm.io/gorm/logger"
)

var logger *logging.Logger = logging.MainLogger // Initially set to main logger, can be overridden by config

func ForceLogLevel(level logging.Level) {
	logger.SetConsoleLogLevel(level)
	logger.SetFileLogLevel(level)
}

func logDB(_ logging.Level, msg string, args ...interface{}) {
	fields := []zap.Field{
		zap.String("msg", fmt.Sprintf(msg, args...)),
	}
	logger.FileLogger.With(fields...).Info(msg)
	logger.ConsoleLogger.Info(fmt.Sprintf(msg, args...))
}

func logQuery(query string, rows int64, elapsed time.Duration, err error) {
	msg := "SQL query"
	fields := []zap.Field{
		zap.String("msg", msg),
		zap.String("query", query),
		zap.Int64("rows", rows),
		zap.Duration("elapsed", elapsed),
		zap.Error(err),
	}
	logger.FileLogger.With(fields...).Trace(msg)
	consoleMsg := fmt.Sprintf("%s: %s, %d rows, %s", msg, query, rows, elapsed.String())
	logger.ConsoleLogger.Trace(consoleMsg)
}

type GormLogger struct {
	logging.Logger
	LogLevel gorm_logger.LogLevel
}

func (l *GormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

func (l *GormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

func (l *GormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

func (l *GormLogger) Debug(ctx context.Context, msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.Logger.ConsoleLevel < logging.TraceLevel && l.Logger.FileLevel < logging.TraceLevel {
		return
	}

	elapsed := time.Since(begin)
	sqlStr, rows := fc()
	logQuery(sqlStr, rows, elapsed, err)
}

func (l *GormLogger) LogMode(level gorm_logger.LogLevel) gorm_logger.Interface {
	l.LogLevel = level
	return l
}
