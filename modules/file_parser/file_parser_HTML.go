package file_parser

import (
	"context"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var htmlLogger = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

// FileParserHTML implements the FileParser interface for HTML files
type FileParserHTML struct{}

// NewHTMLParser creates a new instance of FileParserHTML
func NewHTMLParser() *FileParserHTML {
	return &FileParserHTML{}
}

// Parse implements the FileParser interface for HTML files
func (p *FileParserHTML) Parse(ctx context.Context, path string, doc *document.Document) errorx.Error {
	htmlLogger.Debug("Parsing HTML file: %s", path)

	// Validate file existence
	if errx := p.validateFile(path); errx != nil {
		return errx
	}

	// Open HTML file
	file, err := os.Open(path)
	if err != nil {
		return errorx.NewErrBadRequest("failed to open HTML file: %w", err)
	}
	defer file.Close()

	// Parse HTML with goquery
	htmlDoc, err := goquery.NewDocumentFromReader(file)
	if err != nil {
		return errorx.NewErrBadRequest("failed to parse HTML file: %w", err)
	}

	// Set basic document properties
	doc.DocumentType = "html"
	doc.CustomMeta = make(map[string]interface{})
	doc.ParserName = "embedded~goquery"
	doc.MimeType = "text/html"

	// Extract content and metadata
	p.extractPageContent(htmlDoc, doc)
	p.extractHTMLMetadata(htmlDoc, doc)

	htmlLogger.Debug("Successfully parsed HTML file: %s (content size: %d)",
		path, doc.ContentSize)

	return nil
}

// validateFile checks if the file exists
func (p *FileParserHTML) validateFile(path string) errorx.Error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return errorx.NewErrBadRequest("file does not exist")
	}
	return nil
}

// extractPageContent extracts clean text content from HTML document
func (p *FileParserHTML) extractPageContent(htmlDoc *goquery.Document, doc *document.Document) {
	// Remove script, style, and other non-content elements
	htmlDoc.Find("script, style, noscript, iframe, object, embed, applet").Remove()

	// Extract text content from body, or entire document if no body
	var textContent string
	body := htmlDoc.Find("body")
	if body.Length() > 0 {
		textContent = p.extractCleanText(body)
	} else {
		textContent = p.extractCleanText(htmlDoc.Selection)
	}

	// Clean up whitespace
	textContent = p.cleanupText(textContent)

	// For HTML, we treat the entire document as one page
	pageTexts := []string{textContent}

	doc.PageTextContent = pageTexts
	doc.TotalPagesCount = 1
	doc.SetNonEmptyPages()

	// Calculate content size
	doc.ContentSize = len(textContent)
}

// extractCleanText extracts clean text from a goquery selection
func (p *FileParserHTML) extractCleanText(selection *goquery.Selection) string {
	var textParts []string

	// Handle different elements with appropriate spacing
	selection.Contents().Each(func(i int, s *goquery.Selection) {
		if goquery.NodeName(s) == "#text" {
			// Direct text node
			text := strings.TrimSpace(s.Text())
			if text != "" {
				textParts = append(textParts, text)
			}
		} else {
			// Element node
			nodeName := goquery.NodeName(s)

			// Skip script, style, and other non-content elements
			if p.isNonContentElement(nodeName) {
				return
			}

			// Add line breaks for block elements before content
			if p.isBlockElement(nodeName) {
				textParts = append(textParts, "\n")
			}

			// Recursively extract text from child elements
			childText := p.extractCleanText(s)
			if childText != "" {
				textParts = append(textParts, childText)
			}

			// Add line breaks for block elements after content
			if p.isBlockElement(nodeName) {
				textParts = append(textParts, "\n")
			} else {
				// Add space for inline elements
				textParts = append(textParts, " ")
			}
		}
	})

	return strings.Join(textParts, "")
}

// isNonContentElement checks if an element should be excluded from text extraction
func (p *FileParserHTML) isNonContentElement(nodeName string) bool {
	nonContentElements := map[string]bool{
		"script":   true,
		"style":    true,
		"noscript": true,
		"iframe":   true,
		"object":   true,
		"embed":    true,
		"applet":   true,
		"svg":      true,
		"canvas":   true,
		"audio":    true,
		"video":    true,
	}
	return nonContentElements[nodeName]
}

