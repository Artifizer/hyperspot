package chat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFormatAttachmentContent_EdgeCases tests edge cases for formatAttachmentContent
func TestFormatAttachmentContent_EdgeCases(t *testing.T) {
	// Test with empty content
	emptyContent := ""
	fileName := "empty.txt"
	originalSize := int64(0)

	formattedContent, contentLength, isTruncated := formatAttachmentContent(fileName, emptyContent, originalSize)

	// Verify the formatted content contains the expected prefix and suffix
	assert.Contains(t, formattedContent, "Attached: "+fileName)
	assert.Contains(t, formattedContent, "--- start of file "+fileName+" ---")
	assert.Contains(t, formattedContent, "--- end of file "+fileName)
	assert.Equal(t, int64(0), contentLength) // Empty content should have 0 truncated size
	assert.False(t, isTruncated)

	// Test with extremely long filename
	extremelyLongFileName := strings.Repeat("extremelylongfilename", 50) + ".txt"
	shortContent := "Short content"
	originalSize = int64(len(shortContent))

	formattedContent, contentLength, isTruncated = formatAttachmentContent(extremelyLongFileName, shortContent, originalSize)

	// Verify the formatted content contains at least the minimal format
	assert.Contains(t, formattedContent, "Attached: "+extremelyLongFileName)
	assert.Contains(t, formattedContent, shortContent)
	assert.Equal(t, int64(len(shortContent)), contentLength) // Should not be truncated
	assert.False(t, isTruncated)
}

// TestFormatAttachmentContent verifies the formatAttachmentContent function
func TestFormatAttachmentContent(t *testing.T) {
	// Test case 1: Content fits within available space
	shortContent := "This is a short content that should fit within the available space."
	fileName := "test.txt"
	originalSize := int64(len(shortContent))

	formattedContent, contentLength, isTruncated := formatAttachmentContent(fileName, shortContent, originalSize)

	// Verify the formatted content contains the expected prefix and suffix
	assert.Contains(t, formattedContent, "Attached: "+fileName)
	assert.Contains(t, formattedContent, "--- start of file "+fileName+" ---")
	assert.Contains(t, formattedContent, "--- end of file "+fileName)
	assert.Contains(t, formattedContent, shortContent)
	assert.Equal(t, int64(len(shortContent)), contentLength)
	assert.False(t, isTruncated)

	// Test case 2: Content exceeds available space and gets truncated
	longContent := strings.Repeat("This is a very long content that will exceed the available space. ", chatConfigInstance.UploadedFileMaxContentLength/20)
	originalSize = int64(len(longContent))

	formattedContent, contentLength, isTruncated = formatAttachmentContent(fileName, longContent, originalSize)

	// Verify the formatted content contains the expected prefix and suffix
	assert.Contains(t, formattedContent, "Attached: "+fileName)
	assert.Contains(t, formattedContent, "--- start of file "+fileName+" ---")
	assert.Contains(t, formattedContent, "--- end of file "+fileName)

	// Verify the content was truncated
	assert.Contains(t, formattedContent, "... [the tail of the file has been truncated]")
	assert.Less(t, contentLength, int64(len(longContent))) // Content should be truncated
	assert.True(t, isTruncated)

	// Test case 3: Very long filename
	veryLongFileName := strings.Repeat("verylongfilename", 50) + ".txt"
	formattedContent, contentLength, isTruncated = formatAttachmentContent(veryLongFileName, shortContent, originalSize)

	// Verify the formatted content contains the expected prefix
	assert.Contains(t, formattedContent, "Attached: "+veryLongFileName)

	// Verify the content is still present
	assert.Contains(t, formattedContent, shortContent)
	assert.Equal(t, int64(len(shortContent)), contentLength)
	assert.False(t, isTruncated)
}

// TestFormatAttachmentContent_Truncation tests that content is properly truncated
func TestFormatAttachmentContent_Truncation(t *testing.T) {
	// Create content that is exactly at the limit
	fileName := "test.txt"
	maxContentSize := chatConfigInstance.UploadedFileMaxContentLength

	// Create content that exactly fits the new limit (16K characters)
	exactContent := strings.Repeat("a", maxContentSize)
	originalSize := int64(len(exactContent))

	formattedContent, contentLength, isTruncated := formatAttachmentContent(fileName, exactContent, originalSize)

	// Verify the content was not truncated
	assert.Equal(t, int64(maxContentSize), contentLength)
	assert.Contains(t, formattedContent, exactContent)
	assert.NotContains(t, formattedContent, "... [the tail of the file has been truncated]")
	assert.False(t, isTruncated)

	// Now test with content that is too long and needs truncation
	longContent := strings.Repeat("This is a test sentence with spaces. ", chatConfigInstance.UploadedFileMaxContentLength/10) // Create content with spaces
	originalSize = int64(len(longContent))

	formattedContent, contentLength, isTruncated = formatAttachmentContent(fileName, longContent, originalSize)

	// Verify the content was truncated
	assert.Less(t, contentLength, int64(len(longContent)))
	assert.Contains(t, formattedContent, "... [the tail of the file has been truncated]")
	assert.Greater(t, contentLength, int64(0))
	assert.True(t, isTruncated)
}

// TestFormatAttachmentContent_WithSpecialChars tests formatting with special characters
func TestFormatAttachmentContent_WithSpecialChars(t *testing.T) {
	// Test with content containing special characters
	specialContent := "Line 1\nLine 2\r\nLine 3\tTabbed\"Quoted\"\\Backslash"
	fileName := "special.txt"
	originalSize := int64(len(specialContent))

	formattedContent, contentLength, isTruncated := formatAttachmentContent(fileName, specialContent, originalSize)

	// Verify the formatted content contains the special characters
	assert.Contains(t, formattedContent, specialContent)
	assert.Contains(t, formattedContent, "Attached: "+fileName)
	assert.Equal(t, int64(len(specialContent)), contentLength)
	assert.False(t, isTruncated)
}

// TestFormatAttachmentContent_TruncationToWhitespace tests truncation to nearest whitespace
func TestFormatAttachmentContent_TruncationToWhitespace(t *testing.T) {
	fileName := "test.txt"

	// Create content that will need truncation with specific whitespace positioning
	baseContent := strings.Repeat("x ", chatConfigInstance.UploadedFileMaxContentLength/2-10) // Creates content with spaces
	content := baseContent + "finalwordwithoutspaces" + strings.Repeat("z", 1000)
	originalSize := int64(len(content))

	formattedContent, contentLength, isTruncated := formatAttachmentContent(fileName, content, originalSize)

	// Verify truncation occurred
	assert.Less(t, contentLength, int64(len(content)))
	assert.Contains(t, formattedContent, "... [the tail of the file has been truncated]")
	assert.True(t, isTruncated)

	// Verify truncation happened at a space (should end with "word " before truncation message)
	contentPart := strings.Split(formattedContent, "... [the tail of the file has been truncated]")[0]
	lines := strings.Split(contentPart, "\n")
	actualContent := ""
	for _, line := range lines {
		if !strings.Contains(line, "Attached:") && !strings.Contains(line, "--- start") {
			actualContent += line
		}
	}

	// The truncated content should end with a space (indicating truncation at whitespace)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(actualContent), " x"),
		"Content should be truncated at whitespace boundary")
}
