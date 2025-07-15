package file_parser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
)

var logger = logging.MainLogger

// DocumentParser interface that different implementations will fulfill
type FileParser interface {
	Parse(ctx context.Context, path string, doc *document.Document) errorx.Error
}

// Type definition for parser constructor functions
type ParserConstructor func() FileParser

// Static map of file extensions to parser constructor methods
var parserMap = map[string]ParserConstructor{
	"docx": func() FileParser { return NewDOCXParser() },
	"pdf":  func() FileParser { return NewPDFParser() },
	"html": func() FileParser { return NewHTMLParser() },
	"htm":  func() FileParser { return NewHTMLParser() },
	"txt":  func() FileParser { return NewTextParser() },
	"text": func() FileParser { return NewTextParser() },
	"md":   func() FileParser { return NewTextParser() },
	"go":   func() FileParser { return NewTextParser() },
	"py":   func() FileParser { return NewTextParser() },
	"cpp":  func() FileParser { return NewTextParser() },
	"java": func() FileParser { return NewTextParser() },
	"js":   func() FileParser { return NewTextParser() },
	"ts":   func() FileParser { return NewTextParser() },
	"yaml": func() FileParser { return NewTextParser() },
	"yml":  func() FileParser { return NewTextParser() },
	// Add more parsers here as they are implemented
	// "docx": func() FileParser { return NewDocxParser() },
	// "pptx": func() FileParser { return NewPptxParser() },
	// "xlsx": func() FileParser { return NewXlsxParser() },

}

// Initialize parsers based on configuration
func initModule() error {
	// Log available parsers
	for ext := range parserMap {
		logger.Debug("Available parser for extension: %s", ext)
	}

	// Validate that all extensions in config are supported by available parsers
	supportedExtensions := GetSupportedExtensions("builtin")
	for _, ext := range supportedExtensions {
		if _, exists := parserMap[ext]; !exists {
			logger.Warn("Extension %s in config has no parser implementation", ext)
		}
	}

	return nil
}

// GetParser returns the appropriate parser for a given file extension
func GetParser(extension string) (FileParser, bool) {
	// Check if extension is in the supported list from config
	if !IsSupportedExtension("builtin", extension) {
		logger.Debug("Extension %s is not in the supported list", extension)
		return nil, false
	}

	// Check if we have a parser constructor for this extension
	constructor, exists := parserMap[extension]
	if !exists {
		logger.Debug("No parser constructor found for extension %s", extension)
		return nil, false
	}

	// Create a new parser instance using the constructor
	logger.Trace("Creating parser for extension '%s'", extension)
	return constructor(), true
}

func getFileExtension(path string) string {
	ext := filepath.Ext(path)
	if len(ext) > 1 {
		return strings.ToLower(ext[1:]) // Remove the dot and convert to lowercase
	}
	return ""
}

// GetConfig returns the file parser config instance
func GetConfig() *FileParserConfig {
	return fileParserConfigInstance
}

// Free functions for parsing documents from different sources

// ParseLocalDocument parses a document from local file path
func ParseLocalDocument(path string) (*document.Document, errorx.Error) {
	// Create base document with origin info
	doc := document.NewDocument()
	doc.OriginType = document.DocumentOriginLocalFile
	doc.OriginLocation = path
	doc.OriginName = filepath.Base(path)
	doc.TenantID = auth.GetTenantID()
	doc.UserID = auth.GetUserID()

	// Get file size
	fileInfo, err := os.Stat(path)
	if err != nil {
		msg := fmt.Sprintf("Failed to get file info: %v", err)
		logger.Error("%s", msg)
		return nil, errorx.NewErrInternalServerError(msg)
	}
	doc.FileSize = fileInfo.Size()

	// Get file creation time using platform-specific implementation
	doc.FileCreationTime = utils.GetFileCreationTime(fileInfo)
	doc.FileUpdateTime = fileInfo.ModTime()
	doc.FileChecksum, err = utils.GetFileChecksum(path)
	if err != nil {
		msg := fmt.Sprintf("Failed to get file checksum: %v", err)
		logger.Error("%s", msg)
		return nil, errorx.NewErrInternalServerError(msg)
	}

	// Extract file extension
	ext := getFileExtension(path)
	doc.DocumentType = ext

	// Check if we have a parser for this extension
	parser, exists := GetParser(ext)
	if !exists {
		msg := fmt.Sprintf("No parser found for extension: %s", ext)
		logger.Warn("%s", msg)
		return doc, newUnsupportedFileTypeError(ext)
	}

	// Use the appropriate parser
	ctx := context.Background()
	err = parser.Parse(ctx, path, doc)
	if err != nil {
		msg := fmt.Sprintf("Failed to parse document: %v", err)
		logger.Error("%s", msg)
		return doc, errorx.NewErrInternalServerError(msg)
	}

	return doc, nil
}

