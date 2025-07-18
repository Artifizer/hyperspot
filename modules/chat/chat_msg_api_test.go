package chat

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/modules/llm"
	"gorm.io/gorm"
)

// Mock implementation of huma.Context for testing
type mockHumaContext struct {
	writer *httptest.ResponseRecorder
	status int
}

// Required methods for huma.Context interface
func (m *mockHumaContext) Operation() *huma.Operation {
	return nil
}

// ResponseWriter returns the underlying http.ResponseWriter
func (m *mockHumaContext) ResponseWriter() http.ResponseWriter {
	return m.writer
}

func (m *mockHumaContext) Context() context.Context {
	return context.Background()
}

func (m *mockHumaContext) Method() string {
	return "POST"
}

func (m *mockHumaContext) Host() string {
	return "localhost"
}

func (m *mockHumaContext) URL() url.URL {
	return url.URL{}
}

func (m *mockHumaContext) Param(name string) string {
	return ""
}

func (m *mockHumaContext) Query(name string) string {
	return ""
}

func (m *mockHumaContext) Header(name string) string {
	return ""
}

func (m *mockHumaContext) EachHeader(cb func(name, value string)) {
	// No-op for testing
}

func (m *mockHumaContext) BodyReader() io.Reader {
	return nil // Not used in our tests
}

func (m *mockHumaContext) GetMultipartForm() (*multipart.Form, error) {
	return nil, nil
}

func (m *mockHumaContext) SetReadDeadline(t time.Time) error {
	// No-op for testing
	return nil
}

func (m *mockHumaContext) SetStatus(code int) {
	m.writer.WriteHeader(code)
	m.status = code
}

func (m *mockHumaContext) SetHeader(name, value string) {
	m.writer.Header().Set(name, value)
}

func (m *mockHumaContext) AppendHeader(name, value string) {
	m.writer.Header().Add(name, value)
}

func (m *mockHumaContext) BodyWriter() io.Writer {
	return m.writer
}

func (m *mockHumaContext) RemoteAddr() string {
	// Return a dummy IP or empty string for testing
	return "127.0.0.1"
}

func (m *mockHumaContext) Status() int {
	// Return a fixed or stored status code for testing
	return m.status
}

func (m *mockHumaContext) TLS() *tls.ConnectionState {
	// Return nil or a dummy tls.ConnectionState for testing
	return nil
}

func (m *mockHumaContext) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
}

// setupTestDBForChatAPI initializes an in-memory SQLite DB and migrates the schema for chat API tests.
func setupTestDBForChatAPI(t *testing.T) *gorm.DB {
	testDB, err := db.InitInMemorySQLite(nil)
	if err != nil {
		t.Fatalf("Failed to connect to test DB: %v", err)
	}
	db.SetDB(testDB)
	if err := db.SafeAutoMigrate(testDB, &ChatMessage{}, &ChatThread{}, &ChatThreadGroup{}, &SystemPrompt{}, &ChatThreadSystemPrompt{}); err != nil {
		t.Fatalf("Failed to auto migrate schema: %v", err)
	}
	return testDB
}

// TestChatMessageAPIThreadIDConsistency_ExistingThread_NonStreaming verifies that the thread ID is the same for both
// the user message and assistant message when using an existing thread in non-streaming mode
func TestChatMessageAPIThreadIDConsistency_ExistingThread_NonStreaming(t *testing.T) {
	// Setup test database
	setupTestDBForChatAPI(t)

	// Register mock LLM service for testing
	ctx := context.Background()
	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread for testing
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "Failed to create test thread")
	require.NotNil(t, thread)

	// Test with existing thread in non-streaming mode
	testChatMessageAPIThreadIDConsistencyWithExistingThread(t, thread.ID, false)
}

// TestChatMessageAPIThreadIDConsistency_ExistingThread_Streaming verifies that the thread ID is the same for both
// the user message and assistant message when using an existing thread in streaming mode
func TestChatMessageAPIThreadIDConsistency_ExistingThread_Streaming(t *testing.T) {
	// Setup test database
	setupTestDBForChatAPI(t)

	// Register mock LLM service for testing
	ctx := context.Background()
	llm.RegisterMockServiceForUnitTests(ctx)

	// Create a thread for testing
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx, "Failed to create test thread")
	require.NotNil(t, thread)

	// Test with existing thread in streaming mode
	testChatMessageAPIThreadIDConsistencyWithExistingThread(t, thread.ID, true)
}

