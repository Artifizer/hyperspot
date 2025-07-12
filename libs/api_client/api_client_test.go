package api_client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	testServer     *httptest.Server
	testServerOnce sync.Once
	testServerURL  string
)

// setupTestServer creates a single test server for all tests
func setupTestServer() {
	testServerOnce.Do(func() {
		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/success":
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
			case "/bad-request":
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"bad request"}`))
			case "/timeout":
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"delayed"}`))
			case "/invalid-json":
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{invalid json}`))
			case "/":
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
			default:
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"not found"}`))
			}
		}))
		testServerURL = testServer.URL
	})
}

// TestMain handles test setup and cleanup
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	// Cleanup
	if testServer != nil {
		testServer.Close()
	}

	// Exit with the same code as the tests
	os.Exit(code)
}

func TestNewBaseAPIClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
	}{
		{
			name:    "valid client creation",
			baseURL: "https://api.example.com",
		},
		{
			name:    "empty base URL",
			baseURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewBaseAPIClient("test-client", tt.baseURL, 1, 1, false, false)
			assert.NotNil(t, client)
			assert.Equal(t, "test-client", client.name)
			assert.Equal(t, tt.baseURL, client.baseUrl)
		})
	}
}

func TestAPIError(t *testing.T) {
	err := &APIError{
		StatusCode: 404,
		Message:    "Not Found",
		RawBody:    "Resource not found",
	}

	assert.Equal(t, "API error (status 404): Not Found", err.Error())
}

func TestMakeRequest(t *testing.T) {
	// Setup shared test server
	setupTestServer()

	// Create client with short timeout for testing
	client := NewBaseAPIClient("test-client", testServerURL, 1, 1, false, false)

	// Test successful request
	t.Run("successful request", func(t *testing.T) {
		ctx := context.Background()
		resp, err := client.Request(ctx, "GET", "/success", "", nil, false)
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		var result map[string]string
		err = json.NewDecoder(resp.Body).Decode(&result)
		assert.NoError(t, err)
		assert.Equal(t, "ok", result["status"])
		resp.Body.Close()
	})

	// Test error response
	t.Run("error response", func(t *testing.T) {
		ctx := context.Background()
		resp, err := client.Request(ctx, "GET", "/bad-request", "", nil, false)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()
	})

	// Test with headers
	t.Run("with headers", func(t *testing.T) {
		ctx := context.Background()
		resp, err := client.Request(ctx, "GET", "/success", "", nil, false)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		resp.Body.Close()
	})

	// Test with context cancellation
	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		resp, err := client.Request(ctx, "GET", "/success", "", nil, false)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		assert.Error(t, err)
	})

	// Test with timeout
	t.Run("Request with invalid JSON response", func(t *testing.T) {
		ctx := context.Background()
		resp, err := client.Request(ctx, "GET", "/invalid-json", "", nil, false)
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		resp.Body.Close()

		var responseBody map[string]string
		err = json.Unmarshal(body, &responseBody)
		assert.Error(t, err)
	})

	// Test RequestAndParse
	t.Run("RequestAndParse successful", func(t *testing.T) {
		ctx := context.Background()
		var responseBody map[string]string
		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.NotNil(t, resp)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
		err = json.Unmarshal(resp.BodyBytes, &responseBody)
		assert.NoError(t, err)
		assert.Equal(t, "ok", responseBody["status"])
		assert.Equal(t, true, client.LikelyIsAlive())
	})

	t.Run("RequestAndParse with error response", func(t *testing.T) {
		ctx := context.Background()
		var responseBody map[string]string
		resp, err := client.RequestAndParse(ctx, "GET", "/bad-request", "", nil)
		assert.NotNil(t, resp)
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.UpstreamResponse.StatusCode)
		err = json.Unmarshal(resp.BodyBytes, &responseBody)
		assert.NoError(t, err)
		assert.Equal(t, "bad request", responseBody["error"])
	})

	t.Run("RequestAndParse with timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		resp, err := client.RequestAndParse(ctx, "GET", "/timeout", "", nil)
		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("RequestAndParse with context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.Nil(t, resp)
		assert.Error(t, err)
	})

	t.Run("RequestAndParse with invalid JSON response", func(t *testing.T) {
		// Create a new test server that returns invalid JSON
		ctx := context.Background()
		resp, err := client.RequestAndParse(ctx, "GET", "/invalid-json", "", nil)
		assert.NotNil(t, resp)
		assert.Error(t, err)
		assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
		assert.Nil(t, resp.BodyBytes)
	})
}

func TestDeadServer(t *testing.T) {
	client := NewBaseAPIClient("test-client", "http://127.0.0.1:9999/non-existing-route", 1, 1, false, false)

	t.Run("RequestAndParse with invalid JSON response", func(t *testing.T) {
		// Create a new test server that returns invalid JSON
		ctx := context.Background()
		resp, err := client.RequestAndParse(ctx, "GET", "/non-existing-domain", "", nil)
		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Equal(t, false, client.LikelyIsAlive())
	})
}

func TestStartOnlineWatchdog(t *testing.T) {
	// Setup shared test server
	setupTestServer()

	// Create client with watchdog enabled
	client := NewBaseAPIClient("test-client", testServerURL, 1, 1, false, true)
	client.SetRetryTimeout(0)

	t.Run("Successful request/response", func(t *testing.T) {
		ctx := context.Background()
		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		if resp != nil {
			assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
			assert.NotNil(t, resp.BodyBytes)
		}
		assert.Equal(t, true, client.LikelyIsAlive())
	})

	t.Run("Bad request response", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		resp, err := client.RequestAndParse(ctx, "GET", "/bad-request", "", nil)
		assert.NotNil(t, resp)
		assert.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.UpstreamResponse.StatusCode)
		// Bad request doesn't mark server as dead, only connection errors do
		assert.Equal(t, true, client.LikelyIsAlive())
	})

	t.Run("Successful request after bad request", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.NotNil(t, resp)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
		assert.Equal(t, true, client.LikelyIsAlive())
	})

	t.Run("Test watchdog functionality", func(t *testing.T) {
		// Start the watchdog
		client.StartOnlineWatchdog(context.Background())

		// Give the watchdog a moment to start
		time.Sleep(100 * time.Millisecond)

		// Server should be detected as alive
		assert.Equal(t, true, client.LikelyIsAlive())

		// Make a request to ensure watchdog is working
		ctx := context.Background()
		resp, err := client.Request(ctx, "GET", "/", "", nil, false)
		assert.NoError(t, err)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}

		// Server should still be alive
		assert.Equal(t, true, client.LikelyIsAlive())
	})
}
