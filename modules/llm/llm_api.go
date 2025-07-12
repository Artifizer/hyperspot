// internal/api/handlers/models.go
package llm

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
)

type ListLLMServicesAPIRequest struct {
	api.PageAPIRequest
}

type LLMServiceDetails struct {
	Name         string                 `json:"name"`
	Capabilities LLMServiceCapabilities `json:"capabilities"`
}

type ListLLMServicesAPIResponse struct {
	Body struct {
		api.PageAPIResponse
		Services []LLMServiceDetails `json:"services"`
	}
}

type LLMModelInfoAPIResponse struct {
	Body LLMModel `json:"body"`
}

type ListLLMModelsAPIRequest struct {
	api.PageAPIRequest
	Service string `path:"service_name" doc:"The LLM service name" example:"lm_studio"`
}

type ListLLMModelsAPIResponse struct {
	Body struct {
		api.PageAPIResponse
		Models []*LLMModel `json:"models"`
	}
}

type LLMPingAPIResponse struct {
	Body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
}

type LLMServiceCapabilitiesAPIResponse struct {
	Body LLMServiceCapabilities `json:"body"`
}

func registerLLMApiRoutes(humaApi huma.API) {
	// List all LLM services
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-llm-services",
		Method:      http.MethodGet,
		Path:        "/llm/services",
		Summary:     "List available LLM services",
		Tags:        []string{"LLM Services"},
	}, func(c context.Context, input *ListLLMServicesAPIRequest) (*ListLLMServicesAPIResponse, error) {
		resp := &ListLLMServicesAPIResponse{}

		err := api.PageAPIInitResponse(&input.PageAPIRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		names := ListAliveServicesNames(c, &input.PageAPIRequest)

		for _, name := range names {
			service, err := GetLLMService(c, name)
			if err != nil {
				return nil, huma.Error404NotFound(fmt.Sprintf("Service '%s' not found", name))
			}

			resp.Body.Services = append(resp.Body.Services, LLMServiceDetails{
				Name:         name,
				Capabilities: *service.GetCapabilities(),
			})
		}

		resp.Body.PageAPIResponse.Total = len(names)
		return resp, nil
	})

	// List models for a service
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-llm-models",
		Method:      http.MethodGet,
		Path:        "/llm/services/{service_name}/models",
		Summary:     "List available models for a service",
		Tags:        []string{"LLM Services"},
	}, func(c context.Context, input *ListLLMModelsAPIRequest) (*ListLLMModelsAPIResponse, error) {
		resp := &ListLLMModelsAPIResponse{}
		err := api.PageAPIInitResponse(&input.PageAPIRequest, &resp.Body.PageAPIResponse)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		service, err := GetLLMService(c, input.Service)
		if err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("Service not found: %v", err))
		}

		models, err := service.ListModels(c, &input.PageAPIRequest)
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to list models: %v", err))
		}

		resp.Body.Models = models
		resp.Body.PageAPIResponse.Total = len(models)
		return resp, nil
	})

	// Get model details
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-service-llm-model",
		Method:      http.MethodGet,
		Path:        "/llm/services/{service_name}/models/{model_name}",
		Summary:     "Get details about a specific LLM service model",
		Tags:        []string{"LLM Services"},
	}, func(c context.Context, input *struct {
		Service   string `path:"service_name" doc:"The LLM service name" example:"lm_studio"`
		ModelName string `path:"model_name" doc:"The LLM model name" example:"qwen2.5-0.5b-instruct"`
	}) (*LLMModelInfoAPIResponse, error) {
		service, err := GetLLMService(c, input.Service)
		if err != nil {
			return nil, huma.Error404NotFound("Service not found")
		}

		model, err := service.GetModel(c, input.ModelName)
		if err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("Model '%s' not found", input.ModelName))
		}

		return &LLMModelInfoAPIResponse{Body: *model}, nil
	})

	// Ping service
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "ping-llm-service",
		Method:      http.MethodGet,
		Path:        "/llm/services/{service_name}/ping",
		Summary:     "Check if a service is available",
		Tags:        []string{"LLM Services"},
	}, func(c context.Context, input *struct {
		ServiceName string `path:"service_name" doc:"The LLM service name" example:"lm_studio"`
	}) (*LLMPingAPIResponse, error) {
		service, err := GetLLMService(c, input.ServiceName)
		if err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("Service '%s' not found", input.ServiceName))
		}

		// Try to ping the service
		if err := service.Ping(c); err != nil {
			resp := &LLMPingAPIResponse{}
			resp.Body.Status = "error"
			resp.Body.Message = fmt.Sprintf("Service unavailable: %v", err)
			return resp, nil
		}

		resp := &LLMPingAPIResponse{}
		resp.Body.Status = "ok"
		resp.Body.Message = "Service is available"
		return resp, nil
	})

	// Get service capabilities
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-llm-service-capabilities",
		Method:      http.MethodGet,
		Path:        "/llm/services/{service_name}/capabilities",
		Summary:     "Get capabilities of a service",
		Tags:        []string{"LLM Services"},
	}, func(c context.Context, input *struct {
		ServiceName string `path:"service_name" doc:"The LLM service name" example:"lm_studio"`
	}) (*LLMServiceCapabilitiesAPIResponse, error) {
		service, err := GetLLMService(c, input.ServiceName)
		if err != nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("Service '%s' not found", input.ServiceName))
		}

		capabilities := service.GetCapabilities()
		return &LLMServiceCapabilitiesAPIResponse{Body: *capabilities}, nil
	})

	// List all models across all services
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-all-llm-models",
		Method:      http.MethodGet,
		Path:        "/llm/models",
		Summary:     "List all models across all services",
		Tags:        []string{"LLM Models"},
	}, func(c context.Context, input *struct{}) (*ListLLMModelsAPIResponse, error) {
		allModels := ListAllModels(c, true)
		resp := &ListLLMModelsAPIResponse{}

		resp.Body.Models = append(resp.Body.Models, allModels...)

		return resp, nil
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-llm-model",
		Method:      http.MethodGet,
		Path:        "/llm/models/{model_name}",
		Summary:     "Get details about a specific model",
		Tags:        []string{"LLM Models"},
	}, func(c context.Context, input *struct {
		ModelName string `path:"model_name" doc:"The LLM model name" example:"qwen2.5-0.5b-instruct"`
	}) (*LLMModelInfoAPIResponse, error) {
		model := GetModel(c, input.ModelName, false, ModelStateAny)
		if model == nil {
			return nil, huma.Error404NotFound(fmt.Sprintf("Model '%s' is not found", input.ModelName))
		}

		return &LLMModelInfoAPIResponse{Body: *model}, nil
	})
}

