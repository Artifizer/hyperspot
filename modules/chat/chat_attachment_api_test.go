package chat

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"gorm.io/gorm"
)

// setupTestDBForAttachmentAPI initializes an in-memory SQLite DB for attachment API tests
func setupTestDBForAttachmentAPI(t *testing.T) *gorm.DB {
	testDB, err := db.InitInMemorySQLite(nil)
	require.NoError(t, err, "Failed to connect to test DB")

	db.SetDB(testDB)
	err = db.SafeAutoMigrate(testDB, &ChatMessage{}, &ChatThread{}, &ChatThreadGroup{}, &SystemPrompt{}, &ChatThreadSystemPrompt{})
	require.NoError(t, err, "Failed to auto migrate schema")

	return testDB
}

// TestFormatAttachmentContent_Success tests the formatAttachmentContent function
func TestFormatAttachmentContent_Success(t *testing.T) {
	// Test with valid content
	testContent := "This is test file content for attachment API testing."
	fileName := "test_file.txt"
	originalSize := int64(len(testContent))

	formattedContent, _, _ := formatAttachmentContent(fileName, testContent, originalSize)

	// Verify results
	assert.Contains(t, formattedContent, fmt.Sprintf("Attached: %s", fileName))
	assert.Contains(t, formattedContent, fmt.Sprintf("--- start of file %s ---", fileName))
	assert.Contains(t, formattedContent, fmt.Sprintf("--- end of file %s", fileName))
	assert.Contains(t, formattedContent, testContent)
}

// TestFormatAttachmentContent_LongContent tests content truncation
func TestFormatAttachmentContent_LongContent(t *testing.T) {
	// Create content that exceeds the limit
	longContent := ""
	for i := 0; i < chatConfigInstance.UploadedFileMaxContentLength*2; i++ {
		longContent += "A"
	}

	fileName := "long_file.txt"
	originalSize := int64(len(longContent))

	formattedContent, contentLength, truncated := formatAttachmentContent(fileName, longContent, originalSize)

	// Verify content is truncated to fit within 16K limit
	assert.LessOrEqual(t, contentLength, int64(chatConfigInstance.UploadedFileMaxContentLength))
	assert.Contains(t, formattedContent, fmt.Sprintf("Attached: %s", fileName))
	assert.Contains(t, formattedContent, fmt.Sprintf("--- start of file %s ---", fileName))
	assert.True(t, truncated)
}

// TestCreateThreadWithAttachment_Success tests successful thread creation with attachment
func TestCreateThreadWithAttachment_Success(t *testing.T) {
	// Setup test database
	setupTestDBForAttachmentAPI(t)

	ctx := context.Background()

	// Test data
	testContent := "# Test Markdown\n\nThis is test content for thread creation."
	fileName := "test.md"
	fileExt := "md"
	originalSize := int64(len(testContent))

	// Calculate full content length before truncation
	fullContentLength := int64(len(testContent))

	// Format the content as it would be done by processFileAttachment
	formattedContent, truncatedContentLength, truncated := formatAttachmentContent(fileName, testContent, originalSize)

	// Create the file attachment message (this creates a new thread)
	msg, errx := CreateFileAttachmentMessage(
		ctx,
		uuid.Nil, // No group ID
		uuid.Nil, // No thread ID - will create new thread
		formattedContent,
		fileName,
		fileExt,
		originalSize,
		fullContentLength,
		truncatedContentLength,
		truncated,
	)

	// Verify results
	assert.Nil(t, errx)
	assert.NotNil(t, msg)
	assert.NotEqual(t, uuid.Nil, msg.ID)
	assert.NotEqual(t, uuid.Nil, msg.ThreadID)
	assert.True(t, msg.IsFileAttachment)
	assert.Equal(t, fileName, msg.AttachedFileName)
	assert.Equal(t, fileExt, msg.AttachedFileExtension)
	assert.Equal(t, originalSize, msg.OriginalFileSize)
	assert.False(t, truncated)
	assert.Contains(t, msg.Content, testContent)
}