// ParseURL parses a document from URL
func ParseURL(url string) (*document.Document, errorx.Error) {
	// Create base document with origin info
	doc := document.NewDocument()
	doc.OriginType = document.DocumentOriginUrl
	doc.OriginLocation = url
	doc.OriginName = url
	doc.TenantID = auth.GetTenantID()
	doc.UserID = auth.GetUserID()

	// Fetch content from URL
	content, contentType, errx := fetchURLContent(url)
	if errx != nil {
		logger.Error("Failed to fetch URL content: %v", errx)
		return nil, errx
	}

	doc.FileSize = int64(len(content))

	// First, try to detect if content is HTML by examining the content itself
	var ext string
	var parser FileParser
	var exists bool

	if isHTMLContent(content, contentType) {
		// Content appears to be HTML, use HTML parser
		ext = "html"
		parser, exists = GetParser("html")
		logger.Debug("Detected HTML content from URL: %s", url)
	} else {
		// Not HTML, determine file extension from content type or URL
		ext = determineExtensionFromURL(url, contentType)
		parser, exists = GetParser(ext)
	}

	doc.DocumentType = ext

	// Check if we have a parser for this extension
	if !exists {
		msg := fmt.Sprintf("No parser found for content type: %s, extension: %s", contentType, ext)
		logger.Warn("%s", msg)
		return doc, errorx.NewErrBadRequest(msg)
	}

	// For URL parsing, we need to create a temporary file since parsers expect file paths
	tempFile, err := createTempFile(content, ext)
	if err != nil {
		msg := fmt.Sprintf("Failed to create temporary file: %v", err)
		logger.Error("%s", msg)
		return doc, errorx.NewErrInternalServerError(msg)
	}
	defer os.Remove(tempFile) // Clean up temp file

	// Use the appropriate parser
	ctx := context.Background()
	err = parser.Parse(ctx, tempFile, doc)
	if err != nil {
		msg := fmt.Sprintf("Failed to parse document from URL: %v", err)
		logger.Error("%s", msg)
		return doc, errorx.NewErrInternalServerError(msg)
	}

	// Update origin location back to URL (parser might have set it to temp file path)
	doc.OriginLocation = url

	return doc, nil
}

// ParseUploadedDocument parses an uploaded document
func ParseUploadedDocument(data []byte, filename string) (*document.Document, error) {
	doc := document.NewDocument()
	doc.OriginType = document.DocumentOriginUploadedFile
	doc.OriginLocation = filename
	doc.OriginName = filepath.Base(filename)
	doc.TenantID = auth.GetTenantID()
	doc.UserID = auth.GetUserID()
	doc.FileSize = int64(len(data))

	// Extract file extension from filename
	ext := getFileExtension(filename)
	doc.DocumentType = ext

	// Check if we have a parser for this extension
	parser, exists := GetParser(ext)
	if !exists {
		logger.Warn("No parser found for extension: %s", ext)
		return doc, newUnsupportedFileTypeError(ext)
	}

	// Create a temporary file to store the uploaded data
	tempFile, err := createTempFile(data, ext)
	if err != nil {
		logger.Error("Failed to create temp file for uploaded document: %v", err)
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile) // Clean up temp file when done

	// Parse the document using the appropriate parser
	logger.Info("Parsing uploaded file with type: %s", ext)
	ctx := context.Background()
	if parseErr := parser.Parse(ctx, tempFile, doc); parseErr != nil {
		logger.Error("Failed to parse uploaded document: %v", parseErr)
		return doc, parseErr
	}

	return doc, nil
}

func InitModule() {
	core.RegisterModule(&core.Module{
		Name:          "file_parser",
		InitAPIRoutes: InitAPIRoutes,
		InitMain:      initModule,
	})

	logger.Info("File parser module initialized")
}

// newUnsupportedFileTypeError creates a new unsupported file type error using errorx
func newUnsupportedFileTypeError(extension string) errorx.Error {
	supported := GetSupportedExtensions("builtin")
	message := fmt.Sprintf("Unsupported file type: '%s'. Supported file types are: %s",
		extension, strings.Join(supported, ", "))
	return errorx.NewErrUnsupportedMediaType("%s", message)
}