// TestChatMessageAPIThreadIDConsistency_NewThread_NonStreaming verifies that the thread ID is the same for both
// the user message and assistant message when creating a new thread in non-streaming mode
func TestChatMessageAPIThreadIDConsistency_NewThread_NonStreaming(t *testing.T) {
	// Setup test database
	setupTestDBForChatAPI(t)

	// Register mock LLM service for testing
	ctx := context.Background()
	llm.RegisterMockServiceForUnitTests(ctx)

	// Test with new thread (nil thread ID) in non-streaming mode
	testChatMessageAPIThreadIDConsistencyWithNewThread(t, false)
}

// TestChatMessageAPIThreadIDConsistency_NewThread_Streaming verifies that the thread ID is the same for both
// the user message and assistant message when creating a new thread in streaming mode
func TestChatMessageAPIThreadIDConsistency_NewThread_Streaming(t *testing.T) {
	// Setup test database
	setupTestDBForChatAPI(t)

	// Register mock LLM service for testing
	ctx := context.Background()
	llm.RegisterMockServiceForUnitTests(ctx)

	// Test with new thread (nil thread ID) in streaming mode
	testChatMessageAPIThreadIDConsistencyWithNewThread(t, true)
}

// Helper function to test thread ID consistency with an existing thread
func testChatMessageAPIThreadIDConsistencyWithExistingThread(t *testing.T, threadID uuid.UUID, streaming bool) {
	ctx := context.Background()

	// Create a request body
	input := &ChatMessageAPIRequestBody{
		ModelName:   "mock-model",
		Temperature: 0.5,
		MaxTokens:   100,
		Content:     "Test message",
		Stream:      streaming,
	}

	// Call the API function directly
	response, err := ChatMessageAPIStream(ctx, uuid.Nil, threadID, input)

	// Verify no error occurred
	assert.Nil(t, err, "ChatMessageAPIStream should not return an error")

	// For streaming response, we need to capture the output
	if streaming {
		// Create a test recorder to capture the response
		rec := httptest.NewRecorder()

		// No need for chi router in unit tests

		// Create a mock huma context with the recorder
		mockCtx := &mockHumaContext{
			writer: rec,
		}

		// Execute the streaming function
		response.Body(mockCtx)

		// Parse the response to extract messages
		responseBody := rec.Body.String()

		// The first data line contains the initial response with both messages
		initialDataLine := strings.Split(responseBody, "\n")[0]
		initialData := strings.TrimPrefix(initialDataLine, "data: ")

		// Parse the initial data to get the messages
		var initialResponse struct {
			PageNumber   int           `json:"page_number"`
			PageSize     int           `json:"page_size"`
			Total        int           `json:"total"`
			ChatMessages []ChatMessage `json:"messages"`
		}
		err = json.Unmarshal([]byte(initialData), &initialResponse)
		require.NoError(t, err, "Failed to unmarshal initial response")

		// Verify we have exactly 2 messages (user and assistant)
		require.Equal(t, 2, len(initialResponse.ChatMessages), "Should have exactly 2 messages")

		// Verify both messages have the same thread ID
		userMsg := initialResponse.ChatMessages[0]
		assistantMsg := initialResponse.ChatMessages[1]

		assert.Equal(t, userMsg.ThreadID, assistantMsg.ThreadID, "User and assistant messages should have the same thread ID")
		assert.Equal(t, threadID, userMsg.ThreadID, "User message should have the specified thread ID")
	} else {
		// For non-streaming, extract the response data directly
		rec := httptest.NewRecorder()

		// Create a mock huma.Context
		mockCtx := &mockHumaContext{
			writer: rec,
		}

		// Execute the response function
		response.Body(mockCtx)

		// Parse the response to extract messages
		var responseData struct {
			PageNumber   int           `json:"page_number"`
			PageSize     int           `json:"page_size"`
			Total        int           `json:"total"`
			ChatMessages []ChatMessage `json:"messages"`
		}
		err = json.Unmarshal(rec.Body.Bytes(), &responseData)
		require.NoError(t, err, "Failed to unmarshal response")

		// Verify we have exactly 2 messages (user and assistant)
		require.Equal(t, 2, len(responseData.ChatMessages), "Should have exactly 2 messages")

		// Verify both messages have the same thread ID
		userMsg := responseData.ChatMessages[0]
		assistantMsg := responseData.ChatMessages[1]

		assert.Equal(t, userMsg.ThreadID, assistantMsg.ThreadID, "User and assistant messages should have the same thread ID")
		assert.Equal(t, threadID, userMsg.ThreadID, "User message should have the specified thread ID")
	}
}

