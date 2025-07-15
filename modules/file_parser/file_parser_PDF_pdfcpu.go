package file_parser

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

var pdfLoggerPdfcpu = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

// FileParserPDF implements the FileParser interface for PDF files
type FileParserPDFPdfcpu struct{}

// NewPDFParser creates a new instance of FileParserPDF
func NewPDFParserPdfcpu() *FileParserPDFPdfcpu {
	return &FileParserPDFPdfcpu{}
}

// Parse implements the FileParser interface for PDF files
func (p *FileParserPDFPdfcpu) Parse(ctx context.Context, path string, doc *document.Document) errorx.Error {
	pdfLoggerPdfcpu.Debug("Parsing PDF file: %s", path)

	// Validate file existence
	if errx := p.validateFile(path); errx != nil {
		return errx
	}

	// Set basic document properties
	doc.DocumentType = "pdf"
	doc.CustomMeta = make(map[string]interface{})
	doc.ParserName = "embedded~pdfcpu"
	doc.MimeType = "application/pdf"

	// Extract PDF information and content
	if errx := p.extractPDFContent(path, doc); errx != nil {
		return errx
	}

	pdfLoggerPdfcpu.Debug("Successfully parsed PDF file: %s (pages: %d, content size: %d)",
		path, doc.TotalPagesCount, doc.ContentSize)

	return nil
}

// validateFile checks if the file exists
func (p *FileParserPDFPdfcpu) validateFile(path string) errorx.Error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return errorx.NewErrBadRequest("file does not exist")
	}
	return nil
}

// extractPDFContent extracts all content and metadata from the PDF
func (p *FileParserPDFPdfcpu) extractPDFContent(path string, doc *document.Document) errorx.Error {
	// Configure pdfcpu
	config := model.NewDefaultConfiguration()
	config.ValidationMode = model.ValidationRelaxed

	pdfLoggerPdfcpu.Debug("Reading PDF context from file: %s", path)

	// Use the file-based context reading instead of ReadSeeker
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		pdfLoggerPdfcpu.Debug("Failed to read PDF context: %v", err)
		return errorx.NewErrBadRequest("failed to read PDF context: %v", err)
	}

	pdfLoggerPdfcpu.Debug("PDF context loaded successfully. Page count: %d", ctx.PageCount)

	// Set page count
	doc.TotalPagesCount = ctx.PageCount

	// Validate that we have pages
	if ctx.PageCount == 0 {
		pdfLoggerPdfcpu.Warn("PDF has zero pages")
		doc.PageTextContent = []string{}
		return nil
	}

	// Extract text content from all pages
	if errx := p.extractTextContent(ctx, doc, config); errx != nil {
		return errx
	}

	// Extract font metadata
	p.extractFontMetadata(ctx, doc)

	// Calculate content size
	doc.ContentSize = 0
	for _, pageText := range doc.PageTextContent {
		doc.ContentSize += len(pageText)
	}

	pdfLoggerPdfcpu.Debug("Extracted text from %d pages, total content size: %d", doc.TotalPagesCount, doc.ContentSize)

	// Set non-empty pages count
	doc.SetNonEmptyPages()

	return nil
}

// extractTextContent extracts text from all pages using content extraction
func (p *FileParserPDFPdfcpu) extractTextContent(ctx *model.Context, doc *document.Document, config *model.Configuration) errorx.Error {
	// Extract text from each page
	pageTexts := make([]string, ctx.PageCount)

	for pageNum := 1; pageNum <= ctx.PageCount; pageNum++ {
		pageText, errx := p.extractSinglePageText(ctx, pageNum)
		if errx != nil {
			pdfLoggerPdfcpu.Warn("Failed to extract text from page %d: %v", pageNum, errx)
			pageTexts[pageNum-1] = ""
		} else {
			pageTexts[pageNum-1] = pageText
		}
	}

	doc.PageTextContent = pageTexts
	return nil
}

// extractSinglePageText extracts text content from a single page
func (p *FileParserPDFPdfcpu) extractSinglePageText(ctx *model.Context, pageNum int) (string, errorx.Error) {
	pdfLoggerPdfcpu.Debug("Extracting text from page %d", pageNum)

	// Extract page content using pdfcpu
	reader, err := pdfcpu.ExtractPageContent(ctx, pageNum)
	if err != nil {
		pdfLoggerPdfcpu.Debug("Failed to extract page content for page %d: %v", pageNum, err)
		return "", errorx.NewErrBadRequest("failed to extract page content: %v", err)
	}

	// Read the content
	content, err := io.ReadAll(reader)
	if err != nil {
		pdfLoggerPdfcpu.Debug("Failed to read page content for page %d: %v", pageNum, err)
		return "", errorx.NewErrBadRequest("failed to read page content: %v", err)
	}

	pdfLoggerPdfcpu.Debug("Page %d content stream size: %d bytes", pageNum, len(content))

	if len(content) == 0 {
		pdfLoggerPdfcpu.Debug("Page %d has no content", pageNum)
		return "", nil
	}

	// Convert PDF content stream to readable text
	text := p.parseContentStreamToText(string(content))

	pdfLoggerPdfcpu.Debug("Page %d extracted text length: %d characters", pageNum, len(text))

	// If we didn't extract any text, try a simpler approach
	if text == "" {
		pdfLoggerPdfcpu.Debug("No text extracted using content stream parsing, trying fallback for page %d", pageNum)
		text = p.fallbackTextExtraction(string(content))
	}

	return text, nil
}

