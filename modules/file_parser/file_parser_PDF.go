package file_parser

import (
	"context"
	"errors"
	"io"

	"github.com/Artifizer/pdf"
	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var pdfLogger = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

// FileParserPDF implements the FileParser interface for PDF files
type FileParserPDF struct{}

// Ensure FileParserPDF implements FileParser interface
var _ FileParser = (*FileParserPDF)(nil)

// NewPDFParser creates a new instance of FileParserPDF
func NewPDFParser() *FileParserPDF {
	return &FileParserPDF{}
}

// Parse implements the FileParser interface for PDF files
func (p *FileParserPDF) Parse(ctx context.Context, reader io.Reader, doc *document.Document) errorx.Error {
	pdfLogger.Debug("Parsing PDF content")

	// Convert reader to ReaderAt if needed
	readerAt, size, err := p.ensureReaderAt(reader)
	if err != nil {
		return errorx.NewErrBadRequest("failed to prepare PDF reader: %w", err)
	}

	// Open PDF from reader
	r, err := pdf.NewReader(readerAt, size)
	if err != nil {
		return errorx.NewErrBadRequest("failed to open PDF: %w", err)
	}

	// Set basic document properties
	doc.DocumentType = "pdf"
	doc.CustomMeta = make(map[string]interface{})
	doc.ParserName = "embedded~ledongthuc-pdf"
	doc.MimeType = "application/pdf"

	// Extract content and metadata
	p.extractPageContent(r, doc)
	p.extractStyledTextMetadata(r, doc)

	totalPages := r.NumPage()
	pdfLogger.Debug("Successfully parsed PDF content (pages: %d, content size: %d)",
		totalPages, doc.ContentSize)

	doc.ContentSize = 0
	for _, pageText := range doc.PageTextContent {
		doc.ContentSize += len(pageText)
	}

	return nil
}

// ensureReaderAt converts an io.Reader to io.ReaderAt if needed
func (p *FileParserPDF) ensureReaderAt(reader io.Reader) (io.ReaderAt, int64, error) {
	// If it's already a ReaderAt, use it directly
	if readerAt, ok := reader.(io.ReaderAt); ok {
		// Try to get size if it's also a Seeker
		if seeker, ok := reader.(io.Seeker); ok {
			size, err := seeker.Seek(0, io.SeekEnd)
			if err != nil {
				return nil, 0, err
			}
			_, err = seeker.Seek(0, io.SeekStart)
			if err != nil {
				return nil, 0, err
			}
			return readerAt, size, nil
		}
		// If we can't determine size, we'll need to read all content anyway
	}

	// Otherwise, read all content into memory and create a ReaderAt
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, err
	}

	return &readerAt{content: content}, int64(len(content)), nil
}

// readerAt implements io.ReaderAt for in-memory content
type readerAt struct {
	content []byte
}

func (ra *readerAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}

	if off >= int64(len(ra.content)) {
		return 0, io.EOF
	}

	n = copy(p, ra.content[off:])
	if n < len(p) {
		err = io.EOF
	}

	return n, err
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