// TestUploadAttachmentToThread_Success tests successful file upload to existing thread
func TestUploadAttachmentToThread_Success(t *testing.T) {
	// Setup test database
	setupTestDBForAttachmentAPI(t)

	ctx := context.Background()

	// Create a test thread first
	thread, errx := CreateChatThread(ctx, uuid.Nil, "Test Thread", nil, nil, nil, []uuid.UUID{})
	require.Nil(t, errx)
	require.NotNil(t, thread)

	// Test data
	testContent := "This is test content for existing thread attachment."
	fileName := "existing_thread_test.txt"
	fileExt := "txt"
	originalSize := int64(len(testContent))

	// Calculate full content length before truncation
	fullContentLength := int64(len(testContent))

	// Format the content as it would be done by processFileAttachment
	formattedContent, truncatedContentLength, isTruncated := formatAttachmentContent(fileName, testContent, originalSize)

	// Create the file attachment message for existing thread
	msg, errx := CreateFileAttachmentMessage(
		ctx,
		uuid.Nil,  // No group ID
		thread.ID, // Existing thread ID
		formattedContent,
		fileName,
		fileExt,
		originalSize,
		fullContentLength,
		truncatedContentLength,
		isTruncated,
	)

	// Verify results
	assert.Nil(t, errx)
	assert.NotNil(t, msg)
	assert.NotEqual(t, uuid.Nil, msg.ID)
	assert.Equal(t, thread.ID, msg.ThreadID) // Should be in the existing thread
	assert.True(t, msg.IsFileAttachment)
	assert.Equal(t, fileName, msg.AttachedFileName)
	assert.Equal(t, fileExt, msg.AttachedFileExtension)
	assert.Equal(t, originalSize, msg.OriginalFileSize)
	assert.Equal(t, fullContentLength, msg.FullContentLength)
	assert.Equal(t, truncatedContentLength, msg.TruncatedContentLength)
	assert.Contains(t, msg.Content, testContent)
}

// TestUploadAttachmentToThread_NonExistentThread tests error handling for non-existent thread
func TestUploadAttachmentToThread_NonExistentThread(t *testing.T) {
	// Setup test database
	setupTestDBForAttachmentAPI(t)

	ctx := context.Background()

	// Use a non-existent thread ID
	nonExistentThreadID := uuid.New()

	// Try to get the non-existent thread (this should fail)
	_, errx := GetChatThread(ctx, nonExistentThreadID)

	// Verify error
	assert.NotNil(t, errx)
	assert.IsType(t, &errorx.ErrNotFound{}, errx)
}

// TestCreateThreadWithAttachment_InvalidGroupID tests error handling for invalid group ID
func TestCreateThreadWithAttachment_InvalidGroupID(t *testing.T) {
	// Setup test database
	setupTestDBForAttachmentAPI(t)

	ctx := context.Background()

	// Use a non-existent group ID
	nonExistentGroupID := uuid.New()

	// Try to get the non-existent group (this should fail)
	_, errx := GetChatThreadGroup(ctx, nonExistentGroupID)

	// Verify error
	assert.NotNil(t, errx)
	assert.IsType(t, &errorx.ErrNotFound{}, errx)
}

