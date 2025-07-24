package chat

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
)

// SystemPromptCreateAPIRequest represents a request to create a system prompt
type SystemPromptCreateAPIRequest struct {
	Name       string `json:"name"`
	Content    string `json:"content"`
	UIMeta     string `json:"ui_meta,omitempty"`
	IsDefault  bool   `json:"is_default,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
	AutoUpdate bool   `json:"auto_update,omitempty"` // If true, the content will be updated from the file on every chat message
}

// SystemPromptAPIRequest represents a request to update a system prompt
type SystemPromptUpdateAPIRequest struct {
	Name       *string `json:"name,omitempty"`
	Content    *string `json:"content,omitempty"`
	UIMeta     *string `json:"ui_meta,omitempty"`
	IsDefault  *bool   `json:"is_default,omitempty"`
	FilePath   *string `json:"file_path,omitempty"`
	AutoUpdate *bool   `json:"auto_update,omitempty"` // If true, the content will be updated from the file on every chat message
}

// SystemPromptAPIResponse represents a response containing a system prompt
type SystemPromptAPIResponse struct {
	Body *SystemPrompt `json:"body"`
}

// SystemPromptAPIResponseList represents a response containing a list of system prompts
type SystemPromptAPIResponseList struct {
	Body struct {
		api.PageAPIResponse
		SystemPrompts []*SystemPrompt `json:"system_prompts"`
	} `json:"body"`
}

// ThreadSystemPromptBulkRequest represents a request to attach/detach multiple system prompts to/from a thread
type ThreadSystemPromptBulkRequest struct {
	ThreadID        string   `json:"thread_id"`
	SystemPromptIDs []string `json:"system_prompt_ids"`
}

// registerSystemPromptAPIRoutes initializes all system prompt API routes
func registerSystemPromptAPIRoutes(humaApi huma.API) {
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-system-prompts",
		Method:      http.MethodGet,
		Path:        "/chat/system-prompts",
		Summary:     "List all system prompts for the current user",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		PageSize int    `query:"page_size" default:"20"`
		Page     int    `query:"page" default:"1"`
		Order    string `query:"order" default:"-updated_at"`
	}) (*SystemPromptAPIResponseList, error) {
		resp := &SystemPromptAPIResponseList{}

		pageRequest := &api.PageAPIRequest{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
			Order:      input.Order,
		}

		err := api.PageAPIInitResponse(pageRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		prompts, errx := ListSystemPrompts(ctx, pageRequest)
		if errx != nil {
			return nil, errx.HumaError()
		}

		resp.Body.SystemPrompts = prompts
		resp.Body.Total = len(prompts)
		return resp, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-system-prompt",
		Method:      http.MethodGet,
		Path:        "/chat/system-prompts/{id}",
		Summary:     "Get a specific system prompt by ID",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"The system prompt UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}) (*SystemPromptAPIResponse, error) {
		promptID, err := uuid.Parse(input.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid system prompt ID format")
		}

		prompt, errx := GetSystemPrompt(ctx, promptID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &SystemPromptAPIResponse{
			Body: prompt,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-system-prompt",
		Method:      http.MethodPost,
		Path:        "/chat/system-prompts",
		Summary:     "Create a new system prompt",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		Body SystemPromptCreateAPIRequest `json:"body"`
	}) (*SystemPromptAPIResponse, error) {
		if input.Body.Name == "" {
			return nil, huma.Error400BadRequest("Name is required")
		}

		if input.Body.Content == "" && input.Body.FilePath == "" {
			return nil, huma.Error400BadRequest("Content or file path is required")
		}

		prompt, errx := CreateSystemPrompt(
			ctx,
			input.Body.Name,
			input.Body.Content,
			input.Body.UIMeta,
			input.Body.IsDefault,
			input.Body.FilePath,
			input.Body.AutoUpdate,
		)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &SystemPromptAPIResponse{
			Body: prompt,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "update-system-prompt",
		Method:      http.MethodPut,
		Path:        "/chat/system-prompts/{id}",
		Summary:     "Update a system prompt",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		ID   string                       `path:"id" doc:"The system prompt UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
		Body SystemPromptUpdateAPIRequest `json:"body"`
	}) (*SystemPromptAPIResponse, error) {
		promptID, err := uuid.Parse(input.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid system prompt ID format")
		}

		if input.Body.Name == nil && input.Body.Content == nil && input.Body.IsDefault == nil && input.Body.FilePath == nil && input.Body.AutoUpdate == nil {
			return nil, huma.Error400BadRequest("At least one field: 'name', 'content', 'is_default', 'file_path' or 'auto_update' must be provided")
		}

		prompt, errx := UpdateSystemPrompt(
			ctx,
			promptID,
			input.Body.Name,
			input.Body.Content,
			input.Body.UIMeta,
			input.Body.IsDefault,
			input.Body.FilePath,
			input.Body.AutoUpdate,
		)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &SystemPromptAPIResponse{
			Body: prompt,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "delete-system-prompt",
		Method:      http.MethodDelete,
		Path:        "/chat/system-prompts/{id}",
		Summary:     "Delete a system prompt",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"The system prompt UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}) (*struct{}, error) {
		promptID, err := uuid.Parse(input.ID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid system prompt ID format")
		}

		errx := DeleteSystemPrompt(ctx, promptID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &struct{}{}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "bulk-attach-system-prompts-to-thread",
		Method:      http.MethodPost,
		Path:        "/chat/system-prompts/attach",
		Summary:     "Attach multiple system prompts to a chat thread",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		Body ThreadSystemPromptBulkRequest `json:"body"`
	}) (*ChatThreadAPIResponse, error) {
		threadID, err := uuid.Parse(input.Body.ThreadID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid thread ID format")
		}

		// Parse all system prompt IDs
		var promptIDs []uuid.UUID
		for _, promptIDStr := range input.Body.SystemPromptIDs {
			promptID, err := uuid.Parse(promptIDStr)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid system prompt ID format: " + promptIDStr)
			}
			promptIDs = append(promptIDs, promptID)
		}

		// Attach all system prompts atomically
		errx := AttachSystemPromptsToThread(ctx, threadID, promptIDs)
		if errx != nil {
			return nil, errx.HumaError()
		}

		// Get the updated thread
		thread, errx := GetChatThread(ctx, threadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadAPIResponse{
			Body: *thread,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "detach-system-prompt-from-thread",
		Method:      http.MethodPost,
		Path:        "/chat/system-prompts/detach",
		Summary:     "Detach given system prompts from a chat thread",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		Body ThreadSystemPromptBulkRequest `json:"body"`
	}) (*ChatThreadAPIResponse, error) {
		threadID, err := uuid.Parse(input.Body.ThreadID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid thread ID format")
		}

		// Parse all system prompt IDs
		var promptIDs []uuid.UUID
		for _, promptIDStr := range input.Body.SystemPromptIDs {
			promptID, err := uuid.Parse(promptIDStr)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid system prompt ID format: " + promptIDStr)
			}
			promptIDs = append(promptIDs, promptID)
		}

		// Detach all system prompts atomically
		errx := DetachSystemPromptsFromThread(ctx, threadID, promptIDs)
		if errx != nil {
			return nil, errx.HumaError()
		}

		// Get the updated thread
		thread, errx := GetChatThread(ctx, threadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadAPIResponse{
			Body: *thread,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "detach-all-system-prompts-from-thread",
		Method:      http.MethodPost,
		Path:        "/chat/system-prompts/detach-all",
		Summary:     "Detach all system prompts from a chat thread",
		Tags:        []string{"System Prompt"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			ThreadID string `json:"thread_id"`
		} `json:"body"`
	}) (*ChatThreadAPIResponse, error) {
		threadID, err := uuid.Parse(input.Body.ThreadID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid thread ID format")
		}

		// Detach all system prompts from the thread
		errx := DetachAllSystemPromptsFromThread(ctx, threadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		// Get the updated thread
		thread, errx := GetChatThread(ctx, threadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadAPIResponse{
			Body: *thread,
		}, nil
	})

	// Get system prompts for a thread
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-thread-system-prompts",
		Method:      http.MethodGet,
		Path:        "/chat/threads/{thread_id}/system-prompts",
		Summary:     "Get all system prompts attached to a thread",
		Tags:        []string{"System Prompt", "Chat Thread"},
	}, func(ctx context.Context, input *struct {
		ThreadID string `path:"thread_id" example:"550e8400-e29b-41d4-a716-446655440001"`
	}) (*SystemPromptAPIResponseList, error) {
		threadID, err := uuid.Parse(input.ThreadID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid thread ID format")
		}

		// Get system prompts for the thread
		prompts, errx := GetSystemPromptsForThread(ctx, threadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		resp := &SystemPromptAPIResponseList{}
		resp.Body.SystemPrompts = prompts
		resp.Body.Total = len(prompts)
		return resp, nil
	})

	// Get threads for a system prompt
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-system-prompt-threads",
		Method:      http.MethodGet,
		Path:        "/chat/system-prompts/{prompt_id}/threads",
		Summary:     "Get all threads that have a specific system prompt attached",
		Tags:        []string{"System Prompt", "Chat Thread"},
	}, func(ctx context.Context, input *struct {
		PromptID string `path:"prompt_id" example:"550e8400-e29b-41d4-a716-446655440002"`
	}) (*ChatThreadAPIResponseList, error) {
		promptID, err := uuid.Parse(input.PromptID)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid system prompt ID format")
		}

		// Get threads for the system prompt
		threads, errx := GetThreadsForSystemPrompt(ctx, promptID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		resp := &ChatThreadAPIResponseList{}
		// Convert []*ChatThread to []ChatThread
		for _, thread := range threads {
			resp.Body.ChatThreads = append(resp.Body.ChatThreads, *thread)
		}
		resp.Body.Total = len(threads)
		return resp, nil
	})
}
