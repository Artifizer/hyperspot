package chat

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
)

type ChatThreadGroupAPIResponse struct {
	Body ChatThreadGroup `json:"body"`
}

type ChatThreadGroupAPIResponseList struct {
	Body struct {
		api.PageAPIResponse
		ChatThreadGroups []ChatThreadGroup `json:"groups"`
	} `json:"body"`
}

func registerChatThreadGroupAPIRoutes(humaApi huma.API) {
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-chat-thread-groups",
		Method:      http.MethodGet,
		Path:        "/chat/groups",
		Summary:     "List all chat thread groups",
		Tags:        []string{"Chat Thread Group"},
	}, func(ctx context.Context, input *struct {
		PageSize int    `query:"page_size" default:"20"`
		Page     int    `query:"page" default:"1"`
		Order    string `query:"order" default:"-updated_at"`
	}) (*ChatThreadGroupAPIResponseList, error) {
		resp := &ChatThreadGroupAPIResponseList{}

		pageRequest := &api.PageAPIRequest{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
			Order:      input.Order,
		}

		err := api.PageAPIInitResponse(pageRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		groups, errx := ListChatThreadGroups(ctx, pageRequest)
		if errx != nil {
			return nil, errx.HumaError()
		}

		for _, group := range groups {
			resp.Body.ChatThreadGroups = append(resp.Body.ChatThreadGroups, *group)
		}

		resp.Body.Total = len(groups)
		return resp, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-chat-thread-group",
		Method:      http.MethodGet,
		Path:        "/chat/groups/{id}",
		Summary:     "Get a specific chat thread group by ID",
		Tags:        []string{"Chat Thread Group"},
	}, func(ctx context.Context, input *struct {
		ID uuid.UUID `path:"id" doc:"The chat thread group UUID" example:"123e4567-e89b-12d3-a456-426614174000"`
	}) (*ChatThreadGroupAPIResponse, error) {
		/*
			groupID, err := uuid.Parse(input.ID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid group ID format")
			}*/

		groupID := input.ID
		group, errx := GetChatThreadGroup(ctx, groupID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadGroupAPIResponse{
			Body: *group,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-chat-thread-group",
		Method:      http.MethodPost,
		Path:        "/chat/groups",
		Summary:     "Create a new chat thread group",
		Tags:        []string{"Chat Thread Group"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Title       string `json:"title"`
			IsPinned    *bool  `json:"is_pinned,omitempty"`
			IsPublic    *bool  `json:"is_public,omitempty"`
			IsTemporary *bool  `json:"is_temporary,omitempty"`
		}
	}) (*ChatThreadGroupAPIResponse, error) {
		group, errx := CreateChatThreadGroup(ctx,
			input.Body.Title,
			input.Body.IsPinned,
			input.Body.IsPublic,
			input.Body.IsTemporary,
		)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadGroupAPIResponse{
			Body: *group,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "update-chat-thread-group",
		Method:      http.MethodPut,
		Path:        "/chat/groups/{id}",
		Summary:     "Update a chat thread group title",
		Tags:        []string{"Chat Thread Group"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Title       *string `json:"title,omitempty"`
			IsPinned    *bool   `json:"is_pinned,omitempty"`
			IsPublic    *bool   `json:"is_public,omitempty"`
			IsTemporary *bool   `json:"is_temporary,omitempty"`
		}
		GroupID uuid.UUID `path:"id" doc:"The chat thread group UUID" example:"123e4567-e89b-12d3-a456-426614174000"`
	}) (*ChatThreadGroupAPIResponse, error) {
		groupID := input.GroupID

		group, errx := UpdateChatThreadGroup(ctx, groupID, input.Body.Title, input.Body.IsPinned, input.Body.IsPublic, input.Body.IsTemporary)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &ChatThreadGroupAPIResponse{
			Body: *group,
		}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "delete-chat-thread-group",
		Method:      http.MethodDelete,
		Path:        "/chat/groups/{id}",
		Summary:     "Delete a chat thread group",
		Tags:        []string{"Chat Thread Group"},
	}, func(ctx context.Context, input *struct {
		GroupID uuid.UUID `path:"id" doc:"The chat thread group UUID" example:"123e4567-e89b-12d3-a456-426614174000"`
	}) (*struct{}, error) {
		groupID := input.GroupID

		errx := DeleteChatThreadGroup(ctx, groupID)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &struct{}{}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-threads-in-group",
		Method:      http.MethodGet,
		Path:        "/chat/groups/{id}/threads",
		Summary:     "Get threads in a group",
		Tags:        []string{"Chat Thread Group"},
	}, func(ctx context.Context, input *struct {
		PageSize int       `query:"page_size" default:"20"`
		Page     int       `query:"page" default:"1"`
		Order    string    `query:"order" default:"-updated_at"`
		GroupID  uuid.UUID `path:"id" doc:"The chat thread group UUID" example:"123e4567-e89b-12d3-a456-426614174000"`
	}) (*ChatThreadAPIResponseList, error) {
		groupID := input.GroupID

		pageRequest := &api.PageAPIRequest{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
			Order:      input.Order,
		}

		threads, errx := ListChatThreads(ctx, groupID, pageRequest)
		if errx != nil {
			return nil, errx.HumaError()
		}

		resp := &ChatThreadAPIResponseList{}
		err := api.PageAPIInitResponse(pageRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		for _, thread := range threads {
			resp.Body.ChatThreads = append(resp.Body.ChatThreads, thread)
		}

		resp.Body.Total = len(threads)
		return resp, nil
	})
}
