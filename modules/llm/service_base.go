package llm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/api_client"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
	"github.com/hypernetix/hyperspot/modules/syscap"
)

// LLMServiceName represents the name of the LLM service
type LLMServiceName string

// BaseLLMService provides common functionality for LLM services
type BaseLLMService struct {
	LLMServicePtr LLMService
	syscap        *syscap.SysCap
	Name          string
	APIKeyEnvVar  string
	baseURL       string
	HttpClient    *http.Client
	llmAPIClient  *openapi_client.OpenAIAPIClient
	logger        *logging.Logger
	mu            sync.RWMutex
}

// NewBaseLLMService creates a new base service
func NewBaseLLMService(LLMServicePtr LLMService, name string, displayName string, baseURL string, serviceConfig config.ConfigLLMService) *BaseLLMService {
	srv := &BaseLLMService{
		LLMServicePtr: LLMServicePtr,
		syscap:        registerLLMServiceSysCap(LLMServicePtr, name, displayName),
		Name:          name,
		baseURL:       baseURL,
		APIKeyEnvVar:  serviceConfig.APIKeyEnvVar,
		HttpClient: &http.Client{
			Timeout: time.Duration(serviceConfig.Upstream.ShortTimeoutSec) * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: time.Duration(serviceConfig.Upstream.ShortTimeoutSec) * time.Second,
				ExpectContinueTimeout: time.Duration(serviceConfig.Upstream.ShortTimeoutSec) * time.Second,
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: !serviceConfig.Upstream.VerifyCert,
				},
			},
		},
	}

	// Create appropriate API client based on API format
	switch serviceConfig.APIFormat {
	case "openai":
		// Default to OpenAI format
		srv.llmAPIClient = openapi_client.NewOpenAIAPIClient(
			name,
			baseURL,
			serviceConfig.APIKey,
			serviceConfig.APIKeyEnvVar,
			serviceConfig.Upstream.ShortTimeoutSec,
			serviceConfig.Upstream.LongTimeoutSec,
			serviceConfig.Upstream.VerifyCert,
			serviceConfig.Upstream.EnableDiscovery,
		)
	// Add other API formats here when needed
	default:
		// Default to OpenAI format
		srv.llmAPIClient = openapi_client.NewOpenAIAPIClient(
			name,
			baseURL,
			serviceConfig.APIKey,
			serviceConfig.APIKeyEnvVar,
			serviceConfig.Upstream.ShortTimeoutSec,
			serviceConfig.Upstream.LongTimeoutSec,
			serviceConfig.Upstream.VerifyCert,
			serviceConfig.Upstream.EnableDiscovery,
		)
	}

	return srv
}

// GetName returns the service name
func (s *BaseLLMService) GetName() string {
	return s.Name
}

func (s *BaseLLMService) GetBaseURL() string {
	return s.baseURL
}

func (s *BaseLLMService) GetFullModelName(modelName string) string {
	return s.LLMServicePtr.GetName() + ServiceNameSeparator + modelName
}

// Ping checks if the service is available using the health check endpoint
func (s *BaseLLMService) Ping(ctx context.Context) error {
	return s.llmAPIClient.Ping(ctx)
}

func (s *BaseLLMService) ChatCompletionsStreamRaw(
	ctx context.Context,
	model *LLMModel,
	req openapi_client.OpenAICompletionRequestRaw,
) (*http.Response, error) {
	req["stream"] = true

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return s.llmAPIClient.RequestWithLongTimeout(ctx, "POST", "/v1/chat/completions", jsonBody)
}

