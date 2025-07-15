package file_parser

import (
	"context"
	"os"

	"github.com/Artifizer/pdf"
	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var pdfLogger = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

// FileParserPDF implements the FileParser interface for PDF files
type FileParserPDF struct{}

// NewPDFParser creates a new instance of FileParserPDF
func NewPDFParser() *FileParserPDF {
	return &FileParserPDF{}
}

// Parse implements the FileParser interface for PDF files
func (p *FileParserPDF) Parse(ctx context.Context, path string, doc *document.Document) errorx.Error {
	pdfLogger.Debug("Parsing PDF file: %s", path)

	// Validate file existence
	if errx := p.validateFile(path); errx != nil {
		return errx
	}

	// Open PDF file
	f, r, err := pdf.Open(path)
	if err != nil {
		return errorx.NewErrBadRequest("failed to open PDF file: %w", err)
	}
	defer f.Close()

	// Set basic document properties
	doc.DocumentType = "pdf"
	doc.CustomMeta = make(map[string]interface{})

	doc.ParserName = "embedded~ledongthuc-pdf"
	doc.MimeType = "application/pdf"

	// Extract content and metadata
	p.extractPageContent(r, doc)
	p.extractStyledTextMetadata(r, doc)

	totalPages := r.NumPage()
	pdfLogger.Debug("Successfully parsed PDF file: %s (pages: %d, content size: %d)",
		path, totalPages, doc.ContentSize)

	doc.ContentSize = 0
	for _, pageText := range doc.PageTextContent {
		doc.ContentSize += len(pageText)
	}

	return nil
}

// validateFile checks if the file exists
func (p *FileParserPDF) validateFile(path string) errorx.Error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return errorx.NewErrBadRequest("file does not exist")
	}
	return nil
}

// extractStyledTextMetadata extracts font and styling information
func (p *FileParserPDF) extractStyledTextMetadata(r *pdf.Reader, doc *document.Document) {
	styledTexts, err := r.GetStyledTexts()
	if err != nil {
		pdfLogger.Warn("Failed to extract styled text from PDF: %v", err)
		return
	}

	fontInfo := p.analyzeFontInformation(styledTexts)
	p.setFontMetadata(doc, fontInfo, len(styledTexts))
}

// analyzeFontInformation analyzes font usage in styled text
func (p *FileParserPDF) analyzeFontInformation(styledTexts []pdf.Text) map[string]interface{} {
	fontMap := make(map[string]bool)

	for _, text := range styledTexts {
		if text.Font != "" {
			fontMap[text.Font] = true
		}
	}

	fonts := make([]string, 0, len(fontMap))
	for font := range fontMap {
		fonts = append(fonts, font)
	}

	return map[string]interface{}{
		"fonts":     fonts,
		"fontCount": len(fonts),
	}
}

// setFontMetadata sets font-related metadata in the document
func (p *FileParserPDF) setFontMetadata(doc *document.Document, fontInfo map[string]interface{}, textElementsCount int) {
	doc.CustomMeta["fonts_used"] = fontInfo["fonts"]
	doc.CustomMeta["unique_font_count"] = fontInfo["fontCount"]
	doc.CustomMeta["text_elements_count"] = textElementsCount
}

// extractPageContent extracts text content organized by pages
func (p *FileParserPDF) extractPageContent(r *pdf.Reader, doc *document.Document) {
	totalPages := r.NumPage()
	pageTexts := make([]string, 0, totalPages)

	for pageIndex := 1; pageIndex <= totalPages; pageIndex++ {
		pageText := p.extractSinglePageText(r, pageIndex)
		pageTexts = append(pageTexts, pageText)
	}

	doc.PageTextContent = pageTexts
	doc.TotalPagesCount = totalPages
	doc.SetNonEmptyPages()
}

// extractSinglePageText extracts text content from a single page
func (p *FileParserPDF) extractSinglePageText(r *pdf.Reader, pageIndex int) string {
	page := r.Page(pageIndex)
	if page.V.IsNull() {
		return ""
	}

	// Use GetPlainText on the page for clean text extraction
	pageText, err := page.GetPlainText(nil)
	if err != nil {
		pdfLogger.Warn("Failed to extract plain text for page %d: %v", pageIndex, err)
		return ""
	}

	return pageText
}
