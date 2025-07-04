package document

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentConstants(t *testing.T) {
	// Test that constants have expected values
	assert.Equal(t, 0, DocumentOriginLocalFile)
	assert.Equal(t, 1, DocumentOriginUrl)
	assert.Equal(t, 2, DocumentOriginUploadedFile)
}

func TestNewDocument(t *testing.T) {
	doc := NewDocument()

	// Verify document is not nil
	require.NotNil(t, doc)

	// Verify CustomMeta is initialized as empty map
	assert.NotNil(t, doc.CustomMeta)
	assert.Empty(t, doc.CustomMeta)

	// Verify other fields have zero values
	assert.Equal(t, 0, doc.OriginType)
	assert.Equal(t, "", doc.OriginLocation)
	assert.Equal(t, "", doc.OriginName)
	assert.Equal(t, "", doc.DocumentType)
	assert.Equal(t, "", doc.FileChecksum)
	assert.Equal(t, time.Time{}, doc.FileCreationTime)
	assert.Equal(t, time.Time{}, doc.FileUpdateTime)
	assert.Equal(t, uuid.Nil, doc.TenantID)
	assert.Equal(t, uuid.Nil, doc.UserID)
	assert.Nil(t, doc.PageTextContent)
	assert.Equal(t, int64(0), doc.FileSize)
	assert.Equal(t, 0, doc.ContentSize)
	assert.Equal(t, 0, doc.TotalPagesCount)
	assert.Equal(t, 0, doc.NonEmptyPagesCount)
	assert.Equal(t, "", doc.MimeType)
	assert.Equal(t, "", doc.ParserName)
}

func TestSetCustomMeta(t *testing.T) {
	doc := NewDocument()

	t.Run("Valid JSON", func(t *testing.T) {
		jsonData := []byte(`{"key1": "value1", "key2": 42, "key3": true}`)
		err := doc.SetCustomMeta(jsonData)

		assert.NoError(t, err)
		assert.Equal(t, "value1", doc.CustomMeta["key1"])
		assert.Equal(t, float64(42), doc.CustomMeta["key2"]) // JSON numbers become float64
		assert.Equal(t, true, doc.CustomMeta["key3"])
	})

	t.Run("Empty JSON Object", func(t *testing.T) {
		doc := NewDocument()
		jsonData := []byte(`{}`)
		err := doc.SetCustomMeta(jsonData)

		assert.NoError(t, err)
		assert.Empty(t, doc.CustomMeta)
	})

	t.Run("Nested JSON", func(t *testing.T) {
		doc := NewDocument()
		jsonData := []byte(`{"nested": {"inner": "value"}, "array": [1, 2, 3]}`)
		err := doc.SetCustomMeta(jsonData)

		assert.NoError(t, err)
		nested := doc.CustomMeta["nested"].(map[string]interface{})
		assert.Equal(t, "value", nested["inner"])

		array := doc.CustomMeta["array"].([]interface{})
		assert.Len(t, array, 3)
		assert.Equal(t, float64(1), array[0])
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		doc := NewDocument()
		jsonData := []byte(`{invalid json}`)
		err := doc.SetCustomMeta(jsonData)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})

	t.Run("Null JSON", func(t *testing.T) {
		doc := NewDocument()
		jsonData := []byte(`null`)
		err := doc.SetCustomMeta(jsonData)

		assert.NoError(t, err)
		assert.Nil(t, doc.CustomMeta)
	})
}

func TestGetCustomMetaJSON(t *testing.T) {
	t.Run("Empty CustomMeta", func(t *testing.T) {
		doc := NewDocument()
		jsonData, err := doc.GetCustomMetaJSON()

		assert.NoError(t, err)
		assert.Equal(t, `{}`, string(jsonData))
	})

	t.Run("With CustomMeta Data", func(t *testing.T) {
		doc := NewDocument()
		doc.CustomMeta["key1"] = "value1"
		doc.CustomMeta["key2"] = 42
		doc.CustomMeta["key3"] = true

		jsonData, err := doc.GetCustomMetaJSON()

		assert.NoError(t, err)

		// Parse back to verify content
		var parsed map[string]interface{}
		err = json.Unmarshal(jsonData, &parsed)
		assert.NoError(t, err)
		assert.Equal(t, "value1", parsed["key1"])
		assert.Equal(t, float64(42), parsed["key2"])
		assert.Equal(t, true, parsed["key3"])
	})

	t.Run("Nil CustomMeta", func(t *testing.T) {
		doc := &Document{CustomMeta: nil}
		jsonData, err := doc.GetCustomMetaJSON()

		assert.NoError(t, err)
		assert.Equal(t, `null`, string(jsonData))
	})

	t.Run("Complex CustomMeta", func(t *testing.T) {
		doc := NewDocument()
		doc.CustomMeta["nested"] = map[string]interface{}{
			"inner":  "value",
			"number": 123,
		}
		doc.CustomMeta["array"] = []interface{}{1, "two", true}

		jsonData, err := doc.GetCustomMetaJSON()

		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), `"nested"`)
		assert.Contains(t, string(jsonData), `"array"`)
	})
}

