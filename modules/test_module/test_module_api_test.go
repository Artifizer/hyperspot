package test_module

import (
	"context"
	"crypto/tls"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitHandler_Success(t *testing.T) {
	// Test successful wait with valid duration
	input := &struct {
		Seconds int `query:"seconds"`
	}{
		Seconds: 1,
	}

	ctx := context.Background()
	response, err := waitHandler(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Contains(t, response.Body.Message, "Waited for 1 seconds")
}

func TestWaitHandler_InvalidSeconds(t *testing.T) {
	// Test with zero seconds
	input := &struct {
		Seconds int `query:"seconds"`
	}{
		Seconds: 0,
	}

	ctx := context.Background()
	response, err := waitHandler(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "Invalid seconds value")
}

func TestWaitHandler_NegativeSeconds(t *testing.T) {
	// Test with negative seconds
	input := &struct {
		Seconds int `query:"seconds"`
	}{
		Seconds: -5,
	}

	ctx := context.Background()
	response, err := waitHandler(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "Invalid seconds value")
}

func TestWaitHandler_ContextCancellation(t *testing.T) {
	// Test context cancellation during wait
	input := &struct {
		Seconds int `query:"seconds"`
	}{
		Seconds: 10, // Long wait
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	response, err := waitHandler(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "Request context error")
}

func TestWaitHandler_ContextTimeout(t *testing.T) {
	// Test context timeout during wait
	input := &struct {
		Seconds int `query:"seconds"`
	}{
		Seconds: 10, // Long wait
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	response, err := waitHandler(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "Request canceled")
}

func TestWaitStreamHandler_Success(t *testing.T) {
	// Test successful stream wait
	input := &struct {
		Body    map[string]interface{} `query:"body"`
		Seconds int                    `query:"seconds"`
	}{
		Body:    map[string]interface{}{"test": "data"},
		Seconds: 1,
	}

	ctx := context.Background()
	response, err := waitStreamHandler(ctx, input)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotNil(t, response.Body)
}

func TestWaitStreamHandler_InvalidSeconds(t *testing.T) {
	// Test with zero seconds
	input := &struct {
		Body    map[string]interface{} `query:"body"`
		Seconds int                    `query:"seconds"`
	}{
		Body:    map[string]interface{}{},
		Seconds: 0,
	}

	ctx := context.Background()
	response, err := waitStreamHandler(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "Invalid seconds value")
}

func TestWaitStreamHandler_ContextCancellation(t *testing.T) {
	// Test context cancellation during stream wait
	input := &struct {
		Body    map[string]interface{} `query:"body"`
		Seconds int                    `query:"seconds"`
	}{
		Body:    map[string]interface{}{"test": "data"},
		Seconds: 10, // Long wait
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	response, err := waitStreamHandler(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "Request canceled")
}

func TestWaitStreamHandler_StreamResponse(t *testing.T) {
	// Test that the stream response actually writes to the response body
	input := &struct {
		Body    map[string]interface{} `query:"body"`
		Seconds int                    `query:"seconds"`
	}{
		Body:    map[string]interface{}{"test": "data"},
		Seconds: 1,
	}

	ctx := context.Background()
	response, err := waitStreamHandler(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Body)

	// Create a mock response writer to capture the stream output
	recorder := httptest.NewRecorder()

	// Create a simple mock context for testing
	mockCtx := &mockHumaContext{writer: recorder}

	// Execute the stream function
	response.Body(mockCtx)

	// Check the response
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Body.String(), "Waited for 1 seconds (stream)")
}

// Mock implementation of huma.Context for testing
type mockHumaContext struct {
	writer *httptest.ResponseRecorder
	status int
}

func (m *mockHumaContext) Operation() *huma.Operation {
	return nil
}

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
	return url.URL{Path: "/test"}
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
	return nil
}

func (m *mockHumaContext) GetMultipartForm() (*multipart.Form, error) {
	return nil, nil
}

func (m *mockHumaContext) SetReadDeadline(t time.Time) error {
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
	return "127.0.0.1"
}

func (m *mockHumaContext) Status() int {
	return m.status
}

func (m *mockHumaContext) TLS() *tls.ConnectionState {
	return nil
}

func (m *mockHumaContext) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
}

func TestRegisterTestModuleAPIRoutes(t *testing.T) {
	// Test that routes are registered correctly
	// This is a basic test - in a real scenario you might want to
	// verify the actual route registration by making requests
	t.Skip("Integration test requires full API setup")
}

// Benchmark tests
func BenchmarkWaitHandler_Success(b *testing.B) {
	input := &struct {
		Seconds int `query:"seconds"`
	}{
		Seconds: 1,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		waitHandler(ctx, input)
	}
}

func BenchmarkWaitStreamHandler_Success(b *testing.B) {
	input := &struct {
		Body    map[string]interface{} `query:"body"`
		Seconds int                    `query:"seconds"`
	}{
		Body:    map[string]interface{}{"test": "data"},
		Seconds: 1,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		waitStreamHandler(ctx, input)
	}
}
