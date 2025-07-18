package llm

import (
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// AnthropicService implements the Service interface for Anthropic online LLM model
type AnthropicService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewAnthropicService creates a new Anthropic service
func NewAnthropicService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *AnthropicService {
	service := &AnthropicService{}
	service.BaseLLMService = NewBaseLLMService(service, "anthropic", baseURL, serviceConfig)
	service.logger = logger.WithField("service", "anthropic")
	return service
}

func (s *AnthropicService) GetName() string {
	return "anthropic"
}

func (s *AnthropicService) IsExternal() bool {
	return true
}

func (s *AnthropicService) GetCapabilities() *LLMServiceCapabilities {
	return &LLMServiceCapabilities{
		InstallModel:               false,
		ImportModel:                false,
		UpdateModel:                false,
		LoadModel:                  false,
		UnloadModel:                false,
		DeleteModel:                false,
		HuggingfaceModelsSupported: false,
		OllamaModelsSupported:      false,
		Streaming:                  false,
		Embedding:                  false,
	}
}

// Implement other required methods following the same pattern as LMStudioService
func init() {
	config.RegisterLLMServiceConfig("anthropic", config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "CLAUDE_API_KEY",
		URLs:         []string{"https://api.anthropic.com"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  60,
			VerifyCert:      true,
			EnableDiscovery: false,
		},
	})
}
