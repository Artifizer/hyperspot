package utils

import (
	"bytes"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMultipartFile implements multipart.File interface for testing
type mockMultipartFile struct {
	*bytes.Reader
}

func (m *mockMultipartFile) Close() error {
	return nil
}

// createTestMultipartFile creates a mock multipart.File with the given content
func createTestMultipartFile(content string) multipart.File {
	return &mockMultipartFile{Reader: bytes.NewReader([]byte(content))}
}

func TestSaveUploadedTempFile(t *testing.T) {
	// Test case 1: Save file to system default temp directory
	t.Run("SaveToDefaultTempDir", func(t *testing.T) {
		// Create test data
		testContent := "test file content"
		testFile := createTestMultipartFile(testContent)
		testFilename := "test_file.txt"

		// Call the function
		tempPath, err := SaveUploadedTempFile(testFile, "", testFilename)
		
		// Assertions
		require.NoError(t, err)
		defer os.Remove(tempPath) // Clean up
		
		// Verify file was created
		assert.FileExists(t, tempPath)
		
		// Verify file extension
		assert.Equal(t, ".txt", filepath.Ext(tempPath))
		
		// Verify file name pattern
		assert.True(t, strings.Contains(filepath.Base(tempPath), "hyperspot_upload_"))
		
		// Verify file content
		content, err := os.ReadFile(tempPath)
		require.NoError(t, err)
		assert.Equal(t, testContent, string(content))
	})

	// Test case 2: Save file to custom temp directory
	t.Run("SaveToCustomTempDir", func(t *testing.T) {
		// Create a custom temp directory
		customTempDir, err := os.MkdirTemp("", "hyperspot_test_*")
		require.NoError(t, err)
		defer os.RemoveAll(customTempDir) // Clean up
		
		// Create test data
		testContent := "custom temp dir test content"
		testFile := createTestMultipartFile(testContent)
		testFilename := "custom_test.pdf"
		
		// Call the function
		tempPath, err := SaveUploadedTempFile(testFile, customTempDir, testFilename)
		
		// Assertions
		require.NoError(t, err)
		
		// Verify file was created in the custom directory
		assert.FileExists(t, tempPath)
		assert.Equal(t, customTempDir, filepath.Dir(tempPath))
		
		// Verify file extension
		assert.Equal(t, ".pdf", filepath.Ext(tempPath))
		
		// Verify file content
		content, err := os.ReadFile(tempPath)
		require.NoError(t, err)
		assert.Equal(t, testContent, string(content))
	})

	// Test case 3: Handle file with no extension
	t.Run("FileWithNoExtension", func(t *testing.T) {
		testContent := "no extension file"
		testFile := createTestMultipartFile(testContent)
		testFilename := "no_extension_file"
		
		tempPath, err := SaveUploadedTempFile(testFile, "", testFilename)
		require.NoError(t, err)
		defer os.Remove(tempPath)
		
		assert.FileExists(t, tempPath)
		assert.Equal(t, "", filepath.Ext(tempPath))
		
		content, err := os.ReadFile(tempPath)
		require.NoError(t, err)
		assert.Equal(t, testContent, string(content))
	})

	// Test case 4: Handle error when creating temp file
	t.Run("ErrorCreatingTempFile", func(t *testing.T) {
		testFile := createTestMultipartFile("error test")
		testFilename := "test.txt"
		
		// Use a non-existent directory to force an error
		nonExistentDir := "/non/existent/directory"
		
		_, err := SaveUploadedTempFile(testFile, nonExistentDir, testFilename)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create temporary file")
	})

	// Test case 5: Handle error when copying file content
	t.Run("ErrorCopyingContent", func(t *testing.T) {
		// Create a mock multipart file that fails on read
		testFile := &mockFailingMultipartFile{}
		testFilename := "failing.txt"
		
		_, err := SaveUploadedTempFile(testFile, "", testFilename)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to copy file content")
	})
}

// mockFailingMultipartFile implements multipart.File interface but fails on read operations
type mockFailingMultipartFile struct{}

func (m *mockFailingMultipartFile) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (m *mockFailingMultipartFile) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (m *mockFailingMultipartFile) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (m *mockFailingMultipartFile) Close() error {
	return nil
}
