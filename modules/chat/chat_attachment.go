package chat

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/file_parser"
)

// formatAttachmentContent formats the file content with prefix and suffix
// Returns the formatted content and the actual truncated content size (excluding prefix/suffix)
func formatAttachmentContent(fileName string, content string, originalSize int64) (string, int64, bool) {
	var maxContentSize = chatConfigInstance.UploadedFileMaxContentLength
	const truncationMessage = "... [the tail of the file has been truncated]"

	var truncatedContent string
	var actualContentSize int
	var truncated bool

	if len(content) <= maxContentSize {
		// Content fits within limit
		truncatedContent = content
		actualContentSize = len(content)
		truncated = false
	} else {
		// Content needs truncation - find nearest whitespace
		truncationPoint := maxContentSize

		// Find the nearest whitespace before the truncation point
		for i := maxContentSize - 1; i >= 0; i-- {
			if content[i] == ' ' || content[i] == '\t' || content[i] == '\n' || content[i] == '\r' {
				truncationPoint = i
				break
			}
			// If we've gone back too far (more than 100 chars), just truncate at original point
			if maxContentSize-i > 100 {
				truncationPoint = maxContentSize
				break
			}
		}

		truncatedContent = content[:truncationPoint] + truncationMessage
		actualContentSize = truncationPoint
		truncated = true
	}

	// Apply prefix and suffix after truncation
	prefix := fmt.Sprintf("Attached: %s\n--- start of file %s ---\n", fileName, fileName)
	suffix := fmt.Sprintf("\n--- end of file %s", fileName)

	formattedContent := prefix + truncatedContent + suffix
	return formattedContent, int64(actualContentSize), truncated
}

// processFileAttachment processes an uploaded file and returns the formatted content and metadata
func processFileAttachment(file multipart.File, header *multipart.FileHeader) (string, string, int64, int64, int64, bool, errorx.Error) {
	// Save to temporary file using the shared tempfile utility
	tempPath, err := utils.SaveUploadedTempFile(file, chatConfigInstance.UploadedFileTempDir, header.Filename)
	if err != nil {
		logging.Error("Error saving temporary file: %v", err)
		return "", "", 0, 0, 0, false, errorx.NewErrInternalServerError("Error saving file")
	}

	// Clean up the temporary file when done
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil {
			logging.Warn("Failed to remove temporary file %s: %v", tempPath, removeErr)
		}
	}()

	// Parse the document from the temporary file
	doc, err := file_parser.ParseLocalDocument(tempPath)
	if err != nil {
		// Check if it's an errorx error (like unsupported file type)
		if errx, ok := err.(errorx.Error); ok {
			return "", "", 0, 0, 0, false, errx
		}

		logging.Error("Failed to parse uploaded document: %v", err)
		return "", "", 0, 0, 0, false, errorx.NewErrInternalServerError(fmt.Sprintf("Failed to parse document: %v", err))
	}

	// Get the original file size
	fileInfo, err := os.Stat(tempPath)
	if err != nil {
		logging.Error("Failed to get file info: %v", err)
		return "", "", 0, 0, 0, false, errorx.NewErrInternalServerError("Failed to get file info")
	}
	originalSize := fileInfo.Size()

	// Join all page content to get the full content
	allContent := strings.Join(doc.PageTextContent, "\n\n")
	allContent = strings.TrimSpace(allContent)

	// Calculate full content length before any truncation
	fullContentLength := int64(len(allContent))

	// Format the content with prefix and suffix (this may truncate)
	formattedContent, truncatedContentLength, truncated := formatAttachmentContent(header.Filename, allContent, originalSize)

	// Get file extension
	fileExt := strings.TrimPrefix(filepath.Ext(header.Filename), ".")

	return formattedContent, fileExt, originalSize, fullContentLength, truncatedContentLength, truncated, nil
}
