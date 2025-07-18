package chat

import (
	"fmt"

	"github.com/hypernetix/hyperspot/libs/logging"
	"go.uber.org/zap"
)

var logger = logging.MainLogger

func (t *ChatThread) Log(level logging.Level, msg string, args ...interface{}) {
	msg = fmt.Sprintf(msg, args...)
	fields := []zap.Field{
		zap.String("thread_id", t.ID.String()),
	}

	textMsg := fmt.Sprintf("Thread %s: %s", t.ID.String(), msg)

	logger.ConsoleLogger.Log(level, textMsg)
	logger.FileLogger.With(fields...).Log(level, msg)
}

// Job logs
func (t *ChatThread) LogTrace(msg string, args ...interface{}) {
	t.Log(logging.TraceLevel, msg, args...)
}

func (t *ChatThread) LogDebug(msg string, args ...interface{}) {
	t.Log(logging.DebugLevel, msg, args...)
}

// Job logs
func (t *ChatThread) LogInfo(msg string, args ...interface{}) {
	t.Log(logging.InfoLevel, msg, args...)
}

// Job logs
func (t *ChatThread) LogWarn(msg string, args ...interface{}) {
	t.Log(logging.WarnLevel, msg, args...)
}

// Job logs
func (t *ChatThread) LogError(msg string, args ...interface{}) {
	t.Log(logging.ErrorLevel, msg, args...)
}