// fetchURLContent fetches content from a URL and returns the content and content type
func fetchURLContent(url string) ([]byte, string, errorx.Error) {
	// Calculate max allowed size in bytes
	maxSizeBytes := int64(fileParserConfigInstance.UploadedFileMaxSizeKB * 1024)
	logger.Debug("Fetching URL content with max size limit: %d KB (%d bytes)",
		fileParserConfigInstance.UploadedFileMaxSizeKB, maxSizeBytes)

	// Create HTTP client with reasonable timeout
	client := &http.Client{
		Timeout: time.Duration(30) * time.Second,
	}

	// Make GET request
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", errorx.NewErrBadRequest("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", errorx.NewErrBadRequest("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	// Check Content-Length header if available
	if contentLength := resp.ContentLength; contentLength > 0 {
		logger.Debug("URL content length from header: %d bytes", contentLength)
		if contentLength > maxSizeBytes {
			logger.Warn("URL content size (%d bytes) exceeds limit (%d bytes) based on Content-Length header",
				contentLength, maxSizeBytes)
			return nil, "", errorx.NewErrPayloadTooLarge("file size %d KB exceeds maximum allowed size %d KB",
				contentLength/1024, fileParserConfigInstance.UploadedFileMaxSizeKB)
		}
	} else {
		logger.Debug("No Content-Length header found, will check size during download")
	}

	// Use LimitReader to prevent reading more than the maximum allowed size
	// Add 1 to detect if the content exceeds the limit
	limitedReader := io.LimitReader(resp.Body, maxSizeBytes+1)

	// Read response body with size limit
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", errorx.NewErrBadRequest("failed to read response body: %w", err)
	}

	// Check if the content size exceeds the limit
	if int64(len(content)) > maxSizeBytes {
		logger.Warn("URL content size (%d bytes) exceeds limit (%d bytes) after download",
			len(content), maxSizeBytes)
		return nil, "", errorx.NewErrPayloadTooLarge("file content size %d KB exceeds maximum allowed size %d KB",
			len(content)/1024, fileParserConfigInstance.UploadedFileMaxSizeKB)
	}

	logger.Debug("Successfully downloaded URL content: %d bytes", len(content))

	// Get content type from headers
	contentType := resp.Header.Get("Content-Type")

	return content, contentType, nil
}

// determineExtensionFromURL determines file extension from URL path or content type
func determineExtensionFromURL(url, contentType string) string {
	// First try to get extension from URL path
	if ext := getFileExtension(url); ext != "" {
		return ext
	}

	// Fall back to determining from content type
	switch {
	case strings.Contains(contentType, "text/html"), strings.Contains(contentType, "application/xhtml"):
		return "html"
	case strings.Contains(contentType, "application/pdf"):
		return "pdf"
	case strings.Contains(contentType, "text/plain"), strings.Contains(contentType, "text/text"):
		return "txt"
	case strings.Contains(contentType, "text/markdown"):
		return "md"
	case strings.Contains(contentType, "application/json"), strings.Contains(contentType, "text/json"):
		return "txt" // Treat JSON as text for now
	case strings.Contains(contentType, "application/xml"), strings.Contains(contentType, "text/xml"):
		return "txt" // Treat XML as text for now
	default:
		// Default to text if we can't determine and it's not obviously HTML
		logger.Debug("Could not determine file type from URL %s or content type %s, defaulting to txt", url, contentType)
		return "txt"
	}
}

// createTempFile creates a temporary file with the given content and extension
func createTempFile(content []byte, ext string) (string, error) {
	// Determine which temp directory to use
	tempDir := fileParserConfigInstance.UploadedFileTempDir
	if tempDir == "" {
		// If not configured, use system default (empty string means system temp dir)
		tempDir = ""
		logger.Debug("Using system default temp directory for file creation")
	} else {
		logger.Debug("Using configured temp directory: %s", tempDir)
	}

	// Create temporary file with appropriate extension in the configured directory
	tempFile, err := os.CreateTemp(tempDir, fmt.Sprintf("hyperspot_url_parse_*.%s", ext))
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file in directory '%s': %w", tempDir, err)
	}
	defer tempFile.Close()

	// Write content to temporary file
	_, err = tempFile.Write(content)
	if err != nil {
		if removeErr := os.Remove(tempFile.Name()); removeErr != nil {
			logger.Warn("Failed to remove temp file %s: %v", tempFile.Name(), removeErr)
		}
		return "", fmt.Errorf("failed to write content to temporary file: %w", err)
	}

	logger.Debug("Created temporary file: %s", tempFile.Name())
	return tempFile.Name(), nil
}

// isHTMLContent determines if the content is HTML by examining content type and content patterns
func isHTMLContent(content []byte, contentType string) bool {
	// First check content type
	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		return true
	}

	// If content type is not decisive, examine the content itself
	contentStr := strings.ToLower(string(content))

	// Look for common HTML patterns
	htmlPatterns := []string{
		"<!doctype html",
		"<html",
		"<head>",
		"<body>",
		"<title>",
		"<meta",
		"<div",
		"<p>",
		"<h1>", "<h2>", "<h3>",
		"<script",
		"<style",
	}

	// Count how many HTML patterns are found
	patternCount := 0
	for _, pattern := range htmlPatterns {
		if strings.Contains(contentStr, pattern) {
			patternCount++
		}
	}

	// If we find multiple HTML patterns, it's likely HTML
	// We use a threshold to avoid false positives
	return patternCount >= 2
}
