package file_parser

import (
	"context"
	"io"
	"strings"

	"github.com/gomutex/godocx/docx"
	"github.com/gomutex/godocx/packager"
	"github.com/gomutex/godocx/wml/ctypes"

	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
)

const maxTableDepth = 12 // maximum depth of nested tables

var docxLogger = logging.NewLogger(logging.InfoLevel, "", logging.DebugLevel, 0, 0, 0)

type FileParserDOCX struct {
	styleCache map[string]bool
	docxDoc    *docx.RootDoc
}

var _ FileParser = (*FileParserDOCX)(nil)

func NewDOCXParser() *FileParserDOCX {
	return &FileParserDOCX{styleCache: make(map[string]bool)}
}

func (p *FileParserDOCX) resetCache() {
	p.styleCache = make(map[string]bool)
}

func (p *FileParserDOCX) Parse(ctx context.Context, reader io.Reader, doc *document.Document) errorx.Error {
	docxLogger.Info("Starting DOCX document parsing")

	data, err := io.ReadAll(reader)
	if err != nil {
		return errorx.NewErrInternalServerError("Failed to read DOCX content: %w", err)
	}

	docxDoc, err := packager.Unpack(&data)
	if err != nil {
		return errorx.NewErrInternalServerError("Failed to unpack DOCX: %w", err)
	}

	p.docxDoc = docxDoc
	p.resetCache()

	doc.DocumentType = "docx"
	doc.ParserName = "embedded~gomutex-godocx"
	doc.MimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	doc.CustomMeta = make(map[string]interface{})

	p.extractDocumentContent(doc, 0)

	doc.ContentSize = 0
	for _, txt := range doc.PageTextContent {
		doc.ContentSize += len(txt)
	}

	docxLogger.Debug("Parsed DOCX pages=%d size=%d", doc.TotalPagesCount, doc.ContentSize)
	return nil
}

func (p *FileParserDOCX) collectStyleIDs() []string {
	var ids []string
	for _, s := range p.docxDoc.DocStyles.StyleList {
		if s.ID != nil && s.Type != nil && *s.Type == "paragraph" {
			ids = append(ids, *s.ID)
		}
	}
	if len(ids) == 0 {
		return []string{"Normal", "Heading1", "Heading2"}
	}
	return ids
}

func (p *FileParserDOCX) extractDocumentContent(doc *document.Document, depth int) {
	content := p.extractContent(p.docxDoc, depth)
	doc.PageTextContent = []string{content.MergedText}
	doc.TotalPagesCount = 1 // Unable to determine total pages count wiithout rendering
	doc.SetNonEmptyPages()

	if p.docxDoc.DocStyles != nil {
		ids := p.collectStyleIDs()
		doc.CustomMeta["styles"] = ids
		doc.CustomMeta["styles_count"] = len(ids)
	}

	doc.CustomMeta["paragraph_count"] = content.Paragraphs
	doc.CustomMeta["total_body_items"] = content.TotalItems
	doc.CustomMeta["table_count"] = content.Tables
}

type docContent struct {
	MergedText string
	Paragraphs int
	Tables     int
	TotalItems int
}

func (p *FileParserDOCX) extractContent(docxDoc *docx.RootDoc, depth int) docContent {
	var content docContent
	var b strings.Builder
	lastWasHeading := false

	if docxDoc.Document != nil && docxDoc.Document.Body != nil {
		for _, child := range docxDoc.Document.Body.Children {
			var txt string
			if child.Para != nil {
				txt = strings.TrimSpace(p.extractTextFromParagraph(child.Para))
				if txt != "" {
					isHeading := p.isHeadingParagraphStruct(child.Para)
					if b.Len() > 0 {
						if lastWasHeading {
							b.WriteString("\n")
						} else {
							b.WriteString("\n\n")
						}
					}
					lastWasHeading = isHeading
					content.Paragraphs++
				}
			} else if child.Table != nil {
				txt = p.extractTextFromTable(child.Table, depth+1)
				if txt != "" {
					if b.Len() > 0 {
						b.WriteString("\n\n")
					}
					lastWasHeading = false
					content.Tables++
				}
			}
			b.WriteString(txt)
			content.TotalItems++
		}
	}
	content.MergedText = b.String()
	return content
}

func escapeMarkdownCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

func (p *FileParserDOCX) extractTextFromTable(tbl *docx.Table, depth int) string {
	return p.extractTextFromCTable(tbl.GetCT(), depth)
}

func (p *FileParserDOCX) extractTextFromCTable(ct *ctypes.Table, depth int) string {
	if depth > maxTableDepth {
		panic("[Table depth limit exceeded]")
	}
	var rows [][]string
	for _, rc := range ct.RowContents {
		row := rc.Row
		var cells []string
		for _, cc := range row.Contents {
			cell := cc.Cell
			var parts []string
			for _, block := range cell.Contents {
				if block.Paragraph != nil {
					parts = append(parts, p.extractTextFromCTPara(block.Paragraph))
				} else if block.Table != nil {
					parts = append(parts, p.extractTextFromCTable(block.Table, depth+1))
				}
			}
			cells = append(cells, strings.Join(parts, " "))
		}
		rows = append(rows, cells)
	}

	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	// Header row
	b.WriteString("|")
	for _, cell := range rows[0] {
		b.WriteString(escapeMarkdownCell(strings.TrimSpace(cell)) + "|")
	}
	b.WriteString("\n")
	// Separator row
	b.WriteString("|")
	for range rows[0] {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	// Data rows
	for _, row := range rows[1:] {
		b.WriteString("|")
		for _, cell := range row {
			b.WriteString(escapeMarkdownCell(strings.TrimSpace(cell)) + "|")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (p *FileParserDOCX) extractTextFromCTPara(ctPara *ctypes.Paragraph) string {
	var b strings.Builder
	for _, child := range ctPara.Children {
		if child.Run != nil {
			for _, rc := range child.Run.Children {
				if rc.Text != nil {
					b.WriteString(rc.Text.Text)
				}
			}
		}
		if child.Link != nil {
			for _, linkChild := range child.Link.Children {
				if linkChild.Run != nil {
					for _, rc := range linkChild.Run.Children {
						if rc.Text != nil {
							b.WriteString(rc.Text.Text)
						}
					}
				}
			}
		}
	}
	return b.String()
}

func (p *FileParserDOCX) extractTextFromParagraph(para *docx.Paragraph) string {
	if para == nil || para.GetCT() == nil {
		return ""
	}
	var b strings.Builder
	for _, run := range para.GetCT().Children {
		if run.Run != nil {
			for _, rc := range run.Run.Children {
				if rc.Text != nil {
					b.WriteString(rc.Text.Text)
				}
			}
		}
	}
	return b.String()
}

func (p *FileParserDOCX) isHeadingParagraphStruct(para *docx.Paragraph) bool {
	ct := para.GetCT()
	if ct == nil {
		return false
	}
	if ct.Property != nil && ct.Property.OutlineLvl != nil {
		lvl := ct.Property.OutlineLvl.Val
		if lvl >= 0 && lvl <= 8 {
			return true
		}
	}
	if ct.Property != nil && ct.Property.Style != nil {
		return p.isHeadingStyleId(ct.Property.Style.Val)
	}
	return false
}

func (p *FileParserDOCX) isHeadingStyleId(styleId string) bool {
	if styleId == "" || p.docxDoc == nil || p.docxDoc.DocStyles == nil {
		return false
	}
	if _, ok := p.styleCache[styleId]; ok {
		return true
	}
	found := false
	for _, s := range p.docxDoc.DocStyles.StyleList {
		if s.ID != nil && *s.ID == styleId && s.Type != nil && *s.Type == "paragraph" {
			if s.ParaProp != nil && s.ParaProp.OutlineLvl != nil {
				found = true
				break
			}
			if s.Name != nil && p.isHeadingStyleName(s.Name.Val) {
				found = true
				break
			}
		}
	}
	if found {
		p.styleCache[styleId] = true
	}
	return found
}

func (p *FileParserDOCX) isHeadingStyleName(name string) bool {
	if name == "" {
		return false
	}
	ln := strings.ToLower(name)
	return strings.HasPrefix(ln, "heading") || strings.HasPrefix(ln, "h1") || strings.HasPrefix(ln, "h2")
}
