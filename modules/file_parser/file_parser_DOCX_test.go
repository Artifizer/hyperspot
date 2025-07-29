package file_parser

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type docxTestCase struct {
	name              string
	fileName          string
	wantContentFile   string
	wantDocType       string
	wantParserName    string
	wantMimeType      string
	wantPagesCount    int
	wantNonEmptyPages int
	wantContentSizeGT int
	wantCustomMeta    map[string]any
}

func TestFileParserDOCX_Parse(t *testing.T) {
	testCases := []docxTestCase{
		{
			name:              "Parse valid multilingual DOCX file",
			fileName:          "test_file_2pages_multilingual.docx",
			wantContentFile:   "test_file_2pages_multilingual.txt",
			wantDocType:       "docx",
			wantParserName:    "embedded~gomutex-godocx",
			wantMimeType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantPagesCount:    1,
			wantNonEmptyPages: 1,
			wantContentSizeGT: 0,
			wantCustomMeta: map[string]any{
				"paragraph_count":  44,
				"total_body_items": 50,
				"table_count":      0,
			},
		},
		{
			name:              "Parse valid DOCX file with table",
			fileName:          "test_file_1table_multilingual.docx",
			wantContentFile:   "test_file_1table_multilingual.txt",
			wantDocType:       "docx",
			wantParserName:    "embedded~gomutex-godocx",
			wantMimeType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantPagesCount:    1,
			wantNonEmptyPages: 1,
			wantContentSizeGT: 0,
			wantCustomMeta: map[string]any{
				"total_body_items": 2,
				"table_count":      1,
			},
		},
		{
			name:              "Parse valid DOCX file with image and table",
			fileName:          "test_file_1table_image_multilingual.docx",
			wantContentFile:   "test_file_1table_image_multilingual.txt",
			wantDocType:       "docx",
			wantParserName:    "embedded~gomutex-godocx",
			wantMimeType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantPagesCount:    1,
			wantNonEmptyPages: 1,
			wantContentSizeGT: 0,
			wantCustomMeta: map[string]any{
				"total_body_items": 13,
				"table_count":      1,
			},
		},
		{
			name:              "Parse valid large real-life DOCX file",
			fileName:          "test_file_big_english.docx",
			wantContentFile:   "test_file_big_english.txt",
			wantDocType:       "docx",
			wantParserName:    "embedded~gomutex-godocx",
			wantMimeType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantPagesCount:    1,
			wantNonEmptyPages: 1,
			wantContentSizeGT: 0,
			wantCustomMeta: map[string]any{
				"total_body_items": 252,
				"paragraph_count":  228,
				"table_count":      0,
			},
		},
		{
			name:              "Parse valid DOCX file with edge cases",
			fileName:          "test_file_1table_edge_cases.docx",
			wantContentFile:   "test_file_1table_edge_cases.txt",
			wantDocType:       "docx",
			wantParserName:    "embedded~gomutex-godocx",
			wantMimeType:      "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantPagesCount:    1,
			wantNonEmptyPages: 1,
			wantContentSizeGT: 0,
			wantCustomMeta: map[string]any{
				"total_body_items": 4,
				"table_count":      1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			file := readFile(t, tc.fileName)
			defer file.Close()

			parser := NewDOCXParser()
			doc := document.NewDocument()

			err := parser.Parse(context.Background(), file, doc)
			assert.NoError(t, err)

			assert.Equal(t, tc.wantDocType, doc.DocumentType)
			assert.Equal(t, tc.wantParserName, doc.ParserName)
			assert.Equal(t, tc.wantMimeType, doc.MimeType)

			assert.Greater(t, doc.ContentSize, tc.wantContentSizeGT)
			assert.Greater(t, len(doc.PageTextContent), 0)
			assert.Equal(t, tc.wantPagesCount, doc.TotalPagesCount)
			assert.Equal(t, tc.wantNonEmptyPages, doc.NonEmptyPagesCount)

			if tc.wantContentFile != "" {
				contentFile := readFile(t, tc.wantContentFile)
				defer contentFile.Close()

				refContent, err := io.ReadAll(contentFile)
				require.NoError(t, err)

				assert.Equal(t, string(refContent), doc.PageTextContent[0])
			}

			for metaKey, metaVal := range tc.wantCustomMeta {
				assert.Contains(t, doc.CustomMeta, metaKey)
				got, ok := doc.CustomMeta[metaKey]
				if !ok {
					t.Errorf("meta key %s not found", metaKey)
					continue
				}
				switch wantTyped := metaVal.(type) {
				case int:
					gotInt, ok := got.(int)
					if !ok {
						t.Errorf("meta key %s: expected int, got %T", metaKey, got)
						continue
					}
					assert.Equal(t, wantTyped, gotInt, "meta key %s", metaKey)
				case string:
					gotStr, ok := got.(string)
					if !ok {
						t.Errorf("meta key %s: expected string, got %T", metaKey, got)
						continue
					}
					assert.Equal(t, wantTyped, gotStr, "meta key %s", metaKey)
				// Добавляй другие типы по необходимости.
				default:
					assert.Equal(t, metaVal, got, "meta key %s", metaKey)
				}
			}
		})
	}
}

func TestFileParserDOCX_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name       string
		content    string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "Parse empty reader",
			content:    "",
			wantErr:    true,
			wantErrMsg: "Failed to unpack DOCX: zip: not a valid zip file",
		},
		{
			name:       "Parse invalid DOCX content",
			content:    "This is not a DOCX file",
			wantErr:    true,
			wantErrMsg: "Failed to unpack DOCX: zip: not a valid zip file",
		},
		{
			name:       "Parse corrupted DOCX content",
			content:    "PK\x03\x04\x14\x00\x06\x00\x08\x00corrupted content",
			wantErr:    true,
			wantErrMsg: "",
		},
	}
	parser := NewDOCXParser()
	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := document.NewDocument()
			err := parser.Parse(ctx, strings.NewReader(tc.content), doc)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tc.wantErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

const testDataDir = "../../testdata"

func readFile(t *testing.T, fileName string) *os.File {
	docxPath := filepath.Join(testDataDir, "docx", fileName)

	if _, err := os.Stat(docxPath); os.IsNotExist(err) {
		t.Fatal("Test file not found", docxPath)
	}
	file, err := os.Open(docxPath)
	require.NoError(t, err)
	return file
}
