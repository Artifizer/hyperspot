package openapi_client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAICompletionRequest_ToMap(t *testing.T) {
	req := OpenAICompletionRequest{
		ModelName: "gpt-4",
		Messages: []OpenAICompletionMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: 0.7,
		MaxTokens:   100,
		Stream:      false,
	}

	result := req.ToMap()

	assert.Equal(t, "gpt-4", result["model"])
	assert.Equal(t, req.Messages, result["messages"])
	assert.Equal(t, 0.7, result["temperature"])
	assert.Equal(t, 100, result["max_tokens"])
	assert.Equal(t, false, result["stream"])
}

func TestNewOpenAIAPIClient(t *testing.T) {
	// Test with API key in environment
	t.Run("With API key", func(t *testing.T) {
		envVar := "TEST_OPENAI_API_KEY"
		os.Setenv(envVar, "test-api-key")
		defer os.Unsetenv(envVar)

		client := NewOpenAIAPIClient("test-client", "https://api.openai.com", "", envVar, 10, 10, true, false)

		assert.Equal(t, "https://api.openai.com", client.GetName())
		assert.Equal(t, "https://api.openai.com", client.GetBaseURL())
		assert.Equal(t, "test-api-key", client.apiKey)
	})

	// Test without API key in environment
	t.Run("Without API key", func(t *testing.T) {
		envVar := "NONEXISTENT_API_KEY"
		client := NewOpenAIAPIClient("test-client", "https://api.openai.com", "", envVar, 10, 10, true, false)

		assert.Equal(t, "api-key-is-not-set", client.apiKey)
	})

	// Test with direct API key but no environment variable
	t.Run("With direct API key", func(t *testing.T) {
		// Ensure environment variable is not set
		envVar := "NONEXISTENT_API_KEY"
		directKey := "direct-api-key"

		client := NewOpenAIAPIClient("test-client", "https://api.openai.com", directKey, envVar, 10, 10, true, false)

		// Should use the direct API key
		assert.Equal(t, directKey, client.apiKey)
	})

	// Test with both environment variable and direct API key
	t.Run("With both env and direct API keys", func(t *testing.T) {
		envVar := "TEST_OPENAI_API_KEY"
		os.Setenv(envVar, "env-api-key")
		defer os.Unsetenv(envVar)
		directKey := "direct-api-key"

		client := NewOpenAIAPIClient("test-client", "https://api.openai.com", directKey, envVar, 10, 10, true, false)

		// Environment variable should take precedence
		assert.Equal(t, "env-api-key", client.apiKey)
		assert.NotEqual(t, directKey, client.apiKey)
	})
}

func TestOpenAIAPIClient_Methods(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		// Return different responses based on the path
		switch r.URL.Path {
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data": [{"id": "gpt-4"}]}`))
		case "/v1/models/gpt-4":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id": "gpt-4", "owned_by": "openai"}`))
		case "/v1/chat/completions":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"model": "gpt-4",
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "Hello there!"
						}
					}
				],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 20,
					"total_tokens": 30
				}
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create client with test server URL
	client := NewOpenAIAPIClient("test-client", server.URL, "", "TEST_API_KEY", 10, 10, true, false)
	client.apiKey = "test-api-key" // Set directly for testing

	ctx := context.Background()

	// Test ListModels
	t.Run("ListModels", func(t *testing.T) {
		resp, err := client.ListModels(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		var data map[string]interface{}
		err = json.Unmarshal(resp.BodyBytes, &data)
		assert.NoError(t, err)
		assert.Contains(t, data, "data")
	})

	// Test GetModel
	t.Run("GetModel", func(t *testing.T) {
		resp, err := client.GetModel(ctx, "gpt-4")
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		var data map[string]interface{}
		err = json.Unmarshal(resp.BodyBytes, &data)
		assert.NoError(t, err)
		assert.Equal(t, "gpt-4", data["id"])
	})

	// Test ChatCompletionsRaw
	t.Run("ChatCompletionsRaw", func(t *testing.T) {
		req := OpenAICompletionRequestRaw{
			"model": "gpt-4",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}

		resp, err := client.ChatCompletionsRaw(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)

		var data map[string]interface{}
		err = json.Unmarshal(resp.BodyBytes, &data)
		assert.NoError(t, err)
		assert.Equal(t, "gpt-4", data["model"])
	})

	// Test Ping
	t.Run("Ping", func(t *testing.T) {
		err := client.Ping(ctx)
		assert.NoError(t, err)
	})
}

func TestOpenAIAPIClient_RequestMethods(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewOpenAIAPIClient("test-client", server.URL, "", "TEST_API_KEY", 10, 10, true, false)
	client.apiKey = "test-api-key" // Set directly for testing

	ctx := context.Background()

	// Test Request
	t.Run("Request", func(t *testing.T) {
		resp, err := client.Request(ctx, "GET", "/test", []byte{})
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.NotNil(t, resp)
	})

	// Test GetWithAPIKey
	t.Run("GetWithAPIKey", func(t *testing.T) {
		resp, err := client.GetWithAPIKey(ctx, "/test")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
	})

	// Test PostWithAPIKey
	t.Run("PostWithAPIKey", func(t *testing.T) {
		body := []byte(`{"test": true}`)
		resp, err := client.PostWithAPIKey(ctx, "/test", body)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.UpstreamResponse.StatusCode)
	})
}
