package document

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Document origin types
const (
	DocumentOriginLocalFile = iota
	DocumentOriginUrl
	DocumentOriginUploadedFile
)

// Document represents a parsed document with all its metadata
type Document struct {
	OriginType         int                    `json:"origin_type"`
	OriginLocation     string                 `json:"origin_location"`
	OriginName         string                 `json:"origin_name"`
	DocumentType       string                 `json:"document_type"`
	FileChecksum       string                 `json:"file_checksum"`
	FileCreationTime   time.Time              `json:"file_creation_time"`
	FileUpdateTime     time.Time              `json:"file_update_time"`
	TenantID           uuid.UUID              `json:"tenant_id"`
	UserID             uuid.UUID              `json:"user_id"`
	PageTextContent    []string               `json:"page_text_content"`
	FileSize           int64                  `json:"file_size"`
	ContentSize        int                    `json:"content_size"`
	TotalPagesCount    int                    `json:"total_pages_count"`
	NonEmptyPagesCount int                    `json:"non_empty_pages_count"`
	MimeType           string                 `json:"mime_type"`
	ParserName         string                 `json:"parser_name"`
	CustomMeta         map[string]interface{} `json:"custom_meta"`
}

// NewDocument creates a new Document instance
func NewDocument() *Document {
	return &Document{
		CustomMeta: make(map[string]interface{}),
	}
}

// SetCustomMeta sets custom metadata from JSON
func (d *Document) SetCustomMeta(jsonData []byte) error {
	return json.Unmarshal(jsonData, &d.CustomMeta)
}

// GetCustomMetaJSON returns custom metadata as JSON
func (d *Document) GetCustomMetaJSON() ([]byte, error) {
	return json.Marshal(d.CustomMeta)
}

// Helper function to count non-empty pages
func (d *Document) SetNonEmptyPages() {
	count := 0
	for _, text := range d.PageTextContent {
		text = strings.TrimSpace(text)
		if len(text) > 0 {
			count++
		}
	}
	d.NonEmptyPagesCount = count
}

func (d *Document) GetTextContent() string {
	if len(d.PageTextContent) == 0 {
		return ""
	}
	return strings.Join(d.PageTextContent, "\n")
}
