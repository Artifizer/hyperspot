package chat

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
)

// ChatThreadAPIResponse is the API response format for a single thread
type ChatThreadAPIResponse struct {
	Body ChatThread `json:"body"`
}

// ChatThreadAPIResponseList is the API response format for a list of threads
type ChatThreadAPIResponseList struct {
	Body struct {
		api.PageAPIResponse
		ChatThreads []ChatThread `json:"threads"`
	} `json:"body"`
}

// InitChatThreadAPIRoutes initializes all chat thread API routes
func registerChatThreadAPIRoutes(humaApi huma.API) {
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-chat-threads",
		Method:      http.MethodGet,
		Path:        "/chat/threads",
		Summary:     "List all chat threads not belonging to a group",
		Tags:        []string{"Chat Thread"},
	}, func(ctx context.Context, input *struct {
		PageSize int    `query:"page_size" default:"20"`
		Page     int    `query:"page" default:"1"`
		Order    string `query:"order" default:"-created_at"`
		GroupID  string `query:"group_id"`
	}) (*ChatThreadAPIResponseList, error) {
		resp := &ChatThreadAPIResponseList{}

		var groupID uuid.UUID
		var err error

		if input.GroupID != "" {
			groupID, err = uuid.Parse(input.GroupID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid group ID format")
			}
		} else {
			groupID = uuid.Nil
		}

		pageRequest := &api.PageAPIRequest{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
			Order:      input.Order,
		}

		err = api.PageAPIInitResponse(pageRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		threads, errx := ListChatThreads(ctx, groupID, pageRequest)
		if errx != nil {
			return nil, errx.HumaError()
		}

		resp.Body.ChatThreads = threads

		resp.Body.Total = len(threads)
		return resp, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-chat-messages-in-thread",
		Method:      http.MethodGet,
		Path:        "/chat/threads/{thread_id}/messages",
		Summary:     "List messages for a specific chat thread",
		Tags:        []string{"Chat Thread"},
	}, func(ctx context.Context, input *struct {
		ThreadID uuid.UUID `path:"thread_id" doc:"The chat thread UUID" example:"550e8400-e29b-41d4-a716-446655440001"`
		PageSize int       `query:"page_size" default:"20"`
		Page     int       `query:"page" default:"1"`
		Order    string    `query:"order" default:"-created_at"`
	}) (*ChatMessageAPIResponseList, error) {
		resp := &ChatMessageAPIResponseList{}

		pageRequest := &api.PageAPIRequest{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
			Order:      input.Order,
		}

		err := api.PageAPIInitResponse(pageRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		messages, errx := listMessages(ctx, input.ThreadID, pageRequest)
		if errx != nil {
			return nil, errx.HumaError()
		}

		for _, message := range messages {
			resp.Body.ChatMessages = append(resp.Body.ChatMessages, *message)
		}

		resp.Body.PageAPIResponse.Total = len(messages)
		return resp, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-chat-thread",
		Method:      http.MethodGet,
		Path:        "/chat/threads/{id}",
		Summary:     "Get a specific chat thread by ID",
		Tags:        []string{"Chat Thread"},
	}, func(ctx context.Context, input *struct {
		ID uuid.UUID `path:"id" doc:"The chat thread UUID" example:"550e8400-e29b-41d4-a716-446655440001"`
	}) (*ChatThreadAPIResponse, error) {
		thread, errx := GetChatThread(ctx, input.ID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadAPIResponse{
			Body: *thread,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-chat-thread",
		Method:      http.MethodPost,
		Path:        "/chat/threads",
		Summary:     "Create a new chat thread",
		Tags:        []string{"Chat Thread"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Title           string   `json:"title"`
			GroupID         string   `json:"group_id,omitempty"`
			IsPinned        *bool    `json:"is_pinned,omitempty"`
			IsPublic        *bool    `json:"is_public,omitempty"`
			IsTemporary     *bool    `json:"is_temporary,omitempty"`
			SystemPromptIDs []string `json:"system_prompt_ids,omitempty"`
		}
	}) (*ChatThreadAPIResponse, error) {
		var groupID uuid.UUID
		var err error
		if input.Body.GroupID != "" {
			groupID, err = uuid.Parse(input.Body.GroupID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid group ID format")
			}
		} else {
			groupID = uuid.Nil
		}

		var systemPromptIDs []uuid.UUID
		for _, promptIDStr := range input.Body.SystemPromptIDs {
			if promptIDStr != "" {
				parsedID, err := uuid.Parse(promptIDStr)
				if err != nil {
					return nil, huma.Error400BadRequest("Invalid system prompt ID format: " + promptIDStr)
				}
				systemPromptIDs = append(systemPromptIDs, parsedID)
			}
		}

		thread, errx := CreateChatThread(ctx, groupID, input.Body.Title, input.Body.IsPinned, input.Body.IsPublic, input.Body.IsTemporary, systemPromptIDs)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadAPIResponse{
			Body: *thread,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "update-chat-thread",
		Method:      http.MethodPut,
		Path:        "/chat/threads/{id}",
		Summary:     "Update a chat thread",
		Tags:        []string{"Chat Thread"},
	}, func(ctx context.Context, input *struct {
		ThreadID uuid.UUID `path:"id" doc:"Chat thread UUID" example:"550e8400-e29b-41d4-a716-446655440001"`
		Body     struct {
			GroupID         *string  `json:"group_id,omitempty"`
			Title           *string  `json:"title,omitempty"`
			IsPinned        *bool    `json:"is_pinned,omitempty"`
			IsPublic        *bool    `json:"is_public,omitempty"`
			IsTemporary     *bool    `json:"is_temporary,omitempty"`
			SystemPromptIDs []string `json:"system_prompt_ids,omitempty"`
		} `json:"body"`
	}) (*ChatThreadAPIResponse, error) {
		var groupID *uuid.UUID
		if input.Body.GroupID != nil {
			if *input.Body.GroupID == "" {
				groupID = &uuid.Nil
			} else {
				groupID = &uuid.UUID{}
				var err error
				*groupID, err = uuid.Parse(*input.Body.GroupID)
				if err != nil {
					return nil, huma.Error400BadRequest("Invalid group ID format")
				}
			}
		} else {
			groupID = nil
		}

		var systemPromptIDs []uuid.UUID
		if input.Body.SystemPromptIDs != nil {
			for _, promptIDStr := range input.Body.SystemPromptIDs {
				if promptIDStr != "" {
					parsedID, err := uuid.Parse(promptIDStr)
					if err != nil {
						return nil, huma.Error400BadRequest("Invalid system prompt ID format: " + promptIDStr)
					}
					systemPromptIDs = append(systemPromptIDs, parsedID)
				}
			}
		}

		thread, errx := UpdateChatThread(ctx, input.ThreadID, input.Body.Title, groupID, input.Body.IsPinned, input.Body.IsPublic, input.Body.IsTemporary, systemPromptIDs)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadAPIResponse{
			Body: *thread,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "delete-chat-thread",
		Method:      http.MethodDelete,
		Path:        "/chat/threads/{id}",
		Summary:     "Delete a chat thread",
		Tags:        []string{"Chat Thread"},
	}, func(ctx context.Context, input *struct {
		ThreadID uuid.UUID `path:"id" doc:"The chat thread UUID" example:"550e8400-e29b-41d4-a716-446655440001"`
	}) (*struct{}, error) {
		errx := DeleteChatThread(ctx, input.ThreadID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &struct{}{}, nil
	})
}