// fallbackTextExtraction provides a simpler text extraction fallback
func (p *FileParserPDFPdfcpu) fallbackTextExtraction(content string) string {
	var result strings.Builder

	// Look for any text in parentheses throughout the content
	inParens := false
	escaped := false

	for _, char := range content {
		if escaped {
			if char == 'n' {
				result.WriteRune('\n')
			} else if char == 'r' {
				result.WriteRune('\r')
			} else if char == 't' {
				result.WriteRune('\t')
			} else {
				result.WriteRune(char)
			}
			escaped = false
			continue
		}

		switch char {
		case '\\':
			if inParens {
				escaped = true
			}
		case '(':
			inParens = true
		case ')':
			if inParens {
				inParens = false
				result.WriteRune(' ')
			}
		default:
			if inParens && char >= 32 && char <= 126 { // printable ASCII
				result.WriteRune(char)
			}
		}
	}

	text := result.String()

	// Clean up whitespace
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	return strings.Join(words, " ")
}

// parseContentStreamToText extracts readable text from PDF content stream
func (p *FileParserPDFPdfcpu) parseContentStreamToText(content string) string {
	var textBuilder strings.Builder

	// Split content into lines for processing
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for text showing operators: Tj, TJ, '
		if strings.Contains(line, " Tj") || strings.Contains(line, " TJ") || strings.Contains(line, "'") {
			// Extract text between parentheses or brackets
			text := p.extractTextFromOperator(line)
			if text != "" {
				textBuilder.WriteString(text)
				textBuilder.WriteString(" ")
			}
		}

		// Look for simple text patterns in parentheses
		if strings.Contains(line, "(") && strings.Contains(line, ")") {
			text := p.extractTextFromParentheses(line)
			if text != "" {
				textBuilder.WriteString(text)
				textBuilder.WriteString(" ")
			}
		}
	}

	result := textBuilder.String()

	// Clean up the text
	result = strings.ReplaceAll(result, "\\n", "\n")
	result = strings.ReplaceAll(result, "\\r", "\r")
	result = strings.ReplaceAll(result, "\\t", "\t")
	result = strings.ReplaceAll(result, "\\(", "(")
	result = strings.ReplaceAll(result, "\\)", ")")
	result = strings.ReplaceAll(result, "\\\\", "\\")

	// Normalize whitespace
	words := strings.Fields(result)
	if len(words) == 0 {
		return ""
	}

	return strings.Join(words, " ")
}

// extractTextFromOperator extracts text from PDF text showing operators
func (p *FileParserPDFPdfcpu) extractTextFromOperator(line string) string {
	// Handle Tj operator: (text) Tj
	if strings.Contains(line, " Tj") {
		return p.extractTextFromParentheses(line)
	}

	// Handle TJ operator: [(text)] TJ or [(text1)(text2)] TJ
	if strings.Contains(line, " TJ") {
		start := strings.Index(line, "[")
		end := strings.LastIndex(line, "]")
		if start >= 0 && end > start {
			arrayContent := line[start+1 : end]
			return p.extractTextFromArray(arrayContent)
		}
	}

	// Handle ' operator: (text) '
	if strings.HasSuffix(line, "'") {
		return p.extractTextFromParentheses(line)
	}

	return ""
}

// extractTextFromParentheses extracts text from within parentheses
func (p *FileParserPDFPdfcpu) extractTextFromParentheses(line string) string {
	var result strings.Builder
	inText := false
	escaped := false

	for _, char := range line {
		if escaped {
			result.WriteRune(char)
			escaped = false
			continue
		}

		switch char {
		case '\\':
			escaped = true
		case '(':
			if !inText {
				inText = true
			} else {
				result.WriteRune(char)
			}
		case ')':
			if inText {
				inText = false
				result.WriteString(" ")
			}
		default:
			if inText {
				result.WriteRune(char)
			}
		}
	}

	return result.String()
}

// extractTextFromArray extracts text from PDF array format
func (p *FileParserPDFPdfcpu) extractTextFromArray(arrayContent string) string {
	var result strings.Builder

	// Simple approach: extract all text within parentheses in the array
	parts := strings.Split(arrayContent, "(")
	for i, part := range parts {
		if i == 0 {
			continue // Skip first empty part
		}

		endIdx := strings.Index(part, ")")
		if endIdx >= 0 {
			text := part[:endIdx]
			result.WriteString(text)
			result.WriteString(" ")
		}
	}

	return result.String()
}

// extractFontMetadata extracts font information from the PDF context
func (p *FileParserPDFPdfcpu) extractFontMetadata(ctx *model.Context, doc *document.Document) {
	// Use pdfcpu's font extraction capabilities
	fonts := make([]string, 0)

	// Try to extract fonts from all pages
	for pageNum := 1; pageNum <= ctx.PageCount; pageNum++ {
		pageFonts, err := pdfcpu.ExtractPageFonts(ctx, pageNum, nil, nil)
		if err != nil {
			pdfLoggerPdfcpu.Debug("Failed to extract fonts from page %d: %v", pageNum, err)
			continue
		}

		for _, font := range pageFonts {
			fontName := font.Name
			if fontName != "" && !contains(fonts, fontName) {
				fonts = append(fonts, fontName)
			}
		}
	}

	// If no fonts found, add a default
	if len(fonts) == 0 {
		fonts = append(fonts, "Unknown")
	}

	// Set font metadata
	doc.CustomMeta["fonts_used"] = fonts
	doc.CustomMeta["unique_font_count"] = len(fonts)
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
