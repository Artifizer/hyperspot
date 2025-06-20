package api_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fatih/color"
	"github.com/hypernetix/hyperspot/libs/logging"
	"go.uber.org/zap"
)

var logger *logging.Logger = logging.MainLogger // Initially set to main logger, can be overridden by config

// Upstream request logs
func (c *BaseAPIClient) logUpstreamRequest(req *http.Request) {
	var prettyJSON bytes.Buffer
	var bodyStr string

	// FIXME: need to cleanup user messages (prompts, completions) content

	fields := []zap.Field{
		zap.String("target", c.GetName()),
		zap.String("method", req.Method),
		zap.String("uri", req.URL.RequestURI()),
	}

	if logger.ConsoleLevel >= logging.DebugLevel || logger.FileLevel >= logging.DebugLevel {
		// Peek at the body without consuming it
		if req.Body != nil {
			// Create a tee reader to read the body without consuming it
			var buf bytes.Buffer
			tee := io.TeeReader(req.Body, &buf)

			// Read the body into a byte slice
			body, err := io.ReadAll(tee)
			if err != nil {
				logging.Error("Error reading request body: %v", err)
				return
			}

			// Restore the body for the actual request
			req.Body = io.NopCloser(&buf)

			// Format the body
			if err := json.Indent(&prettyJSON, body, "", "  "); err == nil {
				bodyStr = prettyJSON.String()
			} else {
				bodyStr = string(body)
			}
		}
	}

	if logger.ConsoleLevel >= logging.TraceLevel {
		method := color.MagentaString(req.Method)
		if bodyStr != "" {
			msg := fmt.Sprintf("Upstream request: %s %s - body: %s", method, req.URL.String(), bodyStr)
			logger.ConsoleLogger.Trace(msg)
		} else {
			msg := fmt.Sprintf("Upstream request: %s %s", method, req.URL.String())
			logger.ConsoleLogger.Debug(msg)
		}
	}

	if logger.FileLevel >= logging.DebugLevel {
		fields = append(fields, zap.String("body", bodyStr))
		fields = append(fields, zap.String("uri", req.URL.RequestURI()))
		logger.FileLogger.With(fields...).Debug("Upstream request")
	}
}

// Upstream response logs
func (c *BaseAPIClient) logUpstreamResponse(method string, path string, statusCode int, body string, duration time.Duration) {
	var prettyJSON bytes.Buffer
	var bodyStr string

	if logger.ConsoleLevel >= logging.DebugLevel || logger.FileLevel >= logging.DebugLevel {
		// First try to unmarshal the string to JSON
		var jsonData interface{}
		if err := json.Unmarshal([]byte(body), &jsonData); err == nil {
			// If it's valid JSON, pretty print it
			if err := json.Indent(&prettyJSON, []byte(body), "", "  "); err == nil {
				bodyStr = prettyJSON.String()
			} else {
				bodyStr = body
			}
		} else {
			bodyStr = body
		}
	}

	fields := []zap.Field{
		zap.String("target", c.GetName()),
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status", statusCode),
	}

	method = color.MagentaString(method)
	msg := fmt.Sprintf("Upstream response: %s %s%s - status: %d, dur: %.3f",
		method, c.GetName(), path, statusCode, duration.Seconds())

	if logger.FileLevel >= logging.TraceLevel {
		fields = append(fields, zap.String("body", bodyStr))
		logger.ConsoleLogger.Debug(msg + " - body: " + bodyStr)
		logger.FileLogger.With(fields...).Trace("Upstream response")
	} else {
		logger.ConsoleLogger.Info(msg)
		logger.FileLogger.With(fields...).Debug("Upstream response")
	}
}