func (s *BaseLLMService) ChatCompletionsRaw(
	ctx context.Context,
	model *LLMModel,
	req openapi_client.OpenAICompletionRequestRaw,
) (*api_client.HTTPResponse, error) {
	modelName := model.UpstreamModelName
	if modelName == "" {
		return nil, fmt.Errorf("sending completion request to service: '%s', model is required", s.GetName())
	}

	logging.Debug("Sending completion request to service: '%s', model: '%s'", s.GetName(), modelName)

	// Handle streaming responses
	if req["stream"] == true {
		return nil, fmt.Errorf("streaming responses are not supported, use ChatCompletionsStream() instead")
	}

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	upstreamResp, err := s.llmAPIClient.PostWithAPIKeyAndLongTimeout(ctx, "/v1/chat/completions", jsonBody)
	if err != nil {
		msg := fmt.Sprintf("completion request failed: %s", err.Error())
		logging.Debug("Completion request failed: %s", msg)
		return nil, fmt.Errorf("%s", msg)
	}

	if upstreamResp.UpstreamResponse.StatusCode == http.StatusOK {
		var respData openapi_client.OpenAICompletionResponseRaw
		if err := json.Unmarshal(upstreamResp.BodyBytes, &respData); err != nil {
			return nil, fmt.Errorf("failed to decode upstream response: %w", err)
		}

		// Update the model value to current ModelName
		respData["model"] = model.Name

		// Convert back to bytes
		updatedBody, err := json.Marshal(respData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal updated response: %w", err)
		}

		upstreamResp.BodyBytes = updatedBody
	}

	return upstreamResp, nil
}

func (s *BaseLLMService) ChatCompletions(
	ctx context.Context,
	model *LLMModel,
	req *openapi_client.OpenAICompletionRequest,
) (*openapi_client.OpenAICompletionResponse, error) {
	resp, err := s.LLMServicePtr.ChatCompletionsRaw(ctx, model, req.ToMap())
	if err != nil {
		return nil, err
	}

	var respObj openapi_client.OpenAICompletionResponse
	respObj.DurationSec = float32(resp.Duration.Seconds())

	if err := json.Unmarshal(resp.BodyBytes, &respObj); err != nil {
		return nil, fmt.Errorf("error decoding response: %v %s", err, resp.BodyBytes)
	}
	respObj.HttpResp = resp

	if len(respObj.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned")
	}

	choice := respObj.Choices[0]
	if choice.Message.Content == "" {
		return nil, fmt.Errorf("no content returned")
	}

	return &respObj, nil
}

func (s *BaseLLMService) GetEmbeddings(
	ctx context.Context,
	model *LLMModel,
	req *openapi_client.OpenAIEmbeddingRequest,
) (*openapi_client.OpenAIEmbeddingResponse, error) {
	req2 := *req
	req2.ModelName = model.UpstreamModelName
	return s.llmAPIClient.GetEmbeddings(ctx, req2)
}

// LikelyAlive returns true if the service was responsive in its last interaction
func (s *BaseLLMService) LikelyIsAlive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.llmAPIClient.LikelyIsAlive()
}

// setLikelyAlive updates the service's alive status
func (s *BaseLLMService) setLikelyIsAlive() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.syscap.SetPresent(true)
	s.llmAPIClient.SetUpstreamLikelyIsOnline()
}

func (s *BaseLLMService) setLikelyIsOffline(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.syscap.SetPresent(false)
	s.llmAPIClient.SetUpstreamLikelyIsOffline(ctx)
}

// makeHTTPCall wraps an HTTP call and updates the LikelyAlive status based on the response
func (s *BaseLLMService) makeHTTPCall(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := s.HttpClient.Do(req)
	if err != nil {
		s.setLikelyIsOffline(ctx)
		return nil, err
	}

	// Consider successful if status code is 2xx
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.setLikelyIsAlive()
	} else {
		s.setLikelyIsOffline(ctx)
	}
	return resp, err
}

func (s *BaseLLMService) ImportModel(ctx context.Context, modelName string, progress chan<- float32) error {
	return &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "model import for given LLM service is not supported"}
}

func (s *BaseLLMService) InstallModel(ctx context.Context, modelName string, progress chan<- float32) error {
	return &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "model installation for given LLM service is not supported"}
}

func (s *BaseLLMService) UpdateModel(ctx context.Context, modelName string, progress chan<- float32) error {
	return &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "model update for given LLM service is not supported"}
}

func (s *BaseLLMService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	return &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "model loading for given LLM service is not supported"}
}

