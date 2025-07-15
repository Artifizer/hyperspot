package llm

import (
	"context"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// GPT4AllService implements the Service interface for GPT4All
type GPT4AllService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewGPT4AllService creates a new GPT4All service
func NewGPT4AllService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *GPT4AllService {
	service := &GPT4AllService{}
	service.BaseLLMService = NewBaseLLMService(service, "gpt4all", baseURL, serviceConfig)
	service.logger = logger.WithField("service", "gpt4all")
	return service
}

func (s *GPT4AllService) GetName() string {
	return "gpt4all"
}

func (s *GPT4AllService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	models, err := s.ListModelsGeneric(ctx, pageRequest)
	for _, model := range models {
		model.State = ModelStateLoaded
		if model.Type == "" {
			model.Type = ModelTypeLLM
		}
	}
	return models, err
}

func (s *GPT4AllService) GetCapabilities() *LLMServiceCapabilities {
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
	config.RegisterLLMServiceConfig("gpt4all", config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "GPT4ALL_API_KEY",
		URLs:         []string{"http://localhost:4891"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  180,
			VerifyCert:      true,
			EnableDiscovery: true,
		},
	})
}
