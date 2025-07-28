package logging

import (
	"fmt"
	"math"
	"strings"

	"github.com/fatih/color"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

type consoleEncoderWithFields struct {
	zapcore.Encoder
	config zapcore.EncoderConfig
}

// NewConsoleEncoderWithFields creates a new console encoder with fields using the provided config
func NewConsoleEncoderWithFields(config zapcore.EncoderConfig) zapcore.Encoder {
	// We need to disable the MessageKey in the config so that the underlying encoder
	// doesn't include the message in its output - we'll handle that ourselves
	configCopy := config
	configCopy.MessageKey = "" // Disable message output in the base encoder

	return &consoleEncoderWithFields{
		Encoder: zapcore.NewConsoleEncoder(configCopy),
		config:  config,
	}
}

// Clone required by zapcore.Encoder interface
func (c *consoleEncoderWithFields) Clone() zapcore.Encoder {
	return &consoleEncoderWithFields{Encoder: c.Encoder.Clone(), config: c.config}
}

// EncodeEntry defines the custom encoding for log entries
func (c *consoleEncoderWithFields) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// Get the original encoding for timestamp and level
	origBuf, err := c.Encoder.EncodeEntry(entry, nil)
	if err != nil {
		return nil, err
	}

	// Get the original string without the message and newline
	origStr := origBuf.String()
	if len(origStr) > 0 && origStr[len(origStr)-1] == '\n' {
		origStr = origStr[:len(origStr)-1]
	}

	// Create our output buffer
	buf := bufferpool.Get()

	// Add the formatted prefix (timestamp, level, etc)
	buf.AppendString(origStr)

	// Ensure there's always a space after the prefix
	if len(origStr) > 0 && !strings.HasSuffix(origStr, " ") {
		buf.AppendByte(' ')
	}

	// Custom fields formatting: [service] other_field: value
	if len(fields) > 0 {
		// Build fields string separately to apply color to it
		var fieldsBuilder strings.Builder

		// First, check for service field and format it specially
		var serviceValue string
		var otherFields []zapcore.Field

		for _, f := range fields {
			if f.Key == ServiceField {
				// Extract service value
				switch f.Type {
				case zapcore.StringType:
					serviceValue = f.String
				default:
					if f.Interface != nil {
						serviceValue = fmt.Sprintf("%v", f.Interface)
					} else {
						serviceValue = fmt.Sprintf("%v", f.Integer)
					}
				}
			} else {
				// Collect other fields
				otherFields = append(otherFields, f)
			}
		}

		// Add service field in square brackets if it exists
		if serviceValue != "" {
			fieldsBuilder.WriteString("[")
			fieldsBuilder.WriteString(serviceValue)
			fieldsBuilder.WriteString("]")

			// Add space if there are other fields
			if len(otherFields) > 0 {
				fieldsBuilder.WriteString(" ")
			}
		}

		// Process other fields in key: value format
		for i, f := range otherFields {
			if i > 0 {
				fieldsBuilder.WriteString(", ")
			}

			// Add the field key
			fieldsBuilder.WriteString(f.Key)
			fieldsBuilder.WriteString(": ")

			// Add the field value based on its type
			switch f.Type {
			case zapcore.StringType:
				fieldsBuilder.WriteString(f.String)
			case zapcore.BoolType:
				if f.Integer == 1 {
					fieldsBuilder.WriteString("true")
				} else {
					fieldsBuilder.WriteString("false")
				}
			case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
				fieldsBuilder.WriteString(fmt.Sprintf("%d", f.Integer))
			case zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type:
				fieldsBuilder.WriteString(fmt.Sprintf("%d", uint64(f.Integer)))
			case zapcore.Float64Type, zapcore.Float32Type:
				fieldsBuilder.WriteString(fmt.Sprintf("%g", math.Float64frombits(uint64(f.Integer))))
			default:
				// For complex types or types we don't handle specifically
				if f.Interface != nil {
					fieldsBuilder.WriteString(fmt.Sprintf("%v", f.Interface))
				} else {
					fieldsBuilder.WriteString(fmt.Sprintf("%v", f.Integer))
				}
			}
		}

		// Add separator only if there are other fields besides service
		if len(otherFields) > 0 && entry.Message != "" {
			fieldsBuilder.WriteString(" - ")
		} else if serviceValue != "" && len(otherFields) == 0 && entry.Message != "" {
			// Add just a space when there's only service field and a message
			fieldsBuilder.WriteString(" ")
		}

		// Get the fields string and apply color to it based on log level
		fieldsStr := fieldsBuilder.String()
		if isTerminal() {
			switch entry.Level {
			case ZapTraceLevel:
				fieldsStr = color.BlueString(fieldsStr)
			case zapcore.DebugLevel:
				fieldsStr = color.MagentaString(fieldsStr)
			case zapcore.InfoLevel:
				fieldsStr = color.CyanString(fieldsStr)
			}
		}

		// Add the colored fields string to the buffer
		buf.AppendString(fieldsStr)
	}

	// Append the main log message (uncolored)
	buf.AppendString(entry.Message)
	buf.AppendByte('\n')

	return buf, nil
}

var bufferpool = buffer.NewPool()
