package logging

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

// Level represents logging levels
type Level int

const (
	NoneLevel Level = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

const (
	// ServiceField is a field that is added to the log message to identify the service that the log message is from
	// example #1 - logging.CreateLogger(&c.Config, "db")
	// example #2 - logging.MainLogger.WithField(logging.ServiceField, "db")
	ServiceField = "service"
)

// Updated Logger struct to include LastInMemMessages field
type Logger struct {
	ConsoleLogger *zapLogger
	FileLogger    *zapLogger
	FileName      string
	ConsoleLevel  Level
	FileLevel     Level
}

// Initialize temporary logger, it will be overridden by config later
/*
var MainLogger *Logger = &Logger{
	ConsoleLogger: &zapLogger{logger: zap.NewNop(), levelSetter: &zap.NewAtomicLevel()},
	ConsoleLevel:  DebugLevel,
	FileLogger:    &zapLogger{logger: zap.NewNop(), levelSetter: &zap.NewAtomicLevel()},
	FileLevel:     ErrorLevel, // default 0
}*/

var MainLogger *Logger = NewLogger(InfoLevel, "", DebugLevel, 0, 0, 0).WithField(ServiceField, "main")

var forcedLogLevel *Level = nil

func ForceLogLevel(level Level) {
	forcedLogLevel = &level
	if MainLogger != nil {
		MainLogger.SetConsoleLogLevel(level)
		MainLogger.SetFileLogLevel(level)
	}
}

func CreateLogger(cfg *config.ConfigLogging, serviceName string) *Logger {
	// Resolve log file path to absolute path in .hyperspot directory
	logFilePath := resolveLogFilePath(cfg.File)

	return NewLogger(
		stringToLevel(cfg.ConsoleLevel),
		logFilePath,
		stringToLevel(cfg.FileLevel),
		cfg.MaxSizeMB,
		cfg.MaxBackups,
		cfg.MaxAgeDays,
	).WithField(ServiceField, serviceName)
}

// Updated NewLogger to accept lastInMemMessages parameter
func NewLogger(
	consoleLevel Level,
	fileName string,
	fileLevel Level,
	maxSizeMB int,
	maxBackups int,
	maxAgeDays int,
) *Logger {
	if forcedLogLevel != nil {
		consoleLevel = *forcedLogLevel
		fileLevel = *forcedLogLevel
	}

	var consoleLogger, fileLogger *zapLogger

	// Console logger setup
	if consoleLevel == NoneLevel {
		consoleLogger = &zapLogger{
			logger: zap.NewNop(),
			// set the console buffer size
		}
	} else {
		atom := zap.NewAtomicLevel()
		atom.SetLevel(getZapLevel(consoleLevel))

		// Create a trace-capable level enabler that wraps the atomic level
		traceEnabler := NewTraceCapableLevelEnabler(atom, consoleLevel >= TraceLevel)

		consoleEncoder := NewConsoleEncoderWithFields(getConsoleEncoderConfig())
		consoleWriter := zapcore.AddSync(os.Stdout)
		consoleCore := zapcore.NewCore(consoleEncoder,
			consoleWriter,
			traceEnabler)
		consoleLogger = &zapLogger{
			logger:       zap.New(consoleCore),
			level:        consoleLevel,
			levelSetter:  &atom,
			traceEnabler: traceEnabler,
			isTerminal:   isTerminal(),
		}
	}

	// File logger setup
	if fileLevel == NoneLevel && fileName == "" {
		fileLogger = &zapLogger{logger: zap.NewNop()}
	} else {
		atom := zap.NewAtomicLevel()
		atom.SetLevel(getZapLevel(fileLevel))

		// Create a trace-capable level enabler that wraps the atomic level
		traceEnabler := NewTraceCapableLevelEnabler(atom, fileLevel >= TraceLevel)

		// Ensure the log directory exists
		if fileName != "" {
			if err := os.MkdirAll(filepath.Dir(fileName), 0755); err != nil {
				// If we can't create the directory, log to stderr and continue
				fmt.Fprintf(os.Stderr, "Warning: Failed to create log directory %s: %v\n", filepath.Dir(fileName), err)
			}
		}

		fileEncoder := zapcore.NewJSONEncoder(getFileEncoderConfig())
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   fileName,
			MaxSize:    maxSizeMB, // MB
			MaxBackups: maxBackups,
			MaxAge:     maxAgeDays, // days
		})
		fileCore := zapcore.NewCore(fileEncoder, fileWriter, traceEnabler)
		fileLogger = &zapLogger{
			logger:       zap.New(fileCore),
			level:        fileLevel,
			levelSetter:  &atom,
			traceEnabler: traceEnabler,
			isTerminal:   false,
		}
	}

	return &Logger{
		ConsoleLogger: consoleLogger,
		FileLogger:    fileLogger,
		FileName:      fileName,
		ConsoleLevel:  consoleLevel,
		FileLevel:     fileLevel, // store the parameter
	}
}

