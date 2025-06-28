package logging

import (
	"go.uber.org/zap/zapcore"
)

// Define a custom zapcore level for trace
const ZapTraceLevel = zapcore.DebugLevel - 1

// TraceCapableLevelEnabler is a custom level enabler that supports trace level
type TraceCapableLevelEnabler struct {
	zapcore.LevelEnabler
	traceEnabled bool
}

// Enabled implements the zapcore.LevelEnabler interface
func (e *TraceCapableLevelEnabler) Enabled(lvl zapcore.Level) bool {
	// Special handling for trace level
	if lvl == ZapTraceLevel {
		return e.traceEnabled
	}
	// For all other levels, delegate to the wrapped enabler
	return e.LevelEnabler.Enabled(lvl)
}

// EnableTrace enables or disables trace logging
func (e *TraceCapableLevelEnabler) EnableTrace(enabled bool) {
	e.traceEnabled = enabled
}

// NewTraceCapableLevelEnabler creates a new level enabler that supports trace level
func NewTraceCapableLevelEnabler(baseEnabler zapcore.LevelEnabler, traceEnabled bool) *TraceCapableLevelEnabler {
	return &TraceCapableLevelEnabler{
		LevelEnabler: baseEnabler,
		traceEnabled: traceEnabled,
	}
}