func (s *BaseLLMService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	return &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "model unloading for given LLM service is not supported"}
}

func (s *BaseLLMService) DeleteModel(ctx context.Context, modelName string, progress chan<- float32) error {
	return &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "model deletion for given LLM service is not supported"}
}

// TODO: name argument is not used, consider removing it
func (s *BaseLLMService) initModelFromAPIResponse(name string, data map[string]interface{}) (*LLMModel, error) {
	// Try different possible model name fields
	if id, ok := data["id"].(string); ok {
		name = id
	} else if mname, ok := data["name"].(string); ok {
		name = mname
	} else if model, ok := data["model"].(string); ok {
		name = model
	} else {
		return nil, fmt.Errorf("failed to parse model name")
	}

	model := NewLLMModel(name, s.LLMServicePtr, data)

	if engine, ok := data["engine"].(string); ok {
		model.RuntimeName = engine
		model.RuntimePtr = LLMRuntime{Name: engine}
	}
	if architecture, ok := data["arch"].(string); ok {
		model.Architecture = architecture
	}
	if quantization, ok := data["quantization"].(string); ok {
		model.Quantization = quantization
	}
	if compatibilityType, ok := data["compatibility_type"].(string); ok {
		if compatibilityType == "gguf" {
			model.IsGGUF = true
		} else if compatibilityType == "mlx" {
			model.IsMLX = true
		}
	}
	if publisher, ok := data["publisher"].(string); ok {
		model.Description = fmt.Sprintf("Publisher: %s", publisher)
	}
	if state, ok := data["state"].(string); ok {
		model.State = ModelState(state)
	} else {
		model.State = ModelStateUnknown
	}

	if s.LLMServicePtr.GetCapabilities().Streaming {
		if stream, ok := data["stream"].(bool); ok {
			model.Streaming = stream
		}
		if streaming, ok := data["streaming"].(bool); ok {
			model.Streaming = streaming
		}
	} else {
		model.Streaming = false
	}

	if size, ok := data["size"].(int64); ok {
		model.Size = size
	}
	if model.Size == 0 {
		if sizeFloat, ok := data["size"].(float64); ok {
			model.Size = int64(sizeFloat)
		}
	}

	return model, nil
}

// GetModel returns details about a specific model
func (s *BaseLLMService) GetModelGeneric(ctx context.Context, name string) (*LLMModel, error) {
	if s.llmAPIClient == nil {
		return nil, fmt.Errorf("no API client configured")
	}

	resp, err := s.llmAPIClient.GetModel(ctx, name)
	if err != nil {
		return nil, &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "failed to get model details", Cause: err}
	}

	var modelData map[string]interface{}
	if err := json.Unmarshal(resp.BodyBytes, &modelData); err != nil {
		return nil, &LLMServiceError{Service: s.LLMServicePtr.GetName(), Message: "invalid model data format"}
	}

	return s.initModelFromAPIResponse(name, modelData)
}

func (s *BaseLLMService) GetModel(ctx context.Context, name string) (*LLMModel, error) {
	return s.GetModelGeneric(ctx, name)
}

func (s *BaseLLMService) ListModelsGeneric(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	if s.llmAPIClient == nil {
		return nil, fmt.Errorf("no API client configured")
	}

	resp, err := s.llmAPIClient.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(resp.BodyBytes, &result); err != nil {
		return nil, &LLMServiceError{Service: s.GetName(), Message: "failed to decode response", Cause: err}
	}

	models := make([]*LLMModel, len(result.Data))
	for i, m := range result.Data {
		var modelName string
		// Try different possible model name fields
		if id, ok := m["id"].(string); ok {
			modelName = id
		} else if name, ok := m["name"].(string); ok {
			modelName = name
		} else if model, ok := m["model"].(string); ok {
			modelName = model
		} else {
			modelName = fmt.Sprintf("model_%d", i)
		}
		model, err := s.initModelFromAPIResponse(modelName, m)
		if err != nil {
			continue
		}
		models[i] = model

		// For external services, mark all discovered models as loaded
		if s.LLMServicePtr.IsExternal() {
			model.State = ModelStateLoaded
		}
	}

	return api.PageAPIPaginate(models, pageRequest), nil
}

