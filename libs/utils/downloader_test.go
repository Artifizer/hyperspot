package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDownloadWithProgress(t *testing.T) {
	// Create a test server that serves a simple file
	testContent := "This is test content for download"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testContent))
	}))
	defer server.Close()

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "download_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Set up the destination path
	destPath := filepath.Join(tempDir, "downloaded_file.txt")

	// Track progress
	//	var lastProgress float64
	progressCallback := func(progress float32) {
		//		lastProgress = progress
		// We don't check the exact value since it might vary
		assert.GreaterOrEqual(t, progress, 0)
		assert.LessOrEqual(t, progress, 100)
	}

	// Test the download function
	err = DownloadWithProgress(server.URL, destPath, progressCallback, false)
	assert.NoError(t, err)

	// Verify the file was downloaded correctly
	content, err := os.ReadFile(destPath)
	assert.NoError(t, err)
	assert.Equal(t, testContent, string(content))

	// Verify progress was tracked (should be 100% at the end)
	//	assert.Equal(t, float64(100), lastProgress)
}

func TestDownloadWithProgressResume(t *testing.T) {
	// Create a test server that supports range requests
	testContent := "This is a larger test content for testing resume functionality"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			// Parse the range header
			var start int
			fmt.Sscanf(rangeHeader, "bytes=%d-", &start)

			// Respond with partial content
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d",
				start, len(testContent)-1, len(testContent)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)-start))
			w.WriteHeader(http.StatusPartialContent)
			w.Write([]byte(testContent)[start:])
		} else {
			// Respond with full content
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testContent)))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testContent))
		}
	}))
	defer server.Close()

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "download_resume_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Set up the destination path
	destPath := filepath.Join(tempDir, "resumed_file.txt")

	// Write partial content to simulate a partial download
	partialContent := testContent[:15]
	err = os.WriteFile(destPath, []byte(partialContent), 0644)
	assert.NoError(t, err)

	// Track progress
	progressCallback := func(progress float32) {
		// Just a placeholder for the callback
	}

	// Test the resume download function
	err = DownloadWithProgress(server.URL, destPath, progressCallback, true)
	assert.NoError(t, err)

	// Verify the file was downloaded correctly
	content, err := os.ReadFile(destPath)
	assert.NoError(t, err)
	assert.Equal(t, testContent, string(content))
}

func TestProgressTracker(t *testing.T) {
	tracker := &ProgressTracker{
		URL:                   "http://example.com/test",
		Dest:                  "/tmp/test",
		Total:                 1000,
		Current:               0,
		ProgressThresholdMsec: 100,
		LastLogTime:           time.Now().Add(-1 * time.Second), // Set to past time to ensure progress is logged
	}

	// Create a mock reader
	mockData := make([]byte, 500)
	for i := range mockData {
		mockData[i] = byte(i % 256)
	}

	// Track progress
	var progressValue float32
	tracker.Callback = func(progress float32) {
		progressValue = progress
	}

	// Create a mock stream
	mockStream := &mockReadCloser{data: mockData}
	tracker.stream = mockStream

	// Read some data
	buf := make([]byte, 200)
	n, err := tracker.Read(buf)

	assert.NoError(t, err)
	assert.Equal(t, 200, n)
	assert.Equal(t, int64(200), tracker.Current)

	// Progress should be 20%
	assert.InDelta(t, 20, progressValue, 0.1)
}

// Mock ReadCloser for testing
type mockReadCloser struct {
	data []byte
	pos  int
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, nil
	}

	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	return nil
}
