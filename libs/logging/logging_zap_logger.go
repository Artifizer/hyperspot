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

	// Store fields separately to avoid JSON serialization
	contextFields []zap.Field
}

func (zl *zapLogger) addToMemory(level Level, msg string, fields ...zap.Field) {
	zl.mu.Lock()
	defer zl.mu.Unlock()

	if zl.lastMessagesLimit <= 0 {
		zl.lastMessages = []any{}
		return
	}

	if level > zl.level {
		return
	}

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
	// Combine context fields with additional fields
	allFields := append(zl.contextFields, fields...)
	if ce := zl.logger.Check(zapcore.Level(-2), msg); ce != nil {
		ce.Write(allFields...)
	}
	zl.addToMemory(TraceLevel, msg, allFields...)
}

func (zl *zapLogger) Debug(msg string, fields ...zap.Field) {
	if zl.isTerminal {
		msg = color.MagentaString(msg)
	}
	// Combine context fields with additional fields
	allFields := append(zl.contextFields, fields...)
	zl.logger.Debug(msg, allFields...)
	zl.addToMemory(DebugLevel, msg, allFields...)
}

func (zl *zapLogger) Info(msg string, fields ...zap.Field) {
	// Combine context fields with additional fields
	allFields := append(zl.contextFields, fields...)
	zl.logger.Info(msg, allFields...)
	zl.addToMemory(InfoLevel, msg, allFields...)
}

func (zl *zapLogger) Warn(msg string, fields ...zap.Field) {
	// Combine context fields with additional fields
	allFields := append(zl.contextFields, fields...)
	zl.logger.Warn(msg, allFields...)
	zl.addToMemory(WarnLevel, msg, allFields...)
}

func (zl *zapLogger) Error(msg string, fields ...zap.Field) {
	// Combine context fields with additional fields
	allFields := append(zl.contextFields, fields...)
	zl.logger.Error(msg, allFields...)
	zl.addToMemory(ErrorLevel, msg, allFields...)
}

func (zl *zapLogger) Fatal(msg string, fields ...zap.Field) {
	// Combine context fields with additional fields
	allFields := append(zl.contextFields, fields...)
	zl.logger.Fatal(msg, allFields...)
	zl.addToMemory(FatalLevel, msg, allFields...)
}

func (zl *zapLogger) With(fields ...zap.Field) *zapLogger {
	// Store fields separately instead of using zap's With to avoid JSON serialization
	newFields := make([]zap.Field, len(zl.contextFields)+len(fields))
	copy(newFields, zl.contextFields)
	copy(newFields[len(zl.contextFields):], fields)

	return &zapLogger{
		logger:            zl.logger, // Use same underlying logger
		lastMessagesLimit: zl.lastMessagesLimit,
		level:             zl.level,
		levelSetter:       zl.levelSetter,
		traceEnabler:      zl.traceEnabler,
		isTerminal:        zl.isTerminal,
		contextFields:     newFields,
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
	return zl.With(zap.Any(key, value))
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
