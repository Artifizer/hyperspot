package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/api_client"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
)

// OllamaService implements the Service interface for Ollama
type OllamaService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewOllamaService creates a new Ollama service
func NewOllamaService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *OllamaService {
	service := &OllamaService{}
	service.BaseLLMService = NewBaseLLMService(service, "ollama", baseURL, serviceConfig)
	service.logger = logger.WithField("service", "ollama")
	return service
}

func (s *OllamaService) GetName() string {
	return "ollama"
}

func (s *OllamaService) InstallModel(ctx context.Context, modelName string, progress chan<- float32) error {
	body := map[string]string{"name": modelName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to marshal request", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/pull", bytes.NewBuffer(jsonBody))
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to create request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.makeHTTPCall(ctx, req)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to install model", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to install model: status %d", resp.StatusCode)}
	}

	return nil
}

func (s *OllamaService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	// Ollama automatically loads models when needed
	return nil
}

func (s *OllamaService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	// Ollama doesn't support explicit unloading
	return nil
}

func (s *OllamaService) DeleteModel(ctx context.Context, modelName string, progress chan<- float32) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("%s/api/delete/%s", s.baseURL, modelName), nil)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to create request", Cause: err}
	}

	resp, err := s.makeHTTPCall(ctx, req)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to delete model", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to delete model: status %d", resp.StatusCode)}
	}

	return nil
}

func (s *OllamaService) ChatCompletionsRaw(
	ctx context.Context,
	model *LLMModel,
	req openapi_client.OpenAICompletionRequestRaw,
) (*api_client.HTTPResponse, error) {
	s.logger.Debug("Sending completion request to Ollama service")
	return s.llmAPIClient.ChatCompletionsRaw(ctx, req)
}

// GetModel returns details about a specific model
func (s *OllamaService) GetModel(ctx context.Context, name string) (*LLMModel, error) {
	return s.GetModelGeneric(ctx, name)
}

var ollamaModelTypes = map[string]string{}

// updateModelType updates the model type based on the model details
func (s *OllamaService) updateModelType(model *LLMModel) {
	if model.Type != "" {
		return
	}

	// Check if we already know this model type from cache
	if cachedType, ok := ollamaModelTypes[model.UpstreamModelName]; ok && cachedType != "" {
		model.Type = cachedType
		return
	}

	ctx := context.Background()
	requestBody, err := json.Marshal(map[string]interface{}{"model": model.UpstreamModelName})
	if err != nil {
		s.logger.Debug("failed to marshal request for model '%s': %v", model.UpstreamModelName, err)
		model.Type = ModelTypeUnknown
		ollamaModelTypes[model.UpstreamModelName] = ModelTypeUnknown
		return
	}

	resp, err := s.llmAPIClient.PostWithAPIKey(ctx, "/api/show", requestBody)

	if err != nil {
		s.logger.Debug("failed to get model details for '%s': %v", model.UpstreamModelName, err)
		model.Type = ModelTypeUnknown
		ollamaModelTypes[model.UpstreamModelName] = ModelTypeUnknown
		return
	}

	// Parse the response to determine model type
	var result map[string]interface{}
	if err := json.Unmarshal(resp.BodyBytes, &result); err != nil {
		s.logger.Debug("failed to parse model details for '%s': %v", model.UpstreamModelName, err)
		model.Type = ModelTypeUnknown
		ollamaModelTypes[model.UpstreamModelName] = ModelTypeUnknown
		return
	}

	// Check if model has specific parameters that indicate its type
	if modelFormat, ok := result["format"].(string); ok {
		if strings.Contains(strings.ToLower(modelFormat), "clip") {
			model.Type = ModelTypeVision
		}
	}

	// Check if model has specific families that indicate its type
	if family, ok := result["family"].(string); ok {
		familyLower := strings.ToLower(family)
		if strings.Contains(familyLower, "embed") {
			model.Type = ModelTypeEmbeddings
		} else if strings.Contains(familyLower, "clip") || strings.Contains(familyLower, "vision") {
			model.Type = ModelTypeVision
		}
	}

	if model.Type == "" {
		model.Type = ModelTypeLLM
	}

	// Store the determined type in the cache for future use
	ollamaModelTypes[model.UpstreamModelName] = model.Type
}