// isBlockElement checks if an element is a block-level element
func (p *FileParserHTML) isBlockElement(nodeName string) bool {
	blockElements := map[string]bool{
		"p":          true,
		"div":        true,
		"h1":         true,
		"h2":         true,
		"h3":         true,
		"h4":         true,
		"h5":         true,
		"h6":         true,
		"ul":         true,
		"ol":         true,
		"li":         true,
		"blockquote": true,
		"pre":        true,
		"table":      true,
		"tr":         true,
		"td":         true,
		"th":         true,
		"section":    true,
		"article":    true,
		"header":     true,
		"footer":     true,
		"nav":        true,
		"aside":      true,
		"main":       true,
		"br":         true,
		"hr":         true,
	}
	return blockElements[nodeName]
}

// cleanupText cleans up extracted text by normalizing whitespace
func (p *FileParserHTML) cleanupText(text string) string {
	// Replace multiple whitespace characters with single spaces
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "\r", " ")

	// Normalize multiple spaces to single space
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	// Normalize multiple newlines to single newlines
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}

	// Trim leading/trailing whitespace
	text = strings.TrimSpace(text)

	return text
}

// extractHTMLMetadata extracts comprehensive metadata from HTML document
func (p *FileParserHTML) extractHTMLMetadata(htmlDoc *goquery.Document, doc *document.Document) {
	metadata := make(map[string]interface{})

	// Extract title
	title := htmlDoc.Find("title").First().Text()
	if title != "" {
		metadata["title"] = strings.TrimSpace(title)
	}

	// Extract meta tags
	metaTags := make(map[string]string)
	htmlDoc.Find("meta").Each(func(i int, s *goquery.Selection) {
		name, exists := s.Attr("name")
		if !exists {
			name, exists = s.Attr("property")
		}
		if exists {
			content, contentExists := s.Attr("content")
			if contentExists && content != "" {
				metaTags[name] = content
			}
		}
	})
	if len(metaTags) > 0 {
		metadata["meta_tags"] = metaTags
	}

	// Extract headings structure
	var headings []map[string]string
	htmlDoc.Find("h1, h2, h3, h4, h5, h6").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			headings = append(headings, map[string]string{
				"level": goquery.NodeName(s),
				"text":  text,
			})
		}
	})
	if len(headings) > 0 {
		metadata["headings"] = headings
		metadata["heading_count"] = len(headings)
	}

	// Extract links
	var links []string
	var externalLinks int
	htmlDoc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists && href != "" {
			links = append(links, href)
			if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
				externalLinks++
			}
		}
	})
	if len(links) > 0 {
		metadata["links_count"] = len(links)
		metadata["external_links"] = externalLinks
	}

	// Extract images
	var images []string
	htmlDoc.Find("img[src]").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if exists && src != "" {
			images = append(images, src)
		}
	})
	if len(images) > 0 {
		metadata["images_count"] = len(images)
	}

	// Extract language information
	if lang, exists := htmlDoc.Find("html").Attr("lang"); exists && lang != "" {
		metadata["language"] = lang
	}

	// Extract description from meta description
	description := htmlDoc.Find("meta[name='description']").AttrOr("content", "")
	if description == "" {
		description = htmlDoc.Find("meta[property='og:description']").AttrOr("content", "")
	}
	if description != "" {
		metadata["description"] = description
	}

	// Extract keywords
	keywords := htmlDoc.Find("meta[name='keywords']").AttrOr("content", "")
	if keywords != "" {
		metadata["keywords"] = keywords
	}

	// Extract character encoding
	charset := htmlDoc.Find("meta[charset]").AttrOr("charset", "")
	if charset == "" {
		charset = htmlDoc.Find("meta[http-equiv='Content-Type']").AttrOr("content", "")
		if strings.Contains(charset, "charset=") {
			parts := strings.Split(charset, "charset=")
			if len(parts) > 1 {
				charset = strings.TrimSpace(parts[1])
			}
		}
	}
	if charset != "" {
		metadata["charset"] = charset
	}

	// Set all metadata
	for key, value := range metadata {
		doc.CustomMeta[key] = value
	}
}
