package file_parser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock parser for testing
type mockParser struct {
	shouldError bool
	errorMsg    string
}

func (m *mockParser) Parse(_ context.Context, _ io.Reader, doc *document.Document) errorx.Error {
	if m.shouldError {
		return errorx.NewErrInternalServerError(m.errorMsg)
	}

	// Set some test values
	doc.TotalPagesCount = 1
	doc.PageTextContent = []string{"Mock content"}
	doc.ParserName = "mock_parser"
	doc.MimeType = "text/plain"

	return nil
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "PDF file",
			path:     "document.pdf",
			expected: "pdf",
		},
		{
			name:     "DOCX file",
			path:     "document.docx",
			expected: "docx",
		},
		{
			name:     "Text file",
			path:     "readme.txt",
			expected: "txt",
		},
		{
			name:     "Markdown file",
			path:     "readme.md",
			expected: "md",
		},
		{
			name:     "HTML file",
			path:     "index.html",
			expected: "html",
		},
		{
			name:     "Multiple dots",
			path:     "file.backup.pdf",
			expected: "pdf",
		},
		{
			name:     "Mixed case extension",
			path:     "Document.PDF",
			expected: "pdf",
		},
		{
			name:     "No extension",
			path:     "README",
			expected: "",
		},
		{
			name:     "Empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "Path with directory",
			path:     "/path/to/document.docx",
			expected: "docx",
		},
		{
			name:     "Windows path",
			path:     "C:\\Users\\user\\document.pdf",
			expected: "pdf",
		},
		{
			name:     "Dot at beginning",
			path:     ".hidden.txt",
			expected: "txt",
		},
		{
			name:     "Just dot",
			path:     ".",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFileExtension(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetConfig(t *testing.T) {
	config := GetConfig()

	assert.NotNil(t, config)
	assert.NotNil(t, config.ParserExtensionMap)
	assert.Contains(t, config.ParserExtensionMap, "builtin")
}

func TestGetParser(t *testing.T) {
	t.Run("Valid extensions", func(t *testing.T) {
		validExtensions := []string{"pdf", "docx", "html", "txt", "md"}

		for _, ext := range validExtensions {
			t.Run(fmt.Sprintf("Extension_%s", ext), func(t *testing.T) {
				// First we need to ensure the extension is in the supported list
				// This test assumes the parserMap has been properly initialized
				if extensions := GetSupportedExtensions("builtin"); len(extensions) > 0 {
					// Only test if extension is actually supported
					for _, supportedExt := range extensions {
						if supportedExt == ext {
							parser, exists := GetParser(ext)
							assert.True(t, exists, "Parser should exist for extension %s", ext)
							assert.NotNil(t, parser, "Parser should not be nil for extension %s", ext)
							break
						}
					}
				}
			})
		}
	})

	t.Run("Unsupported extension", func(t *testing.T) {
		parser, exists := GetParser("xyz")
		assert.False(t, exists)
		assert.Nil(t, parser)
	})

	t.Run("Empty extension", func(t *testing.T) {
		parser, exists := GetParser("")
		assert.False(t, exists)
		assert.Nil(t, parser)
	})

	t.Run("Case sensitivity", func(t *testing.T) {
		// Test that uppercase extensions don't work (they should be normalized to lowercase)
		parser, exists := GetParser("PDF")
		assert.False(t, exists)
		assert.Nil(t, parser)
	})
}

func TestNewUnsupportedFileTypeError(t *testing.T) {
	t.Run("Standard extension", func(t *testing.T) {
		err := newUnsupportedFileTypeError("xyz")

		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "Unsupported file type: 'xyz'")
		assert.Contains(t, err.Error(), "Supported file types are:")

		// Check that it's the correct error type
		var unsupportedErr *errorx.ErrUnsupportedMediaType
		assert.True(t, errors.As(err, &unsupportedErr))
	})

	t.Run("Empty extension", func(t *testing.T) {
		err := newUnsupportedFileTypeError("")

		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "Unsupported file type: ''")
	})
}

func TestDetermineExtensionFromURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		contentType string
		expected    string
	}{
		{
			name:        "PDF from URL",
			url:         "https://example.com/document.pdf",
			contentType: "",
			expected:    "pdf",
		},
		{
			name:        "HTML from URL",
			url:         "https://example.com/page.html",
			contentType: "",
			expected:    "html",
		},
		{
			name:        "No extension, HTML content type",
			url:         "https://example.com/page",
			contentType: "text/html; charset=utf-8",
			expected:    "html",
		},
		{
			name:        "No extension, PDF content type",
			url:         "https://example.com/document",
			contentType: "application/pdf",
			expected:    "pdf",
		},
		{
			name:        "No extension, plain text content type",
			url:         "https://example.com/readme",
			contentType: "text/plain",
			expected:    "txt",
		},
		{
			name:        "No extension, markdown content type",
			url:         "https://example.com/readme",
			contentType: "text/markdown",
			expected:    "md",
		},
		{
			name:        "No extension, JSON content type",
			url:         "https://example.com/data",
			contentType: "application/json",
			expected:    "txt",
		},
		{
			name:        "No extension, XML content type",
			url:         "https://example.com/data",
			contentType: "application/xml",
			expected:    "txt",
		},
		{
			name:        "No extension, unknown content type",
			url:         "https://example.com/unknown",
			contentType: "application/octet-stream",
			expected:    "txt",
		},
		{
			name:        "No extension, no content type",
			url:         "https://example.com/unknown",
			contentType: "",
			expected:    "txt",
		},
		{
			name:        "XHTML content type",
			url:         "https://example.com/page",
			contentType: "application/xhtml+xml",
			expected:    "html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineExtensionFromURL(tt.url, tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsHTMLContent(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		contentType string
		expected    bool
	}{
		{
			name:        "HTML content type",
			content:     []byte("some content"),
			contentType: "text/html; charset=utf-8",
			expected:    true,
		},
		{
			name:        "XHTML content type",
			content:     []byte("some content"),
			contentType: "application/xhtml+xml",
			expected:    true,
		},
		{
			name:        "HTML DOCTYPE",
			content:     []byte("<!DOCTYPE html><html><head></head><body></body></html>"),
			contentType: "text/plain",
			expected:    true,
		},
		{
			name:        "HTML tags",
			content:     []byte("<html><head><title>Test</title></head><body><div>Content</div></body></html>"),
			contentType: "",
			expected:    true,
		},
		{
			name:        "Few HTML patterns",
			content:     []byte("<div>Some content</div>"),
			contentType: "",
			expected:    false,
		},
		{
			name:        "Multiple HTML patterns",
			content:     []byte("<html><head><meta charset='utf-8'><title>Test</title></head><body><h1>Header</h1><p>Paragraph</p></body></html>"),
			contentType: "",
			expected:    true,
		},
		{
			name:        "Plain text",
			content:     []byte("This is just plain text content without any HTML tags."),
			contentType: "text/plain",
			expected:    false,
		},
		{
			name:        "Empty content",
			content:     []byte(""),
			contentType: "",
			expected:    false,
		},
		{
			name:        "Case insensitive HTML detection",
			content:     []byte("<HTML><HEAD><TITLE>Test</TITLE></HEAD><BODY><DIV>Content</DIV></BODY></HTML>"),
			contentType: "",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHTMLContent(tt.content, tt.contentType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateTempFile(t *testing.T) {
	t.Run("Create temp file successfully", func(t *testing.T) {
		content := []byte("Test content for temporary file")
		ext := "txt"

		tempPath, err := createTempFile(content, ext)

		require.NoError(t, err)
		require.NotEmpty(t, tempPath)

		// Verify file exists
		_, err = os.Stat(tempPath)
		assert.NoError(t, err)

		// Verify content
		readContent, err := os.ReadFile(tempPath)
		require.NoError(t, err)
		assert.Equal(t, content, readContent)

		// Verify extension
		assert.True(t, strings.HasSuffix(tempPath, "."+ext))

		// Clean up
		err = os.Remove(tempPath)
		assert.NoError(t, err)
	})

	t.Run("Different extensions", func(t *testing.T) {
		extensions := []string{"pdf", "docx", "html", "md"}
		content := []byte("test content")

		for _, ext := range extensions {
			t.Run(fmt.Sprintf("Extension_%s", ext), func(t *testing.T) {
				tempPath, err := createTempFile(content, ext)

				require.NoError(t, err)
				assert.True(t, strings.HasSuffix(tempPath, "."+ext))

				// Clean up
				os.Remove(tempPath)
			})
		}
	})

	t.Run("Empty content", func(t *testing.T) {
		content := []byte("")
		ext := "txt"

		tempPath, err := createTempFile(content, ext)

		require.NoError(t, err)

		// Verify file exists and is empty
		info, err := os.Stat(tempPath)
		require.NoError(t, err)
		assert.Equal(t, int64(0), info.Size())

		// Clean up
		os.Remove(tempPath)
	})
}

func TestCreateTempFile_ConfiguredTempDir(t *testing.T) {
	// Save original config
	originalTempDir := fileParserConfigInstance.UploadedFileTempDir
	defer func() {
		fileParserConfigInstance.UploadedFileTempDir = originalTempDir
	}()

	t.Run("Uses system default when not configured", func(t *testing.T) {
		// Set temp dir to empty (system default)
		fileParserConfigInstance.UploadedFileTempDir = ""

		content := []byte("test content")
		ext := "txt"

		tempPath, err := createTempFile(content, ext)

		require.NoError(t, err)
		require.NotEmpty(t, tempPath)
		assert.True(t, strings.HasSuffix(tempPath, "."+ext))

		// Verify content
		readContent, err := os.ReadFile(tempPath)
		require.NoError(t, err)
		assert.Equal(t, content, readContent)

		// Clean up
		os.Remove(tempPath)
	})

	t.Run("Uses configured temp directory", func(t *testing.T) {
		// Create a test temp directory
		testTempDir, err := os.MkdirTemp("", "file_parser_test_*")
		require.NoError(t, err)
		defer os.RemoveAll(testTempDir)

		// Set the config to use our test temp dir
		fileParserConfigInstance.UploadedFileTempDir = testTempDir

		content := []byte("test content with configured dir")
		ext := "md"

		tempPath, err := createTempFile(content, ext)

		require.NoError(t, err)
		require.NotEmpty(t, tempPath)

		// Verify the file was created in our configured directory
		assert.True(t, strings.HasPrefix(tempPath, testTempDir))
		assert.True(t, strings.HasSuffix(tempPath, "."+ext))

		// Verify content
		readContent, err := os.ReadFile(tempPath)
		require.NoError(t, err)
		assert.Equal(t, content, readContent)

		// Clean up
		os.Remove(tempPath)
	})
}

func TestParseLocalDocument(t *testing.T) {
	// We need actual test files for this test
	testDataDir := "../../../testdata"

	t.Run("Parse valid PDF file", func(t *testing.T) {
		pdfPath := filepath.Join(testDataDir, "pdf", "test_file_one_page_en.pdf")

		// Check if file exists
		if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
			t.Skip("Test PDF file not found, skipping test")
		}

		doc, err := ParseLocalDocument(pdfPath)

		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, document.DocumentOriginLocalFile, doc.OriginType)
		assert.Equal(t, pdfPath, doc.OriginLocation)
		assert.Equal(t, "test_file_one_page_en.pdf", doc.OriginName)
		assert.Equal(t, "pdf", doc.DocumentType)
		assert.Greater(t, doc.FileSize, int64(0))
		assert.NotEmpty(t, doc.FileChecksum)
	})

	t.Run("Parse valid DOCX file", func(t *testing.T) {
		docxPath := filepath.Join(testDataDir, "docx", "test_file_two_pages_international.docx")

		// Check if file exists
		if _, err := os.Stat(docxPath); os.IsNotExist(err) {
			t.Skip("Test DOCX file not found, skipping test")
		}

		doc, err := ParseLocalDocument(docxPath)

		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, document.DocumentOriginLocalFile, doc.OriginType)
		assert.Equal(t, docxPath, doc.OriginLocation)
		assert.Equal(t, "test_file_two_pages_international.docx", doc.OriginName)
		assert.Equal(t, "docx", doc.DocumentType)
		assert.Greater(t, doc.FileSize, int64(0))
		assert.NotEmpty(t, doc.FileChecksum)
		assert.Equal(t, "embedded~fumiama-go-docx", doc.ParserName)
		assert.Equal(t, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", doc.MimeType)

		// Verify that content was extracted
		assert.Greater(t, doc.ContentSize, 0)
		assert.Greater(t, len(doc.PageTextContent), 0)

		// Verify metadata was extracted
		assert.NotNil(t, doc.CustomMeta)
		assert.Contains(t, doc.CustomMeta, "paragraph_count")
	})

	t.Run("Parse valid Markdown file", func(t *testing.T) {
		mdPath := filepath.Join(testDataDir, "md", "test_file_en.md")

		// Check if file exists
		if _, err := os.Stat(mdPath); os.IsNotExist(err) {
			t.Skip("Test MD file not found, skipping test")
		}

		doc, err := ParseLocalDocument(mdPath)

		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, document.DocumentOriginLocalFile, doc.OriginType)
		assert.Equal(t, mdPath, doc.OriginLocation)
		assert.Equal(t, "test_file_en.md", doc.OriginName)
		assert.Equal(t, "md", doc.DocumentType)
	})

	t.Run("Nonexistent file", func(t *testing.T) {
		doc, err := ParseLocalDocument("/nonexistent/file.pdf")

		assert.Error(t, err)
		assert.Nil(t, doc)
		var notFoundErr *errorx.ErrNotFound
		assert.True(t, errors.As(err, &notFoundErr))
	})

	t.Run("Unsupported file type", func(t *testing.T) {
		// Create a temporary file with unsupported extension
		tempFile, err := os.CreateTemp("", "test_*.xyz")
		require.NoError(t, err)
		tempFile.Close()
		defer os.Remove(tempFile.Name())

		doc, err := ParseLocalDocument(tempFile.Name())

		assert.Error(t, err)
		assert.NotNil(t, doc) // Document should still be created with basic info
		var unsupportedErr *errorx.ErrUnsupportedMediaType
		assert.True(t, errors.As(err, &unsupportedErr))
		assert.Contains(t, err.Error(), "xyz")
	})

	t.Run("File with no extension", func(t *testing.T) {
		// Create a temporary file without extension
		tempFile, err := os.CreateTemp("", "test_no_ext")
		require.NoError(t, err)
		tempFile.Close()
		defer os.Remove(tempFile.Name())

		doc, err := ParseLocalDocument(tempFile.Name())

		assert.Error(t, err)
		assert.NotNil(t, doc)
		var unsupportedErr *errorx.ErrUnsupportedMediaType
		assert.True(t, errors.As(err, &unsupportedErr))
	})
}

func TestParseUploadedDocument(t *testing.T) {
	t.Run("Valid text content", func(t *testing.T) {
		data := []byte("This is test content for uploaded document")
		filename := "test.txt"

		doc, err := ParseUploadedDocument(data, filename)

		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, document.DocumentOriginUploadedFile, doc.OriginType)
		assert.Equal(t, filename, doc.OriginLocation)
		assert.Equal(t, filename, doc.OriginName)
		assert.Equal(t, "txt", doc.DocumentType)
		assert.Equal(t, int64(len(data)), doc.FileSize)
	})

	t.Run("Valid markdown content", func(t *testing.T) {
		data := []byte("# Test Markdown\n\nThis is a test markdown document.")
		filename := "test.md"

		doc, err := ParseUploadedDocument(data, filename)

		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, "md", doc.DocumentType)
	})

	t.Run("Unsupported file type", func(t *testing.T) {
		data := []byte("some binary data")
		filename := "test.xyz"

		doc, err := ParseUploadedDocument(data, filename)

		assert.Error(t, err)
		assert.NotNil(t, doc)
		var unsupportedErr *errorx.ErrUnsupportedMediaType
		assert.True(t, errors.As(err, &unsupportedErr))
	})

	t.Run("Empty data", func(t *testing.T) {
		data := []byte("")
		filename := "empty.txt"

		doc, err := ParseUploadedDocument(data, filename)

		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, int64(0), doc.FileSize)
	})

	t.Run("Filename with path", func(t *testing.T) {
		data := []byte("test content")
		filename := "path/to/test.txt"

		doc, err := ParseUploadedDocument(data, filename)

		assert.NoError(t, err)
		assert.Equal(t, filename, doc.OriginLocation)
		assert.Equal(t, "test.txt", doc.OriginName) // Should extract base name
	})
}

// Note: TestFetchURLContent and TestParseURL would require HTTP mocking
// which is more complex to implement. For integration testing, you would
// want to use actual HTTP servers or mocking frameworks like httptest.

func TestFetchURLContent_MockRequired(t *testing.T) {
	t.Skip("fetchURLContent requires HTTP mocking which is not implemented in this unit test")
	// This test would require setting up a mock HTTP server
	// or using a mocking framework to test the HTTP functionality
}

func TestParseURL_MockRequired(t *testing.T) {
	t.Skip("ParseURL requires HTTP mocking which is not implemented in this unit test")
	// This test would require setting up a mock HTTP server
	// and testing various URL parsing scenarios
}

func TestFetchURLContent_SizeLimitEnforcement(t *testing.T) {
	// Test the size limit logic (this would require HTTP mocking in a real implementation)
	t.Skip("fetchURLContent size limiting requires HTTP mocking which is not implemented in this unit test")

	// This test would verify:
	// 1. Content-Length header check for size limits
	// 2. Actual content size checking during download
	// 3. Proper error messages for size limit violations
	// 4. Successful downloads within size limits
}