func (s *OllamaService) initModelFromAPIResponse(data map[string]interface{}) (*LLMModel, error) {
	id, ok := data["model"].(string)
	if !ok {
		return nil, fmt.Errorf("'model' field is not found in %v", data)
	}

	model := NewLLMModel(id, s, data)

	if size, ok := data["size"].(int64); ok {
		model.Size = size
	}
	if model.Size == 0 {
		if sizeFloat, ok := data["size"].(float64); ok {
			model.Size = int64(sizeFloat)
		}
	}

	if modifiedAt, ok := data["modified_at"].(string); ok {
		if parsedTime, err := time.Parse(time.RFC3339Nano, modifiedAt); err == nil {
			model.ModifiedAt = parsedTime.UnixMilli()
		} else {
			return nil, fmt.Errorf("failed to parse 'modified_at' field: %v", err)
		}
	}
	if details, ok := data["details"].(map[string]interface{}); ok {
		if parentModel, ok := details["parent_model"].(string); ok {
			model.Description = fmt.Sprintf("Parent Model: %s", parentModel)
		}
		if format, ok := details["format"].(string); ok {
			model.Quantization = format
		}
		if family, ok := details["family"].(string); ok {
			model.Architecture = family
		}
		if families, ok := details["families"].([]interface{}); ok {
			familyNames := make([]string, len(families))
			for i, f := range families {
				if familyName, ok := f.(string); ok {
					familyNames[i] = familyName
				}
			}
			model.Description += fmt.Sprintf(", Families: %s", strings.Join(familyNames, ", "))
		}
		if parameterSize, ok := details["parameter_size"].(string); ok {
			if size, err := strconv.ParseInt(strings.TrimSuffix(parameterSize, "B"), 10, 64); err == nil {
				model.Parameters = size
			}
		}
		if quantizationLevel, ok := details["quantization_level"].(string); ok {
			model.Quantization = quantizationLevel
		}
	}
	model.State = ModelStateUnknown

	s.updateModelType(model)

	return model, nil
}

// ListModels returns a list of all models
func (s *OllamaService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	// First, load all the models available
	resp, err := s.llmAPIClient.GetWithAPIKey(ctx, "/api/tags")
	if err != nil {
		return nil, err
	}

	var result struct {
		Models []map[string]interface{} `json:"models"`
	}

	if err := json.Unmarshal(resp.BodyBytes, &result); err != nil {
		return nil, &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to decode response '%s'", string(resp.BodyBytes)), Cause: err}
	}

	models := make([]*LLMModel, len(result.Models))
	for i, data := range result.Models {
		model, err := s.initModelFromAPIResponse(data)
		if err != nil {
			s.logger.Debug("failed to init model: %v", err)
			continue
		}
		model.State = ModelStateNonLoaded
		models[i] = model
	}

	resp, err = s.llmAPIClient.GetWithAPIKey(ctx, "/api/ps")
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resp.BodyBytes, &result); err != nil {
		return nil, &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to decode response '%s'", string(resp.BodyBytes)), Cause: err}
	}
	for _, data := range result.Models {
		modelName, ok := data["model"].(string)
		if !ok {
			s.logger.Trace("'model' field is not found in %v", data)
			continue
		}

		for _, m := range models {
			if m.UpstreamModelName == modelName {
				m.State = ModelStateLoaded
				break
			}
		}
	}

	// Second, get only loaded models using /api/ps endpoint

	psResp, err := s.llmAPIClient.GetWithAPIKey(ctx, "/api/ps")
	if err != nil {
		s.logger.Warn("failed to get loaded models: %v", err)
		// Continue with the models we have, just don't mark any as loaded
		return api.PageAPIPaginate(models, pageRequest), nil
	}

	if err := json.Unmarshal(psResp.BodyBytes, &result); err != nil {
		s.logger.Warn("failed to decode ps response: %v", err)
		// Continue with the models we have, just don't mark any as loaded
		return api.PageAPIPaginate(models, pageRequest), nil
	}

	// Create a map of loaded model names for quick lookup
	loadedModels := make(map[string]bool)
	for _, model := range result.Models {
		if modelName, ok := model["model"].(string); ok {
			loadedModels[modelName] = true
		}
	}

	// Update model states based on loaded status
	for _, model := range models {
		if loadedModels[model.UpstreamModelName] {
			model.State = ModelStateLoaded
		}
	}

	return api.PageAPIPaginate(models, pageRequest), nil
}

// Ping checks if the Ollama service is available
func (s *OllamaService) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/api/version", nil)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to create request", Cause: err}
	}

	resp, err := s.makeHTTPCall(ctx, req)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "service unavailable", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("service returned status %d", resp.StatusCode)}
	}

	return nil
}

func (s *OllamaService) GetCapabilities() *LLMServiceCapabilities {
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
		OllamaModelsSupported:      true,
		Streaming:                  true,
		Embedding:                  true,
	}
}

func (s *OllamaService) GetEmbeddings(
	ctx context.Context,
	model *LLMModel,
	req *openapi_client.OpenAIEmbeddingRequest,
) (*openapi_client.OpenAIEmbeddingResponse, error) {
	s.logger.Debug("Sending embedding request to Ollama service for model %s", model.UpstreamModelName)

	// Use the base implementation which handles the API call through the OpenAI client
	req2 := *req
	req2.ModelName = model.UpstreamModelName
	return s.llmAPIClient.GetEmbeddings(ctx, req2)
}

func init() {
	config.RegisterLLMServiceConfig("ollama", config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "OLLAMA_API_KEY",
		URLs:         []string{"http://localhost:11434"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  180,
			VerifyCert:      true,
			EnableDiscovery: true,
		},
	})
}
