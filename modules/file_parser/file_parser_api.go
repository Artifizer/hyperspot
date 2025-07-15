package file_parser

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/document"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
)

var apiLogger = logging.MainLogger

// FileParseResponse represents the response from a file parsing operation
type FileParseResponse struct {
	Body document.Document `json:"body"`
}

// FileParserInfoResponse represents information about available file parsers
type FileParserInfoResponse struct {
	Body struct {
		api.PageAPIResponse
		SupportedExtensions map[string][]string `json:"supported_extensions"`
	}
}

// registerFileParserAPIRoutes registers all file parser API routes
func registerFileParserAPIRoutes(humaApi huma.API) {
	// Get information about available file parsers
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-file-parser-info",
		Method:      http.MethodGet,
		Path:        "/file-parser/info",
		Summary:     "Get information about available file parsers",
		Tags:        []string{"File Parser"},
	}, func(ctx context.Context, input *struct{}) (*FileParserInfoResponse, error) {
		resp := &FileParserInfoResponse{}
		resp.Body.PageNumber = 1
		resp.Body.PageSize = 100
		resp.Body.Total = 1

		// Get all parser IDs and their supported extensions
		parserIDs := GetParserIDs()
		resp.Body.SupportedExtensions = make(map[string][]string)

		for _, parserID := range parserIDs {
			resp.Body.SupportedExtensions[parserID] = GetSupportedExtensions(parserID)
		}

		return resp, nil
	})

	// Parse a file from a local path
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "parse-local-file",
		Method:      http.MethodPost,
		Path:        "/file-parser/parse-local",
		Summary:     "Parse a file from a local path",
		Tags:        []string{"File Parser"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			FilePath string `json:"file_path" doc:"Path to the local file to parse"`
		} `json:"body"`
	}) (*FileParseResponse, error) {
		filePath := input.Body.FilePath

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, huma.Error404NotFound(fmt.Sprintf("File not found: %s", filePath))
		}

		// Parse the document
		doc, err := ParseLocalDocument(filePath)
		if err != nil {
			// Check if it's an errorx error (like unsupported file type)
			if errx, ok := err.(errorx.Error); ok {
				return nil, errx.HumaError()
			}

			apiLogger.Error("Failed to parse local document: %v", err)
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to parse document: %v", err))
		}

		return &FileParseResponse{
			Body: *doc,
		}, nil
	})

	// Upload and parse a file
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:  "upload-file",
		Method:       http.MethodPost,
		Path:         "/file-parser/upload",
		Summary:      "Upload and parse a file",
		Description:  "Accepts multipart/form-data file uploads",
		Tags:         []string{"File Parser"},
		MaxBodyBytes: int64(fileParserConfigInstance.UploadedFileMaxSizeKB) * 1024,
	}, func(ctx context.Context, input *struct {
		RawBody multipart.Form
	}) (*FileParseResponse, error) {
		apiLogger.Trace("Processing file upload, found %d file keys", len(input.RawBody.File))

		// Get the file from the multipart form
		files := input.RawBody.File["file"]
		if len(files) == 0 {
			return nil, huma.Error400BadRequest("No file provided in the request")
		}

		// Use the first file
		fileHeader := files[0]
		file, err := fileHeader.Open()
		if err != nil {
			apiLogger.Error("Failed to open uploaded file: %v", err)
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to open uploaded file: %v", err))
		}
		defer file.Close()

		// Save to temporary file
		// TODO, FIXME: need to use common file uploader config
		tempPath, err := utils.SaveUploadedTempFile(file, "", fileHeader.Filename)
		if err != nil {
			apiLogger.Error("Error saving temporary file: %v", err)
			return nil, huma.Error500InternalServerError("Error saving file")
		}

		// Clean up the temporary file when done
		defer func() {
			if removeErr := os.Remove(tempPath); removeErr != nil {
				apiLogger.Warn("Failed to remove temporary file %s: %v", tempPath, removeErr)
			}
		}()

		// Parse the document from the temporary file
		doc, err := ParseLocalDocument(tempPath)
		if err != nil {
			// Check if it's an errorx error (like unsupported file type)
			if errx, ok := err.(errorx.Error); ok {
				return nil, errx.HumaError()
			}

			apiLogger.Error("Failed to parse uploaded document: %v", err)
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to parse document: %v", err))
		}

		return &FileParseResponse{
			Body: *doc,
		}, nil
	})

	// Parse a file from a URL
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "parse-url",
		Method:      http.MethodPost,
		Path:        "/file-parser/parse-url",
		Summary:     "Parse a file from a URL",
		Tags:        []string{"File Parser"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			URL string `json:"url" doc:"URL of the file to parse"`
		} `json:"body"`
	}) (*FileParseResponse, error) {
		url := input.Body.URL

		// Parse the document from URL
		doc, errx := ParseURL(url)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &FileParseResponse{
			Body: *doc,
		}, nil
	})
}

// InitAPIRoutes initializes the API routes for the file parser module
func InitAPIRoutes(humaApi huma.API) {
	apiLogger.Info("Initializing file parser API routes")
	registerFileParserAPIRoutes(humaApi)
}
