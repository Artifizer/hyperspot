package dataset

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hypernetix/hyperspot/libs/api"
)

type DatasetJobParamsAPIResponse struct {
	Body DatasetJobParams `json:"body"`
}

type DatasetAPIResponseItem Dataset

type DatasetAPIResponse struct {
	Body DatasetAPIResponseItem `json:"body"`
}

type ListDatasetAPIRequest struct {
	api.PageAPIRequest
}

type ListDatasetAPIResponse struct {
	Body struct {
		api.PageAPIResponse
		Datasets []DatasetAPIResponseItem `json:"datasets"`
	} `json:"body"`
}

func initDatasetAPIRoutes(humaApi huma.API) {
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-dataset-job-params",
		Method:      http.MethodGet,
		Path:        "/job_params/dataset",
		Summary:     "Job parameters schema for Dataset Job",
		Tags:        []string{"LLM Benchmarks Datasets"},
	}, func(ctx context.Context, input *struct{}) (*DatasetJobParamsAPIResponse, error) {
		return &DatasetJobParamsAPIResponse{Body: DatasetJobParams{}}, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-datasets",
		Method:      http.MethodGet,
		Path:        "/datasets",
		Summary:     "List all datasets",
		Tags:        []string{"LLM Benchmarks Datasets"},
	}, func(ctx context.Context, request *ListDatasetAPIRequest) (*ListDatasetAPIResponse, error) {
		resp := &ListDatasetAPIResponse{}
		err := api.PageAPIInitResponse(&request.PageAPIRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		datasets := ListDatasets(ctx, &request.PageAPIRequest)
		for _, ds := range datasets {
			resp.Body.Datasets = append(resp.Body.Datasets, DatasetAPIResponseItem(*ds))
		}
		resp.Body.PageAPIResponse.Total = len(datasets)

		return resp, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-dataset",
		Method:      http.MethodGet,
		Path:        "/datasets/{id}",
		Summary:     "Get a dataset by ID",
		Tags:        []string{"LLM Benchmarks Datasets"},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id" doc:"The dataset name" example:"humaneval"`
	}) (*DatasetAPIResponse, error) {
		ds := GetDataset(input.ID)
		if ds == nil {
			return nil, huma.Error404NotFound("Dataset not found")
		}

		return &DatasetAPIResponse{
			Body: DatasetAPIResponseItem(*ds),
		}, nil
	})
}