// Helper function to test thread ID consistency with a new thread (nil thread ID)
func testChatMessageAPIThreadIDConsistencyWithNewThread(t *testing.T, streaming bool) {
	ctx := context.Background()

	// Create a request body
	input := &ChatMessageAPIRequestBody{
		ModelName:   "mock-model",
		Temperature: 0.5,
		MaxTokens:   100,
		Content:     "Test message for new thread",
		Stream:      streaming,
	}

	// Call the API function directly with nil thread ID to create a new thread
	response, err := ChatMessageAPIStream(ctx, uuid.Nil, uuid.Nil, input)

	// Verify no error occurred
	assert.Nil(t, err, "ChatMessageAPIStream should not return an error")

	// For streaming response, we need to capture the output
	if streaming {
		// Create a test recorder to capture the response
		rec := httptest.NewRecorder()

		// No need for chi router in unit tests

		// Create a mock huma context with the recorder
		mockCtx := &mockHumaContext{
			writer: rec,
		}

		// Execute the streaming function
		response.Body(mockCtx)

		// Parse the response to extract messages
		responseBody := rec.Body.String()

		// The first data line contains the initial response with both messages
		initialDataLine := strings.Split(responseBody, "\n")[0]
		initialData := strings.TrimPrefix(initialDataLine, "data: ")

		// Parse the initial data to get the messages
		var initialResponse struct {
			PageNumber   int           `json:"page_number"`
			PageSize     int           `json:"page_size"`
			Total        int           `json:"total"`
			ChatMessages []ChatMessage `json:"messages"`
		}
		err = json.Unmarshal([]byte(initialData), &initialResponse)
		require.NoError(t, err, "Failed to unmarshal initial response")

		// Verify we have exactly 2 messages (user and assistant)
		require.Equal(t, 2, len(initialResponse.ChatMessages), "Should have exactly 2 messages")

		// Verify both messages have the same thread ID
		userMsg := initialResponse.ChatMessages[0]
		assistantMsg := initialResponse.ChatMessages[1]

		assert.Equal(t, userMsg.ThreadID, assistantMsg.ThreadID, "User and assistant messages should have the same thread ID")
		assert.NotEqual(t, uuid.Nil, userMsg.ThreadID, "User message should have a valid thread ID")
	} else {
		// For non-streaming, extract the response data directly
		rec := httptest.NewRecorder()

		// Create a mock huma.Context
		mockCtx := &mockHumaContext{
			writer: rec,
		}

		// Execute the response function
		response.Body(mockCtx)

		// Parse the response to extract messages
		var responseData struct {
			PageNumber   int           `json:"page_number"`
			PageSize     int           `json:"page_size"`
			Total        int           `json:"total"`
			ChatMessages []ChatMessage `json:"messages"`
		}
		err = json.Unmarshal(rec.Body.Bytes(), &responseData)
		require.NoError(t, err, "Failed to unmarshal response")

		// Verify we have exactly 2 messages (user and assistant)
		require.Equal(t, 2, len(responseData.ChatMessages), "Should have exactly 2 messages")

		// Verify both messages have the same thread ID
		userMsg := responseData.ChatMessages[0]
		assistantMsg := responseData.ChatMessages[1]

		assert.Equal(t, userMsg.ThreadID, assistantMsg.ThreadID, "User and assistant messages should have the same thread ID")
		assert.NotEqual(t, uuid.Nil, userMsg.ThreadID, "User message should have a valid thread ID")
	}
}
