package chat

import (
	"context"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// registerChatAttachmentAPIRoutes registers all chat attachment API routes
func registerChatAttachmentAPIRoutes(humaApi huma.API) {
	// Upload attachment to existing thread
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:  "upload-attachment-to-thread",
		Method:       http.MethodPost,
		Path:         "/chat/threads/{thread_id}/attachment",
		Summary:      "Upload a file attachment to an existing chat thread",
		Description:  "Accepts multipart/form-data file uploads, parses the file, and creates a new message with the file content",
		Tags:         []string{"Chat Thread", "File Attachment"},
		MaxBodyBytes: int64(chatConfigInstance.UploadedFileMaxSizeKB) * 1024,
	}, func(ctx context.Context, input *struct {
		ThreadID uuid.UUID `path:"thread_id" doc:"The chat thread UUID" example:"550e8400-e29b-41d4-a716-446655440001"`
		RawBody  multipart.Form
	}) (*ChatMessageAPIResponse, error) {
		// Verify thread exists
		_, errx := GetChatThread(ctx, input.ThreadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		logging.Debug("Processing file attachment upload for thread %s, found %d file keys", input.ThreadID, len(input.RawBody.File))

		// Get the file from the multipart form
		files := input.RawBody.File["file"]
		if len(files) == 0 {
			return nil, huma.Error400BadRequest("No file provided in the request")
		}

		// Use the first file
		fileHeader := files[0]
		file, err := fileHeader.Open()
		if err != nil {
			logging.Error("Failed to open uploaded file: %v", err)
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to open uploaded file: %v", err))
		}
		defer file.Close()

		// Process the file attachment
		formattedContent, fileExt, originalSize, fullContentLength, truncatedContentLength, isTruncated, errx := processFileAttachment(file, fileHeader)
		if errx != nil {
			return nil, errx.HumaError()
		}

		if fullContentLength > int64(chatConfigInstance.UploadedFileMaxContentLength) {
			return nil, huma.Error400BadRequest("File content is too long")
		}

		if fullContentLength == 0 {
			return nil, huma.Error400BadRequest("File text content is empty")
		}

		// Create the special file attachment message
		msg, errx := CreateFileAttachmentMessage(
			ctx,
			uuid.Nil,
			input.ThreadID,
			formattedContent,
			fileHeader.Filename,
			fileExt,
			originalSize,
			fullContentLength,
			truncatedContentLength,
			isTruncated,
		)

		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatMessageAPIResponse{
			Body: *msg,
		}, nil
	})

	// Create new thread with attachment
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:  "create-chat-attachment",
		Method:       http.MethodPost,
		Path:         "/chat/attachments",
		Summary:      "Create a new chat thread with a file attachment as the first message",
		Description:  "Accepts multipart/form-data file uploads, parses the file, creates a new thread, and adds the file content as the first message",
		Tags:         []string{"Chat Message", "File Attachment"},
		MaxBodyBytes: int64(chatConfigInstance.UploadedFileMaxSizeKB) * 1024,
	}, func(ctx context.Context, input *struct {
		RawBody multipart.Form
		GroupID string `query:"group_id" doc:"optional group ID to create the thread in"`
	}) (*ChatMessageAPIResponse, error) {
		var groupID uuid.UUID
		var err error

		if input.GroupID != "" {
			groupID, err = uuid.Parse(input.GroupID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid group ID format")
			}

			// Verify group exists if provided
			_, errx := GetChatThreadGroup(ctx, groupID)
			if errx != nil {
				return nil, errx.HumaError()
			}
		} else {
			groupID = uuid.Nil
		}

		logging.Debug("Processing file attachment upload for new thread, found %d file keys", len(input.RawBody.File))

		// Get the file from the multipart form
		files := input.RawBody.File["file"]
		if len(files) == 0 {
			return nil, huma.Error400BadRequest("No file provided in the request")
		}

		// Use the first file
		fileHeader := files[0]
		file, err := fileHeader.Open()
		if err != nil {
			logging.Error("Failed to open uploaded file: %v", err)
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to open uploaded file: %v", err))
		}
		defer file.Close()

		// Process the file attachment
		formattedContent, fileExt, originalSize, fullContentLength, truncatedContentLength, isTruncated, errx := processFileAttachment(file, fileHeader)
		if errx != nil {
			return nil, errx.HumaError()
		}

		// Create the attachment message (this will create a new thread automatically when threadID is uuid.Nil)
		msg, errx := CreateFileAttachmentMessage(
			ctx,
			groupID,
			uuid.Nil, // No thread ID - will create a new thread
			formattedContent,
			fileHeader.Filename,
			fileExt,
			originalSize,
			fullContentLength,
			truncatedContentLength,
			isTruncated,
		)

		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatMessageAPIResponse{
			Body: *msg,
		}, nil
	})
}
