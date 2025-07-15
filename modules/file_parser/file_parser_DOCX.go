package file_parser

import (
	"context"
	"os"
	"strings"

	"github.com/gomutex/godocx"
	"github.com/gomutex/godocx/docx"
	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var docxLogger = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

// FileParserDOCX implements the FileParser interface for DOCX files
type FileParserDOCX struct{}

// NewDOCXParser creates a new instance of FileParserDOCX
func NewDOCXParser() *FileParserDOCX {
	return &FileParserDOCX{}
}

// Parse implements the FileParser interface for DOCX files
func (p *FileParserDOCX) Parse(ctx context.Context, path string, doc *document.Document) errorx.Error {
	docxLogger.Debug("Parsing DOCX file: %s", path)

	// Validate file existence
	if errx := p.validateFile(path); errx != nil {
		return errx
	}

	// Open DOCX file
	docxDoc, err := godocx.OpenDocument(path)
	if err != nil {
		return errorx.NewErrBadRequest("failed to open DOCX file: %w", err)
	}

	// Set basic document properties
	doc.DocumentType = "docx"
	doc.CustomMeta = make(map[string]interface{})

	doc.ParserName = "embedded~gomutex-godocx"
	doc.MimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	// Extract content and metadata
	p.extractPageContent(docxDoc, doc)
	p.extractDocumentMetadata(docxDoc, doc)

	totalPages := len(doc.PageTextContent)
	docxLogger.Debug("Successfully parsed DOCX file: %s (pages: %d, content size: %d)",
		path, totalPages, doc.ContentSize)

	doc.ContentSize = 0
	for _, pageText := range doc.PageTextContent {
		doc.ContentSize += len(pageText)
	}

	return nil
}

// validateFile checks if the file exists
func (p *FileParserDOCX) validateFile(path string) errorx.Error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return errorx.NewErrBadRequest("file does not exist")
	}
	return nil
}

// extractDocumentMetadata extracts metadata from the DOCX document
func (p *FileParserDOCX) extractDocumentMetadata(docxDoc *docx.RootDoc, doc *document.Document) {
	// Extract document properties if available
	properties := make(map[string]interface{})

	// Extract styles information if available
	if docxDoc.DocStyles != nil {
		styles := p.extractStylesInformation(docxDoc)
		if len(styles) > 0 {
			properties["styles"] = styles
			properties["styles_count"] = len(styles)
		}
	}

	// Extract paragraph count
	paragraphCount := p.countParagraphs(docxDoc)
	properties["paragraph_count"] = paragraphCount

	// Set metadata in the document
	for k, v := range properties {
		doc.CustomMeta[k] = v
	}
}

// extractStylesInformation extracts style information from the document
func (p *FileParserDOCX) extractStylesInformation(docxDoc *docx.RootDoc) []string {
	// Extract style IDs if available
	styles := []string{}

	if docxDoc.DocStyles != nil && docxDoc.DocStyles.StyleList != nil {
		for _, style := range docxDoc.DocStyles.StyleList {
			if style.ID != nil {
				styles = append(styles, *style.ID)
			}
		}
	}

	if len(styles) == 0 {
		// Return default styles if none found
		return []string{"Normal", "Heading1", "Heading2"}
	}

	return styles
}

// countParagraphs counts the number of paragraphs in the document
func (p *FileParserDOCX) countParagraphs(docxDoc *docx.RootDoc) int {
	// Count paragraphs in the document
	count := 0

	// If document structure is available, count paragraphs
	if docxDoc.Document != nil && docxDoc.Document.Body != nil {
		// This is a simplified implementation
		// In a real implementation, you would traverse the document structure
		// and count all paragraphs, including those in tables, etc.

		// For now, we'll use a basic approach based on the extracted text
		content := p.extractAllText(docxDoc)
		// Rough estimate: count newlines as paragraph separators
		count = len(strings.Split(content, "\n"))
	}

	return count
}

// extractPageContent extracts text content organized by pages
func (p *FileParserDOCX) extractPageContent(docxDoc *docx.RootDoc, doc *document.Document) {
	// Extract all text content
	content := p.extractAllText(docxDoc)

	// In DOCX, page breaks are not explicitly marked in the XML structure
	// For simplicity, we'll treat the entire document as a single page
	// In a more sophisticated implementation, we could try to detect page breaks
	pageTexts := []string{content}

	doc.PageTextContent = pageTexts
	doc.TotalPagesCount = len(pageTexts)
	doc.SetNonEmptyPages()
}

// extractAllText extracts all text content from the document
func (p *FileParserDOCX) extractAllText(docxDoc *docx.RootDoc) string {
	var builder strings.Builder

	// If document structure is available, extract text
	if docxDoc.Document != nil && docxDoc.Document.Body != nil {
		// This is a simplified implementation
		// In a real implementation, you would traverse the document structure
		// and extract text from paragraphs, tables, etc.

		// Extract text from paragraphs in the body
		for _, child := range docxDoc.Document.Body.Children {
			// Check if the child contains a paragraph
			if child.Para != nil {
				// Extract text from the paragraph using the GetCT() method
				paraCT := child.Para.GetCT()
				if paraCT != nil && paraCT.Children != nil {
					for _, paraChild := range paraCT.Children {
						if paraChild.Run != nil && len(paraChild.Run.Children) > 0 {
							for _, runChild := range paraChild.Run.Children {
								if runChild.Text != nil {
									builder.WriteString(runChild.Text.Text)
								}
							}
						}
					}
				}
				builder.WriteString("\n")
			}

			// TODO: Add support for tables and other content types
		}
	}

	return builder.String()
}