type OpenAICompletionAPIResponse struct {
	Body openapi_client.OpenAICompletionResponseRaw `json:"body"`
}

type OpenAIEmbeddingAPIResponse struct {
	Body openapi_client.OpenAIEmbeddingResponse `json:"body"`
}

type OpenAIEmbeddingRequest struct {
	Body openapi_client.OpenAIEmbeddingRequest `json:"body"`
}

type OpenAIEmbeddingResponse struct {
	Body openapi_client.OpenAIEmbeddingResponse `json:"body"`
}

func registerOpenAIAPIRoutes(humaApi huma.API) {
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-chat-completion",
		Method:      http.MethodPost,
		Path:        "/v1/chat/completions",
		Summary:     "Create chat completion",
		Tags:        []string{"OpenAI compatible API"},
	}, func(ctx context.Context, input *struct {
		Body openapi_client.OpenAICompletionRequestRaw
	}) (*huma.StreamResponse, error) {
		body := input.Body // read body ASAP, because it has it's own timeout

		// Find the model
		modelName, ok := body["model"].(string)
		if !ok {
			return nil, huma.Error400BadRequest("model field is required")
		}

		model, err := GetModelOrFirstLoaded(ctx, modelName)
		if err != nil {
			return nil, err.HumaError()
		}

		body["model"] = model.UpstreamModelName

		stream, ok := body["stream"].(bool)

		if ok && stream {
			if !model.ServicePtr.GetCapabilities().Streaming {
				body["stream"] = false
				stream = false
			}
		}

		if ok && stream {
			upstreamResp, err := model.ServicePtr.ChatCompletionsStreamRaw(ctx, model, body)
			if err != nil {
				return nil, huma.Error502BadGateway(fmt.Sprintf("LLM service '%s' returned an error: %v",
					model.ServicePtr.GetName(), err))
			}

			// Check for non-200 responses.
			if upstreamResp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(upstreamResp.Body)
				if err := upstreamResp.Body.Close(); err != nil {
					logging.Error("Failed to close upstream response body: %v", err)
				}
				return nil, fmt.Errorf("upstream error (%d): %s", upstreamResp.StatusCode, string(bodyBytes))
			}

			// Return a streaming response.
			return &huma.StreamResponse{
				Body: func(ctx huma.Context) {
					if err := ctx.SetReadDeadline(time.Now().Add(config.GetServerTimeout())); err != nil {
						logging.Error("Failed to set read deadline: %v", err)
					}

					// defer upstreamResp.Body.Close()
					// Optionally copy some headers from the upstream response.
					ctx.SetHeader("Content-Type", upstreamResp.Header.Get("Content-Type"))

					// Create a buffer and continuously stream data.
					buf := make([]byte, 4096)
					for {
						n, err := upstreamResp.Body.Read(buf)
						if n > 0 {
							// Write the read bytes to the client.
							_, writeErr := ctx.BodyWriter().Write(buf[:n])
							if writeErr != nil {
								log.Printf("error writing to response: %v", writeErr)
								return
							}
							// Flush after each write if supported.
							if flusher, ok := ctx.BodyWriter().(http.Flusher); ok {
								flusher.Flush()
							}
						}
						if err != nil {
							if err == io.EOF {
								break
							}
							log.Printf("error reading from upstream: %v", err)
							break
						}
					}
				},
			}, nil
		} else {
			// Proxy the request to the LLM service
			upstreamResp, err := model.ServicePtr.ChatCompletionsRaw(ctx, model, body)
			if err != nil {
				return nil, huma.Error502BadGateway(fmt.Sprintf("LLM service '%s' returned an error: %v",
					model.ServicePtr.GetName(), err))
			}

			return &huma.StreamResponse{
				Body: func(ctx huma.Context) {
					if err := ctx.SetReadDeadline(time.Now().Add(config.GetServerTimeout())); err != nil {
						logging.Error("Failed to set read deadline: %v", err)
					}
					ctx.SetHeader("Content-Type", "application/json")
					if _, err := ctx.BodyWriter().Write(upstreamResp.BodyBytes); err != nil {
						logging.Error("Failed to write response body: %v", err)
					}
				},
			}, nil
		}
	})

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "create-embedding",
		Method:      http.MethodPost,
		Path:        "/v1/embeddings",
		Summary:     "Create embedding",
		Tags:        []string{"OpenAI compatible API"},
	}, func(ctx context.Context, input *struct {
		Body openapi_client.OpenAIEmbeddingRequest
	}) (*OpenAIEmbeddingResponse, error) {
		request := input.Body // read body ASAP, because it has it's own timeout

		// Find the model
		if request.ModelName == "" {
			return nil, huma.Error400BadRequest("the 'model' field is required")
		}

		model, err := GetModelOrFirstLoaded(ctx, request.ModelName)
		if err != nil {
			return nil, err.HumaError()
		}

		if !model.ServicePtr.GetCapabilities().Embedding {
			return nil, huma.Error400BadRequest(fmt.Sprintf("embedding is not supported by the '%s' service", model.ServicePtr.GetName()))
		}

		if model.Type != ModelTypeEmbeddings {
			return nil, huma.Error400BadRequest(fmt.Sprintf("model '%s' is not an embedding model", model.Name))
		}

		ret, errx := model.ServicePtr.GetEmbeddings(ctx, model, &request)
		if errx != nil {
			return nil, huma.Error502BadGateway(fmt.Sprintf("LLM service '%s' returned an error: %v",
				model.ServicePtr.GetName(), errx))
		}

		ret.Model = model.Name

		return &OpenAIEmbeddingResponse{
			Body: *ret,
		}, nil
	})
}
