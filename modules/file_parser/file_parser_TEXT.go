package file_parser

import (
	"context"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var textLogger = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

// FileParserText implements the FileParser interface for text files
type FileParserText struct{}

// NewTextParser creates a new instance of FileParserText
func NewTextParser() *FileParserText {
	return &FileParserText{}
}

// Parse implements the FileParser interface for text files
func (p *FileParserText) Parse(ctx context.Context, path string, doc *document.Document) errorx.Error {
	textLogger.Debug("Parsing text file: %s", path)

	// Validate file existence
	if errx := p.validateFile(path); errx != nil {
		return errx
	}

	// Read file content
	content, errx := p.readFileContent(path)
	if errx != nil {
		return errx
	}

	// Set basic document properties
	doc.DocumentType = getFileExtension(path)
	doc.CustomMeta = make(map[string]interface{})
	doc.ParserName = "embedded~text"
	doc.MimeType = "text/plain"

	// Extract content and metadata
	p.extractPageContent(content, doc)
	p.extractTextMetadata(content, doc)

	textLogger.Debug("Successfully parsed text file: %s (content size: %d)",
		path, doc.ContentSize)

	return nil
}

// validateFile checks if the file exists
func (p *FileParserText) validateFile(path string) errorx.Error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return errorx.NewErrBadRequest("file does not exist")
	}
	return nil
}

// readFileContent reads the entire file content
func (p *FileParserText) readFileContent(path string) (string, errorx.Error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errorx.NewErrInternalServerError("failed to read text file: %v", err)
	}

	// Check if content is valid UTF-8, if not try to handle it gracefully
	if !utf8.Valid(content) {
		textLogger.Warn("File contains non-UTF-8 content, attempting to clean it")
		// Convert invalid UTF-8 sequences to replacement characters
		content = []byte(strings.ToValidUTF8(string(content), ""))
	}

	return string(content), nil
}

// extractPageContent extracts text content and organizes it
func (p *FileParserText) extractPageContent(content string, doc *document.Document) {
	// Clean up the content
	cleanContent := p.cleanupText(content)

	// For text files, we treat the entire content as one page
	pageTexts := []string{cleanContent}

	doc.PageTextContent = pageTexts
	doc.TotalPagesCount = 1
	doc.SetNonEmptyPages()

	// Calculate content size
	doc.ContentSize = len(cleanContent)
}

// cleanupText normalizes text content
func (p *FileParserText) cleanupText(text string) string {
	// Normalize line endings to Unix-style
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// Trim excessive whitespace but preserve intentional formatting
	// Remove trailing spaces from lines
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	text = strings.Join(lines, "\n")

	// Remove excessive empty lines (more than 2 consecutive)
	for strings.Contains(text, "\n\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n\n", "\n\n\n")
	}

	// Trim leading and trailing whitespace from entire document
	text = strings.TrimSpace(text)

	return text
}

// extractTextMetadata extracts metadata from text content
func (p *FileParserText) extractTextMetadata(content string, doc *document.Document) {
	metadata := make(map[string]interface{})

	// Basic text statistics
	metadata["character_count"] = len(content)
	metadata["character_count_no_spaces"] = len(strings.ReplaceAll(content, " ", ""))

	// Word count (simple split by whitespace)
	words := strings.Fields(content)
	metadata["word_count"] = len(words)

	// Line count
	lines := strings.Split(content, "\n")
	metadata["line_count"] = len(lines)

	// Non-empty line count
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}
	metadata["non_empty_line_count"] = nonEmptyLines

	// Paragraph count (estimate based on empty lines)
	paragraphs := strings.Split(content, "\n\n")
	nonEmptyParagraphs := 0
	for _, para := range paragraphs {
		if strings.TrimSpace(para) != "" {
			nonEmptyParagraphs++
		}
	}
	metadata["paragraph_count"] = nonEmptyParagraphs

	// Character encoding information
	if utf8.ValidString(content) {
		metadata["encoding"] = "UTF-8"
	} else {
		metadata["encoding"] = "non-UTF-8 (converted)"
	}

	// Simple language detection hints (basic checks)
	p.extractLanguageHints(content, metadata)

	// Text complexity metrics
	p.extractComplexityMetrics(content, words, metadata)

	// Set all metadata
	for key, value := range metadata {
		doc.CustomMeta[key] = value
	}
}

// extractLanguageHints provides basic language detection hints
func (p *FileParserText) extractLanguageHints(content string, metadata map[string]interface{}) {
	// Convert to lowercase for analysis
	lowerContent := strings.ToLower(content)

	// Count common English words
	englishWords := []string{"the", "and", "or", "but", "in", "on", "at", "to", "for", "of", "with", "by"}
	englishCount := 0
	for _, word := range englishWords {
		englishCount += strings.Count(lowerContent, " "+word+" ")
		// Also check at beginning and end of text
		if strings.HasPrefix(lowerContent, word+" ") {
			englishCount++
		}
		if strings.HasSuffix(lowerContent, " "+word) {
			englishCount++
		}
	}

	if englishCount > 0 {
		metadata["english_indicators"] = englishCount
	}

	// Check for common programming language indicators
	codeIndicators := []string{
		"function", "class", "import", "export", "var", "let", "const",
		"def", "print", "return", "if", "else", "for", "while",
		"#include", "public", "private", "static", "void",
	}
	codeCount := 0
	for _, indicator := range codeIndicators {
		if strings.Contains(lowerContent, indicator) {
			codeCount++
		}
	}

	if codeCount > 2 {
		metadata["likely_code"] = true
		metadata["code_indicators"] = codeCount
	}
}

// extractComplexityMetrics calculates text complexity metrics
func (p *FileParserText) extractComplexityMetrics(content string, words []string, metadata map[string]interface{}) {
	if len(words) == 0 {
		return
	}

	// Average word length
	totalWordLength := 0
	for _, word := range words {
		// Clean word of punctuation for length calculation
		cleanWord := strings.Trim(word, ".,!?;:\"'()[]{}/-")
		totalWordLength += len(cleanWord)
	}
	avgWordLength := float64(totalWordLength) / float64(len(words))
	metadata["average_word_length"] = avgWordLength

	// Sentence count (estimate based on sentence-ending punctuation)
	sentenceEnders := []string{".", "!", "?"}
	sentenceCount := 0
	for _, ender := range sentenceEnders {
		sentenceCount += strings.Count(content, ender)
	}
	if sentenceCount > 0 {
		metadata["estimated_sentence_count"] = sentenceCount
		metadata["average_words_per_sentence"] = float64(len(words)) / float64(sentenceCount)
	}

	// Unique word count (simple case-insensitive)
	uniqueWords := make(map[string]bool)
	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,!?;:\"'()[]{}/-"))
		if cleanWord != "" {
			uniqueWords[cleanWord] = true
		}
	}
	metadata["unique_word_count"] = len(uniqueWords)

	if len(uniqueWords) > 0 {
		metadata["vocabulary_richness"] = float64(len(uniqueWords)) / float64(len(words))
	}
}
