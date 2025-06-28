package logging

import (
	"fmt"
	"strings"
	"sync"

	"github.com/fatih/color"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Updated zapLogger struct to hold in-memory messages
type zapLogger struct {
	logger       *zap.Logger
	level        Level
	levelSetter  *zap.AtomicLevel
	traceEnabler *TraceCapableLevelEnabler

	mu                sync.Mutex
	lastMessages      []any
	lastMessagesLimit int
	isTerminal        bool
}

func (zl *zapLogger) addToMemory(level Level, msg string, fields ...zap.Field) {
	if zl.lastMessagesLimit <= 0 {
		zl.lastMessages = []any{}
		return
	}

	if level > zl.level {
		return
	}

	zl.mu.Lock()
	defer zl.mu.Unlock()

	msg = fmt.Sprintf("%s: %s", strings.ToUpper(levelToString(level)), msg)

	zl.lastMessages = append(zl.lastMessages, msg)
	if len(zl.lastMessages) > zl.lastMessagesLimit {
		zl.lastMessages = zl.lastMessages[1:]
	}
}

func (zl *zapLogger) Trace(msg string, fields ...zap.Field) {
	// Use Check to respect log level filtering
	if zl.isTerminal {
		msg = color.BlueString(msg)
	}
	if ce := zl.logger.Check(zapcore.Level(-2), msg); ce != nil {
		ce.Write(fields...)
	}
	zl.addToMemory(TraceLevel, msg, fields...)
}

func (zl *zapLogger) Debug(msg string, fields ...zap.Field) {
	if zl.isTerminal {
		msg = color.MagentaString(msg)
	}
	zl.logger.Debug(msg, fields...)
	zl.addToMemory(DebugLevel, msg, fields...)
}

func (zl *zapLogger) Info(msg string, fields ...zap.Field) {
	zl.logger.Info(msg, fields...)
	zl.addToMemory(InfoLevel, msg, fields...)
}

func (zl *zapLogger) Warn(msg string, fields ...zap.Field) {
	zl.logger.Warn(msg, fields...)
	zl.addToMemory(WarnLevel, msg, fields...)
}

func (zl *zapLogger) Error(msg string, fields ...zap.Field) {
	zl.logger.Error(msg, fields...)
	zl.addToMemory(ErrorLevel, msg, fields...)
}

func (zl *zapLogger) Fatal(msg string, fields ...zap.Field) {
	zl.logger.Fatal(msg, fields...)
	zl.addToMemory(FatalLevel, msg, fields...)
}

func (zl *zapLogger) With(fields ...zap.Field) *zapLogger {
	return &zapLogger{
		logger:            zl.logger.With(fields...),
		lastMessagesLimit: zl.lastMessagesLimit,
		level:             zl.level,
		levelSetter:       zl.levelSetter,
		// Notice: Not copying lastMessages to start fresh
	}
}

func (zl *zapLogger) Log(level Level, msg string, args ...interface{}) {
	formattedMsg := fmt.Sprintf(msg, args...)
	switch level {
	case TraceLevel:
		zl.Trace(formattedMsg)
	case DebugLevel:
		zl.Debug(formattedMsg)
	case InfoLevel:
		zl.Info(formattedMsg)
	case WarnLevel:
		zl.Warn(formattedMsg)
	case ErrorLevel:
		zl.Error(formattedMsg)
	case FatalLevel:
		zl.Fatal(formattedMsg)
	default:
		zl.Info(formattedMsg)
	}
}

func (zl *zapLogger) WithField(key string, value interface{}) *zapLogger {
	return &zapLogger{
		logger:            zl.logger.With(zap.Any(key, value)),
		lastMessagesLimit: zl.lastMessagesLimit,
		level:             zl.level,
		levelSetter:       zl.levelSetter,
		// Notice: Not copying lastMessages to start fresh
	}
}

func (zl *zapLogger) SetLogLevel(level Level) {
	if zl.levelSetter != nil {
		zl.levelSetter.SetLevel(getZapLevel(level))
	}
	// Update trace enabler if available
	if zl.traceEnabler != nil {
		zl.traceEnabler.EnableTrace(level >= TraceLevel)
	}
	zl.level = level
}

func (zl *zapLogger) GetLastMessages() []any {
	zl.mu.Lock()
	defer zl.mu.Unlock()
	return zl.lastMessages
}

func (zl *zapLogger) SetInMemoryMessagesLimit(limit int) {
	zl.mu.Lock()
	defer zl.mu.Unlock()
	zl.lastMessagesLimit = limit
}
