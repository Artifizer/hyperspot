package llm

import (
	"context"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var GPT4AllServiceName = LLMServiceName("gpt4all")

// GPT4AllService implements the Service interface for GPT4All
type GPT4AllService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewGPT4AllService creates a new GPT4All service
func NewGPT4AllService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *GPT4AllService {
	service := &GPT4AllService{}
	service.BaseLLMService = NewBaseLLMService(service, string(GPT4AllServiceName), "GPT4All", baseURL, serviceConfig)
	service.logger = logger.WithField("service", string(GPT4AllServiceName))
	return service
}

func (s *GPT4AllService) GetName() string {
	return string(GPT4AllServiceName)
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
	config.RegisterLLMServiceConfig(string(GPT4AllServiceName), config.ConfigLLMService{
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
