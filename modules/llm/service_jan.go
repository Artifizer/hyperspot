package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var JanServiceName = LLMServiceName("jan")

// JanService implements the Service interface for Jan.ai
type JanService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewJanService creates a new Jan service
func NewJanService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *JanService {
	service := &JanService{}
	service.BaseLLMService = NewBaseLLMService(service, string(JanServiceName), "Jan", baseURL, serviceConfig)
	service.logger = logger.WithField("service", string(JanServiceName))
	return service
}

func (s *JanService) GetName() string {
	return string(JanServiceName)
}

func (s *JanService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	models, err := s.ListModelsGeneric(ctx, nil) // Get all models, because we are going to filter it in memory
	if err != nil {
		return nil, err
	}

	filteredModels := make([]*LLMModel, 0)

	for _, model := range models {
		_, ok := model.rawOpenAPIResponse["status"].(string)
		if !ok {
			s.logger.Debug("model %s has no status, skipping", model.Name)
			continue
		}
		model.State = ModelStateNonLoaded // TODO: Jan API doesnt allow to check model status

		filteredModels = append(filteredModels, model)
		if model.Type == "" {
			model.Type = ModelTypeLLM
		}
	}

	return api.PageAPIPaginate(filteredModels, pageRequest), nil
}

func (s *JanService) InstallModel(ctx context.Context, modelName string, progress chan<- float32) error {
	// Jan manages model installation through its GUI
	return &LLMServiceError{Service: s.GetName(), Message: "model installation not supported via API"}
}

func (s *JanService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	body := map[string]string{"model": model.UpstreamModelName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to marshal request", Cause: err}
	}

	resp, err := s.llmAPIClient.PostWithAPIKey(ctx, "/v1/models/start", jsonBody)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to load model", Cause: err}
	}

	if resp.UpstreamResponse.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to load model: status %d", resp.UpstreamResponse.StatusCode)}
	}

	model.State = ModelStateLoaded

	return nil
}

func (s *JanService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	body := map[string]string{"model": model.UpstreamModelName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to marshal request", Cause: err}
	}

	resp, err := s.llmAPIClient.PostWithAPIKey(ctx, "/v1/models/stop", jsonBody)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to unload model", Cause: err}
	}

	if resp.UpstreamResponse.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to unload model: status %d", resp.UpstreamResponse.StatusCode)}
	}

	model.State = ModelStateNonLoaded

	return nil
}

func (s *JanService) DeleteModel(ctx context.Context, modelName string, progress chan<- float32) error {
	// Jan manages model deletion through its GUI
	return &LLMServiceError{Service: s.GetName(), Message: "model deletion not supported via API"}
}

// GetModel returns details about a specific model
func (s *JanService) GetModel(ctx context.Context, name string) (*LLMModel, error) {
	return s.GetModelGeneric(ctx, name)
}

func (s *JanService) GetCapabilities() *LLMServiceCapabilities {
	return &LLMServiceCapabilities{
		InstallModel:               false,
		ImportModel:                false,
		UpdateModel:                false,
		LoadModel:                  true,
		LoadModelProgress:          false,
		LoadModelCancel:            false,
		LoadModelTimeout:           false,
		UnloadModel:                true,
		DeleteModel:                false,
		HuggingfaceModelsSupported: false,
		OllamaModelsSupported:      false,
		Streaming:                  false,
		Embedding:                  false,
	}
}

func init() {
	config.RegisterLLMServiceConfig(string(JanServiceName), config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "JAN_API_KEY",
		URLs:         []string{"http://localhost:1337", "http://localhost:1338"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  180,
			VerifyCert:      true,
			EnableDiscovery: true,
		},
	})
}