// TestMultipartFormProcessing_NoFile tests error handling when no file is provided
func TestMultipartFormProcessing_NoFile(t *testing.T) {
	// Create multipart form without file field
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add a non-file field
	err := writer.WriteField("other_field", "other_value")
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	// Parse the multipart form
	req := httptest.NewRequest("POST", "/test", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	err = req.ParseMultipartForm(32 << 20)
	require.NoError(t, err)

	form := req.MultipartForm

	// Test that we handle missing file correctly
	files := form.File["file"]
	assert.Len(t, files, 0, "Should have no files in the form")
}

// TestMultipartFormProcessing_MultipleFiles tests handling of multiple files (should use first)
func TestMultipartFormProcessing_MultipleFiles(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Create first file
	part1, err := writer.CreateFormFile("file", "file1.txt")
	require.NoError(t, err)
	_, err = part1.Write([]byte("Content of file 1"))
	require.NoError(t, err)

	// Create second file
	part2, err := writer.CreateFormFile("file", "file2.txt")
	require.NoError(t, err)
	_, err = part2.Write([]byte("Content of file 2"))
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	// Parse the multipart form
	req := httptest.NewRequest("POST", "/test", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	err = req.ParseMultipartForm(32 << 20)
	require.NoError(t, err)

	form := req.MultipartForm

	// Test that we get multiple files
	files := form.File["file"]
	assert.Len(t, files, 2, "Should have 2 files in the form")

	// Test that we would use the first file
	firstFile := files[0]
	assert.Equal(t, "file1.txt", firstFile.Filename)
}

// TestFormatAttachmentContent_Integration tests the integration with real content
func TestFormatAttachmentContent_Integration(t *testing.T) {
	// Test with markdown content
	markdownContent := `# Test Document

## Section 1
This is a test section with some content.

## Section 2
Another section with different content.

- List item 1
- List item 2
- List item 3

**Bold text** and *italic text*.`

	fileName := "test_document.md"
	originalSize := int64(len(markdownContent))

	formattedContent, _, _ := formatAttachmentContent(fileName, markdownContent, originalSize)

	// Verify the structure
	assert.Contains(t, formattedContent, "Attached: "+fileName)
	assert.Contains(t, formattedContent, "--- start of file "+fileName+" ---")
	assert.Contains(t, formattedContent, "--- end of file "+fileName)
	assert.Contains(t, formattedContent, "# Test Document")
	assert.Contains(t, formattedContent, "## Section 1")
	assert.Contains(t, formattedContent, "**Bold text**")

	// Verify the total length is reasonable
	assert.Less(t, len(formattedContent), 16*1024, "Formatted content should be within size limit")
}

// TestCreateFileAttachmentMessage_ValidData tests the CreateFileAttachmentMessage function with valid data
func TestCreateFileAttachmentMessage_ValidData(t *testing.T) {
	// Setup test database
	setupTestDBForAttachmentAPI(t)

	ctx := context.Background()

	// Test data
	content := "Test file content for message creation"
	fileName := "test.txt"
	fileExt := "txt"
	originalSize := int64(len(content))
	fullContentLength := int64(len(content))
	truncatedContentLength := int64(len(content))
	isTruncated := false

	// Create file attachment message
	msg, errx := CreateFileAttachmentMessage(ctx, uuid.Nil, uuid.Nil, content, fileName, fileExt, originalSize, fullContentLength, truncatedContentLength, isTruncated)

	// Verify results
	assert.Nil(t, errx)
	assert.NotNil(t, msg)
	assert.NotEqual(t, uuid.Nil, msg.ID)
	assert.NotEqual(t, uuid.Nil, msg.ThreadID)
	assert.True(t, msg.IsFileAttachment)
	assert.Equal(t, fileName, msg.AttachedFileName)
	assert.Equal(t, fileExt, msg.AttachedFileExtension)
	assert.Equal(t, originalSize, msg.OriginalFileSize)
	assert.Equal(t, fullContentLength, msg.FullContentLength)
	assert.Equal(t, truncatedContentLength, msg.TruncatedContentLength)
	assert.Equal(t, content, msg.Content)
}

// TestCreateFileAttachmentMessage_EmptyFileName tests error handling for empty filename
func TestCreateFileAttachmentMessage_EmptyFileName(t *testing.T) {
	// Setup test database
	setupTestDBForAttachmentAPI(t)

	ctx := context.Background()

	// Test with empty filename
	content := "Test content"
	fileName := ""
	fileExt := "txt"
	originalSize := int64(len(content))
	fullContentLength := int64(len(content))
	truncatedContentLength := int64(len(content))
	isTruncated := false

	// Create file attachment message should succeed even with empty filename
	msg, errx := CreateFileAttachmentMessage(ctx, uuid.Nil, uuid.Nil, content, fileName, fileExt, originalSize, fullContentLength, truncatedContentLength, isTruncated)

	// Verify results - should still work
	assert.Nil(t, errx)
	assert.NotNil(t, msg)
	assert.Equal(t, fileName, msg.AttachedFileName) // Should preserve empty filename
}