// Internal log function
func logOne(logger *zapLogger, level Level, msg string, fields ...zap.Field) {
	if logger == nil {
		return
	}

	switch level {
	case TraceLevel:
		logger.Trace(msg, fields...)
	case DebugLevel:
		logger.Debug(msg, fields...)
	case InfoLevel:
		logger.Info(msg, fields...)
	case WarnLevel:
		logger.Warn(msg, fields...)
	case ErrorLevel:
		logger.Error(msg, fields...)
	case FatalLevel:
		logger.Fatal(msg, fields...)
	}
}

// Helper functions
func getConsoleEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewDevelopmentEncoderConfig()
	// Only use colored output if stdout is a terminal
	if isTerminal() {
		cfg.EncodeLevel = traceCapableColorLevelEncoder
	} else {
		cfg.EncodeLevel = traceCapableLevelEncoder
	}
	cfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006/01/02 15:04:05.000")
	return cfg
}

// traceCapableLevelEncoder is a custom level encoder that properly displays TRACE level with 5-char padding
func traceCapableLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch l {
	case ZapTraceLevel:
		enc.AppendString("TRACE")
	case zapcore.DebugLevel:
		enc.AppendString("DEBUG")
	case zapcore.InfoLevel:
		enc.AppendString("INFO ")
	case zapcore.WarnLevel:
		enc.AppendString("WARN ")
	case zapcore.ErrorLevel:
		enc.AppendString("ERROR")
	case zapcore.FatalLevel:
		enc.AppendString("FATAL")
	default:
		zapcore.CapitalLevelEncoder(l, enc)
	}
}

// traceCapableColorLevelEncoder is a custom level encoder that properly displays colored TRACE level with 5-char padding
func traceCapableColorLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch l {
	case ZapTraceLevel:
		enc.AppendString(color.BlueString("TRACE"))
	case zapcore.DebugLevel:
		enc.AppendString(color.MagentaString("DEBUG"))
	case zapcore.InfoLevel:
		enc.AppendString(color.CyanString("INFO "))
	case zapcore.WarnLevel:
		enc.AppendString(color.YellowString("WARN "))
	case zapcore.ErrorLevel:
		enc.AppendString(color.RedString("ERROR"))
	case zapcore.FatalLevel:
		enc.AppendString(color.RedString("FATAL"))
	default:
		zapcore.CapitalColorLevelEncoder(l, enc)
	}
}

// isTerminal checks if os.Stdout is a terminal
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func getFileEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncodeLevel = traceCapableLevelEncoder
	return cfg
}