func TestSetNonEmptyPages(t *testing.T) {
	t.Run("No Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.SetNonEmptyPages()

		assert.Equal(t, 0, doc.NonEmptyPagesCount)
	})

	t.Run("All Empty Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"", "", ""}
		doc.SetNonEmptyPages()

		assert.Equal(t, 0, doc.NonEmptyPagesCount)
	})

	t.Run("All Non-Empty Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"page1", "page2", "page3"}
		doc.SetNonEmptyPages()

		assert.Equal(t, 3, doc.NonEmptyPagesCount)
	})

	t.Run("Mixed Empty and Non-Empty Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"page1", "", "page3", "", "page5"}
		doc.SetNonEmptyPages()

		assert.Equal(t, 3, doc.NonEmptyPagesCount)
	})

	t.Run("Whitespace Only Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"content", "   ", "\t\n", "more content"}
		doc.SetNonEmptyPages()

		// Note: current implementation counts whitespace-only as non-empty
		assert.Equal(t, 2, doc.NonEmptyPagesCount)
	})

	t.Run("Single Character Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"a", "", "b", "", "c"}
		doc.SetNonEmptyPages()

		assert.Equal(t, 3, doc.NonEmptyPagesCount)
	})
}

func TestGetTextContent(t *testing.T) {
	t.Run("No Pages", func(t *testing.T) {
		doc := NewDocument()
		content := doc.GetTextContent()

		assert.Equal(t, "", content)
	})

	t.Run("Empty PageTextContent Slice", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{}
		content := doc.GetTextContent()

		assert.Equal(t, "", content)
	})

	t.Run("Single Page", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"This is page 1 content"}
		content := doc.GetTextContent()

		assert.Equal(t, "This is page 1 content", content)
	})

	t.Run("Multiple Pages", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"Page 1", "Page 2", "Page 3"}
		content := doc.GetTextContent()

		assert.Equal(t, "Page 1\nPage 2\nPage 3", content)
	})

	t.Run("Pages with Empty Content", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"Page 1", "", "Page 3"}
		content := doc.GetTextContent()

		assert.Equal(t, "Page 1\n\nPage 3", content)
	})

	t.Run("Pages with Newlines", func(t *testing.T) {
		doc := NewDocument()
		doc.PageTextContent = []string{"Line 1\nLine 2", "Page 2 content"}
		content := doc.GetTextContent()

		assert.Equal(t, "Line 1\nLine 2\nPage 2 content", content)
	})

	t.Run("Large Number of Pages", func(t *testing.T) {
		doc := NewDocument()
		pages := make([]string, 100)
		for i := 0; i < 100; i++ {
			pages[i] = fmt.Sprintf("Page %d content", i+1)
		}
		doc.PageTextContent = pages
		content := doc.GetTextContent()

		lines := strings.Split(content, "\n")
		assert.Len(t, lines, 100)
		assert.Equal(t, "Page 1 content", lines[0])
		assert.Equal(t, "Page 100 content", lines[99])
	})
}

func TestDocumentJSONSerialization(t *testing.T) {
	t.Run("Full Document Serialization", func(t *testing.T) {
		doc := NewDocument()
		doc.OriginType = DocumentOriginLocalFile
		doc.OriginLocation = "/path/to/file.pdf"
		doc.OriginName = "file.pdf"
		doc.DocumentType = "pdf"
		doc.FileChecksum = "abc123"
		doc.FileCreationTime = time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		doc.FileUpdateTime = time.Date(2023, 1, 2, 12, 0, 0, 0, time.UTC)
		doc.TenantID = uuid.MustParse("12345678-1234-1234-1234-123456789012")
		doc.UserID = uuid.MustParse("87654321-4321-4321-4321-210987654321")
		doc.PageTextContent = []string{"Page 1", "Page 2"}
		doc.FileSize = 1024
		doc.ContentSize = 512
		doc.TotalPagesCount = 2
		doc.NonEmptyPagesCount = 2
		doc.MimeType = "application/pdf"
		doc.ParserName = "test-parser"
		doc.CustomMeta["custom"] = "value"

		jsonData, err := json.Marshal(doc)
		assert.NoError(t, err)

		var unmarshaled Document
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)

		assert.Equal(t, doc.OriginType, unmarshaled.OriginType)
		assert.Equal(t, doc.OriginLocation, unmarshaled.OriginLocation)
		assert.Equal(t, doc.OriginName, unmarshaled.OriginName)
		assert.Equal(t, doc.DocumentType, unmarshaled.DocumentType)
		assert.Equal(t, doc.FileChecksum, unmarshaled.FileChecksum)
		assert.True(t, doc.FileCreationTime.Equal(unmarshaled.FileCreationTime))
		assert.True(t, doc.FileUpdateTime.Equal(unmarshaled.FileUpdateTime))
		assert.Equal(t, doc.TenantID, unmarshaled.TenantID)
		assert.Equal(t, doc.UserID, unmarshaled.UserID)
		assert.Equal(t, doc.PageTextContent, unmarshaled.PageTextContent)
		assert.Equal(t, doc.FileSize, unmarshaled.FileSize)
		assert.Equal(t, doc.ContentSize, unmarshaled.ContentSize)
		assert.Equal(t, doc.TotalPagesCount, unmarshaled.TotalPagesCount)
		assert.Equal(t, doc.NonEmptyPagesCount, unmarshaled.NonEmptyPagesCount)
		assert.Equal(t, doc.MimeType, unmarshaled.MimeType)
		assert.Equal(t, doc.ParserName, unmarshaled.ParserName)
		assert.Equal(t, doc.CustomMeta["custom"], unmarshaled.CustomMeta["custom"])
	})
}

func TestDocumentOriginTypeValues(t *testing.T) {
	doc := NewDocument()

	// Test setting different origin types
	doc.OriginType = DocumentOriginLocalFile
	assert.Equal(t, 0, doc.OriginType)

	doc.OriginType = DocumentOriginUrl
	assert.Equal(t, 1, doc.OriginType)

	doc.OriginType = DocumentOriginUploadedFile
	assert.Equal(t, 2, doc.OriginType)
}