func (s *BaseLLMService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	return s.ListModelsGeneric(ctx, pageRequest)
}

// IsExternal returns true if the service requires an API key
func (s *BaseLLMService) IsExternal() bool {
	return false
}

func (s *BaseLLMService) GetCapabilities() *LLMServiceCapabilities {
	return &LLMServiceCapabilities{
		InstallModel:               false,
		ImportModel:                false,
		UpdateModel:                false,
		LoadModel:                  false,
		LoadModelProgress:          false,
		LoadModelCancel:            false,
		LoadModelTimeout:           false,
		UnloadModel:                false,
		DeleteModel:                false,
		HuggingfaceModelsSupported: false,
		OllamaModelsSupported:      false,
	}
}

// LLMService defines the interface for LLM services
type LLMService interface {
	GetName() string
	GetModel(ctx context.Context, name string) (*LLMModel, error)
	InstallModel(ctx context.Context, name string, progress chan<- float32) error    // install a model from a remote website
	ImportModel(ctx context.Context, name string, progress chan<- float32) error     // import a model from a file
	UpdateModel(ctx context.Context, name string, progress chan<- float32) error     // update a model from a remote website
	LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error   // load installed/imported model into memory
	UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error // unload model from memory
	DeleteModel(ctx context.Context, name string, progress chan<- float32) error     // delete model permanently
	Ping(ctx context.Context) error
	LikelyIsAlive() bool
	ChatCompletionsRaw(ctx context.Context, model *LLMModel, req openapi_client.OpenAICompletionRequestRaw) (*api_client.HTTPResponse, error)
	ChatCompletionsStreamRaw(ctx context.Context, model *LLMModel, req openapi_client.OpenAICompletionRequestRaw) (*http.Response, error)
	ChatCompletions(ctx context.Context, model *LLMModel, req *openapi_client.OpenAICompletionRequest) (*openapi_client.OpenAICompletionResponse, error)
	GetEmbeddings(ctx context.Context, model *LLMModel, req *openapi_client.OpenAIEmbeddingRequest) (*openapi_client.OpenAIEmbeddingResponse, error)
	ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error)
	GetBaseURL() string
	IsExternal() bool
	GetCapabilities() *LLMServiceCapabilities
}

// LLMServiceError represents an error from an LLM service
type LLMServiceError struct {
	Service string
	Message string
	Cause   error
}

type LLMServiceCapabilities struct {
	InstallModel               bool `json:"install_model" doc:"Whether the service can install a model from a remote website"`
	ImportModel                bool `json:"import_model" doc:"Whether the service can import a model from a file"`
	UpdateModel                bool `json:"update_model" doc:"Whether the service can update a model from a remote website"`
	LoadModel                  bool `json:"load_model" doc:"Whether the service can load a model into memory"`
	LoadModelProgress          bool `json:"load_model_progress" doc:"Whether the service can report progress during model loading"`
	LoadModelCancel            bool `json:"load_model_cancel" doc:"Whether the service can cancel model loading"`
	LoadModelTimeout           bool `json:"load_model_timeout" doc:"Whether the service can handle a timeout for model loading"`
	UnloadModel                bool `json:"unload_model" doc:"Whether the service can unload a model from memory"`
	DeleteModel                bool `json:"delete_model" doc:"Whether the service can delete a model permanently"`
	HuggingfaceModelsSupported bool `json:"huggingface_models_supported" doc:"Whether the service supports Huggingface models"`
	OllamaModelsSupported      bool `json:"ollama_models_supported" doc:"Whether the service supports Ollama models"`
	Streaming                  bool `json:"streaming" doc:"Whether the service supports completion streaming"`
	Embedding                  bool `json:"embedding" doc:"Whether the service supports embedding"`
}

func (e *LLMServiceError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Service, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Service, e.Message)
}