func getZapLevel(level Level) zapcore.Level {
	switch level {
	case TraceLevel:
		return ZapTraceLevel
	case DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

func stringToLevel(level string) Level {
	switch level {
	case "trace":
		return TraceLevel
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal":
		return FatalLevel
	default:
		return InfoLevel
	}
}

func levelToString(level Level) string {
	switch level {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	default:
		return "info"
	}
}

func Trace(msg string, args ...interface{}) {
	MainLogger.Log(TraceLevel, msg, args...)
}

func Debug(msg string, args ...interface{}) {
	MainLogger.Log(DebugLevel, msg, args...)
}

func Info(msg string, args ...interface{}) {
	MainLogger.Log(InfoLevel, msg, args...)
}

func Warn(msg string, args ...interface{}) {
	MainLogger.Log(WarnLevel, msg, args...)
}

func Error(msg string, args ...interface{}) {
	MainLogger.Log(ErrorLevel, msg, args...)
}

func Fatal(msg string, args ...interface{}) {
	MainLogger.Log(FatalLevel, msg, args...)
}

func (l *Logger) GetMinLogLevel() Level {
	if l.ConsoleLevel < l.FileLevel {
		return l.ConsoleLevel
	}
	return l.FileLevel
}

// Add this method to the Logger struct
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	if l == nil {
		return nil
	}

	// Convert map to zap fields
	zapFields := make([]zap.Field, 0, len(fields))
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}

	return &Logger{
		ConsoleLogger: l.ConsoleLogger.With(zapFields...),
		FileLogger:    l.FileLogger.With(zapFields...),
		ConsoleLevel:  l.ConsoleLevel,
		FileLevel:     l.FileLevel,
	}
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	if l == nil {
		return nil
	}
	return &Logger{
		ConsoleLogger: l.ConsoleLogger.With(fields...),
		FileLogger:    l.FileLogger.With(fields...),
		ConsoleLevel:  l.ConsoleLevel,
		FileLevel:     l.FileLevel,
		FileName:      l.FileName,
	}
}

func (l *Logger) WithField(key string, value interface{}) *Logger {
	if l == nil {
		return nil
	}
	return l.WithFields(map[string]interface{}{key: value})
}

func (l *Logger) Trace(msg string, args ...interface{}) {
	l.Log(TraceLevel, msg, args...)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Log(DebugLevel, msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.Log(InfoLevel, msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Log(WarnLevel, msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.Log(ErrorLevel, msg, args...)
}

func (l *Logger) Fatal(msg string, args ...interface{}) {
	l.Log(FatalLevel, msg, args...)
}

func (l *Logger) Log(level Level, msg string, args ...interface{}) {
	if level > l.ConsoleLevel && level > l.FileLevel {
		return
	}

	msgToLog := fmt.Sprintf(msg, args...)

	logOne(l.ConsoleLogger, level, msgToLog)
	logOne(l.FileLogger, level, msgToLog)
}

func (l *Logger) GetLastMessages() string {
	msgs := ""
	for _, msg := range l.ConsoleLogger.GetLastMessages() {
		msgs += fmt.Sprintf("%v\n", msg)
	}
	return msgs
}

func (l *Logger) SetLastMessagesLimit(limit int) {
	l.ConsoleLogger.SetInMemoryMessagesLimit(limit)
}

func (l *Logger) SetConsoleLogLevel(level Level) {
	l.ConsoleLevel = level
	l.ConsoleLogger.SetLogLevel(level)
}

func (l *Logger) SetFileLogLevel(level Level) {
	l.FileLevel = level
	l.FileLogger.SetLogLevel(level)
}

// resolveLogFilePath converts relative log file paths to absolute paths in the configured server home directory
func resolveLogFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}

	// If already absolute, return as-is
	if filepath.IsAbs(filePath) {
		return filePath
	}

	// For relative paths, resolve to the configured server home directory
	// First try to get the configured server home directory
	cfg := config.Get()
	var homeDir string
	var err error

	if cfg != nil && cfg.Server.HomeDir != "" {
		// Use the configured server home directory
		homeDir, err = config.ResolveHomeDir(cfg.Server.HomeDir)
	} else {
		// Fall back to default .hyperspot directory
		homeDir, err = config.GetHyperspotHomeDir()
	}

	if err != nil {
		// Fallback to original path if resolution fails
		fmt.Fprintf(os.Stderr, "Warning: Failed to resolve home directory: %v\n", err)
		return filePath
	}

	return filepath.Join(homeDir, filePath)
}
