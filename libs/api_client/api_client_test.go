package api_client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
	// Setup test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}
	}))
	defer server.Close()

	// Create client with short timeout for testing
	client := NewBaseAPIClient("test-client", server.URL, 1, 1, false, false)

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
	response := http.StatusOK
	responsePtr := &response

	// Find an available port by creating a listener; do not close it so that the server can use it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find an available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Create server with the specific port
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(*responsePtr)
			w.Write([]byte(`{"status":"ok"}`))
		case "/success":
			w.WriteHeader(*responsePtr)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))

	// Configure the server to use our specific port
	server.Listener = listener
	server.Start()

	time.Sleep(200 * time.Millisecond)

	client := NewBaseAPIClient("test-client", fmt.Sprintf("http://127.0.0.1:%d", port), 1, 1, false, true)
	client.SetRetryTimeout(2 * time.Second)

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

	response = http.StatusInternalServerError

	t.Run("Failed request/response", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.NotNil(t, resp)
		assert.Error(t, err)
		assert.Equal(t, http.StatusInternalServerError, resp.UpstreamResponse.StatusCode)
		assert.Equal(t, false, client.LikelyIsAlive())
	})

	response = http.StatusOK

	t.Run("Failed request/response", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.NotNil(t, resp)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
		assert.Equal(t, true, client.LikelyIsAlive())
	})

	server.Close()

	t.Run("Failed request/response", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		resp, err := client.RequestAndParse(ctx, "GET", "/success", "", nil)
		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Equal(t, false, client.LikelyIsAlive())
	})

	server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.WriteHeader(*responsePtr)
			w.Write([]byte(`{"status":"ok"}`))
		case "/success":
			w.WriteHeader(*responsePtr)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	server.Listener, _ = net.Listen("tcp", fmt.Sprintf(":%d", port))
	server.Start()

	client.StartOnlineWatchdog(context.Background())

	// Poll until the server is detected as alive or timeout
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.LikelyIsAlive() {
			break
		}
		// Make a direct request to help trigger the watchdog
		resp, err := client.Request(context.Background(), "GET", "/", "", nil, false)
		resp.Body.Close()
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}
	assert.Equal(t, true, client.LikelyIsAlive())
}
