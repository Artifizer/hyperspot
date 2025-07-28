// API request logs
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"go.uber.org/zap"
)

var LoggerConfigKey = "api"

var logger *logging.Logger = logging.MainLogger.WithField(logging.ServiceField, utils.GoCallerPackageName(1)) // Initially set to main logger, can be overridden by config

func logAPIRequest(req *http.Request) {
	var body string

	// Only log body if debug level is enabled and it's not a file upload
	if logger.ConsoleLevel >= logging.DebugLevel ||
		logger.FileLevel >= logging.DebugLevel {
		// Skip body logging for file uploads (multipart/form-data)
		contentType := req.Header.Get("Content-Type")
		isFileUpload := contentType != "" && strings.HasPrefix(contentType, "multipart/form-data")

		// Peek at the body without consuming it (only if not a file upload)
		if req.Body != nil && !isFileUpload {
			// Create a tee reader to read the body while preserving it
			var buf bytes.Buffer
			tee := io.TeeReader(req.Body, &buf)

			// Read a limited amount of data for logging
			peekBytes := make([]byte, 1024)
			n, _ := tee.Read(peekBytes)
			if n > 0 {
				body = string(peekBytes[:n])
				if n == 1024 {
					body += "... (truncated)"
				}
			}

			// Restore the body for other processors
			req.Body = io.NopCloser(io.MultiReader(&buf, req.Body))
		}
	}

	fields := []zap.Field{
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
		zap.String("remote", req.RemoteAddr),
	}

	if body != "" {
		fields = append(fields, zap.String("body", body))
	}

	if logger.ConsoleLevel >= logging.DebugLevel {
		method := color.MagentaString(req.Method)
		uri := color.CyanString(req.URL.RequestURI())
		from := req.RemoteAddr

		if logger.ConsoleLevel >= logging.DebugLevel {
			msg := fmt.Sprintf("API request:  %s %s from %s", method, uri, from)
			if body != "" {
				msg += " body: " + body
			} else {
				msg += " body: (skipped, multipart/form-data)"
			}
			logger.ConsoleLogger.Debug(msg)
		} else {
			msg := fmt.Sprintf("API request:  %s %s from %s", method, uri, from)
			logger.ConsoleLogger.Debug(msg)
		}
	}
	if logger.FileLevel >= logging.InfoLevel {
		if logger.FileLevel >= logging.TraceLevel {
			logger.FileLogger.With(fields...).Debug("API request")
		} else {
			logger.FileLogger.With(fields...).Info("API request")
		}
	}
}

// API response logs
func logAPIResponse(req *http.Request, w http.ResponseWriter, duration float64) {
	var prettyJSON bytes.Buffer

	fields := []zap.Field{
		zap.Int("status", w.(*responseRecorder).Status()),
		zap.Float64("duration", duration),
	}
	if logger.FileLevel >= logging.DebugLevel {
		if recorder, ok := w.(*responseRecorder); ok {
			err := json.Indent(&prettyJSON, recorder.body.Bytes(), "", "  ")
			if err != nil {
				prettyJSON.Write(recorder.body.Bytes())
			}
			fields = append(fields, zap.String("body", prettyJSON.String()))
		} else {
			logger.Error("Failed to cast response writer to responseRecorder")
			return
		}
	}
	if logger.ConsoleLevel >= logging.InfoLevel {
		method := color.MagentaString(req.Method)
		uri := color.CyanString(req.URL.RequestURI())
		from := req.RemoteAddr
		statusCode := w.(*responseRecorder).Status()
		statusStr := strconv.Itoa(statusCode)
		sizeStr := color.BlueString(fmt.Sprintf("%6dB", w.(*responseRecorder).BytesWritten()))
		durationStr := color.GreenString(fmt.Sprintf("%.6fs", duration))
		if statusCode >= 200 && statusCode < 300 {
			statusStr = color.GreenString(statusStr)
		} else if statusCode >= 300 && statusCode < 400 {
			statusStr = color.YellowString(statusStr)
		} else {
			statusStr = color.RedString(statusStr)
		}
		msg := fmt.Sprintf("API response: %s %s from %s - %3s %s in %s", method, uri, from, statusStr, sizeStr, durationStr)
		if logger.ConsoleLevel >= logging.DebugLevel {
			if prettyJSON.Len() > 0 {
				msg += " body: " + prettyJSON.String()
			}
			logger.ConsoleLogger.Debug(msg)
		} else {
			logger.ConsoleLogger.Info(msg)
		}
	}
	if logger.FileLevel >= logging.InfoLevel {
		if logger.FileLevel >= logging.DebugLevel {
			logger.FileLogger.With(fields...).Debug("API response")
		} else {
			logger.FileLogger.With(fields...).Info("API response")
		}
	}
}
