package sysinfo

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/utils"
)

type SysInfoAPIResponse struct {
	Body utils.SystemInfo `json:"body"`
}

type GetSysInfoByIDInput struct {
	ID uuid.UUID `path:"id" doc:"System info UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}

func initSysInfoAPIRoutes(humaApi huma.API) {
	// GET /sysinfo - Get current system information
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-sysinfo",
		Method:      http.MethodGet,
		Path:        "/sysinfo",
		Summary:     "Get current system information",
		Tags:        []string{"System Information"},
	}, func(ctx context.Context, input *struct{}) (*SysInfoAPIResponse, error) {
		sysInfo, err := GetSysInfo()
		if err != nil {
			return nil, errorx.NewErrInternalServerError(err.Error())
		}

		return &SysInfoAPIResponse{
			Body: *sysInfo,
		}, nil
	})

	// GET /sysinfo/{id} - Get historical system information by ID
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-sysinfo-by-id",
		Method:      http.MethodGet,
		Path:        "/sysinfo/{id}",
		Summary:     "Get historical system information by ID",
		Tags:        []string{"System Information"},
	}, func(ctx context.Context, input *GetSysInfoByIDInput) (*SysInfoAPIResponse, error) {
		/*
			sysInfoId, err := uuid.Parse(input.ID)
			if err != nil {
				return nil, huma.Error400BadRequest("Invalid sysinfo ID")
			}
		*/
		sysInfoId := input.ID

		sysInfo, errx := GetSysInfoFromDB(sysInfoId)
		if errx != nil {
			return nil, errx.HumaError()
		}

		return &SysInfoAPIResponse{
			Body: *sysInfo,
		}, nil
	})
}
