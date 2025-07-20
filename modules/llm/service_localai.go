package llm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/api_client"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
)

var LocalAIServiceName = LLMServiceName("localai")

// LocalAIService implements the Service interface for LocalAI
type LocalAIService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewLocalAIService creates a new LocalAI service
func NewLocalAIService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *LocalAIService {
	service := &LocalAIService{}
	service.BaseLLMService = NewBaseLLMService(service, string(LocalAIServiceName), "LocalAI", baseURL, serviceConfig)
	service.logger = logger.WithField("service", string(LocalAIServiceName))
	return service
}

func (s *LocalAIService) GetName() string {
	return string(LocalAIServiceName)
}

func (s *LocalAIService) InstallModel(ctx context.Context, modelName string, progress chan<- float32) error {
	// LocalAI requires models to be installed manually in its model directory
	return &LLMServiceError{Service: s.GetName(), Message: "model installation not supported via API"}
}

func (s *LocalAIService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	// LocalAI loads models automatically when needed
	return nil
}

func (s *LocalAIService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	// LocalAI manages model unloading automatically
	return nil
}

func (s *LocalAIService) DeleteModel(ctx context.Context, modelName string, progress chan<- float32) error {
	// LocalAI requires models to be deleted manually from its model directory
	return &LLMServiceError{Service: s.GetName(), Message: "model deletion not supported via API"}
}

func (s *LocalAIService) ChatCompletionsRaw(
	ctx context.Context,
	model *LLMModel,
	req openapi_client.OpenAICompletionRequestRaw,
) (*api_client.HTTPResponse, error) {
	s.logger.Debug("Sending completion request to LocalAI service")

	return s.llmAPIClient.ChatCompletionsRaw(ctx, req)
}

// GetModel returns details about a specific model
func (s *LocalAIService) GetModel(ctx context.Context, name string) (*LLMModel, error) {
	// Get model details from LocalAI
	return s.GetModelGeneric(ctx, name)
}

// ListModels returns a list of all models
func (s *LocalAIService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	models, err := s.ListModelsGeneric(ctx, pageRequest)
	for _, model := range models {
		model.State = ModelStateLoaded
		if model.Type == "" {
			model.Type = ModelTypeLLM
		}
	}
	return models, err
}

// Ping checks if the LocalAI service is available
func (s *LocalAIService) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/v1/health", nil)
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

func (s *LocalAIService) GetCapabilities() *LLMServiceCapabilities {
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
		Streaming:                  false,
		Embedding:                  false,
	}
}

func init() {
	config.RegisterLLMServiceConfig(string(LocalAIServiceName), config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "LOCALAI_API_KEY",
		URLs:         []string{"http://localhost:8080", "http://localhost:8081"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  180,
			VerifyCert:      true,
			EnableDiscovery: true,
		},
	})
}
