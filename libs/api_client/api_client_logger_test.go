package api_client

import (
	"context"
	"testing"

	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/stretchr/testify/assert"
)

func TestLogUpstreamRequest(t *testing.T) {
	logger.SetConsoleLogLevel(logging.DebugLevel)
	logger.SetLastMessagesLimit(10)

	// Create a test API client
	client := NewBaseAPIClient("test-api", "http://127.0.0.1:9821/test-endpoint", 10, 10, false, false)

	t.Run("Log request with JSON body", func(t *testing.T) {
		logger.SetConsoleLogLevel(logging.TraceLevel)
		logger.SetLastMessagesLimit(10)
		// Create a test request with a JSON body
		jsonBody := `{"key": "value", "nested": {"data": true}}`

		resp, err := client.Request(context.Background(), "POST", "/test1", "", []byte(jsonBody), true)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, logger.GetLastMessages(), "http://127.0.0.1:9821/test-endpoint/test1 - body: ")
	})

	t.Run("Log request with empty body", func(t *testing.T) {
		// Create a test request with a JSON body
		jsonBody := ``

		resp, err := client.Request(context.Background(), "POST", "/test2", "", []byte(jsonBody), true)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, logger.GetLastMessages(), "http://127.0.0.1:9821/test-endpoint/test2")
	})
}
