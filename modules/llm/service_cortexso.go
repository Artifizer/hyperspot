package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
)

var CortexSoServiceName = LLMServiceName("cortexso")

// CortexSoService implements the Service interface for CortexSo
type CortexSoService struct {
	*BaseLLMService
	logger *logging.Logger
}

// NewCortexSoService creates a new CortexSo service
func NewCortexSoService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *CortexSoService {
	service := &CortexSoService{}
	service.BaseLLMService = NewBaseLLMService(service, string(CortexSoServiceName), "Cortex.so", baseURL, serviceConfig)
	service.logger = logger.WithField("service", string(CortexSoServiceName))
	return service
}

func (s *CortexSoService) GetName() string {
	return string(CortexSoServiceName)
}

func splitTableRow(row string) []string {
	parts := strings.Split(row, "|")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ListModels returns a list of all models
func (s *CortexSoService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	models, err := s.ListModelsGeneric(ctx, pageRequest)
	if err != nil {
		return nil, err
	}

	// Run the 'cortex ps' command line to see if model is actuall running
	// Expected output is something like:
	// +--------------------+-----------+--------+-----------+------------------------+
	// | Model              | Engine    | RAM    | VRAM      | Up time                |
	// +--------------------+-----------+--------+-----------+------------------------+
	// | tinyllama:1b-gguf  | llama-cpp | 0.00 B | 671.34 MB | 4 seconds              |
	// +--------------------+-----------+--------+-----------+------------------------+
	// | codestral:22b-gguf | llama-cpp | 0.00 B | 12.42 GB  | 39 minutes, 17 seconds |
	// +--------------------+-----------+--------+-----------+------------------------+

	// Use utils.Exec to run 'cortex ps' and parse the output
	var result utils.ExecResult

	switch runtime.GOOS {
	case "windows":
		result = utils.Exec(ctx, []string{"cortex.exe", "ps"}, "")
	default:
		result = utils.Exec(ctx, []string{"cortex", "ps"}, "")
	}

	if result.ExitCode != 0 {
		s.logger.Debug("cortex ps failed: %v", result.Stderr)
		return models, nil // Still return what we have
	}

	// Parse the output table
	runningModels := map[string]bool{}
	for _, line := range result.Stdout {
		// Skip header lines and separators
		if line == "" || line[0] == '+' || (line[0] == '|' && (len(line) < 10 || strings.Contains(line, "Model") && strings.Contains(line, "Engine"))) {
			continue
		}
		if line[0] != '|' {
			continue
		}
		cols := splitTableRow(line)
		if len(cols) < 1 {
			continue
		}
		modelName := cols[0]
		if modelName != "" {
			runningModels[modelName] = true
		}
	}

	// Update model states
	for _, model := range models {
		if runningModels[model.UpstreamModelName] {
			model.State = ModelStateLoaded
		} else {
			model.State = ModelStateNonLoaded
		}
		if model.Type == "" {
			model.Type = ModelTypeLLM
		}
	}

	return api.PageAPIPaginate(models, pageRequest), nil
}

func (s *CortexSoService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	if progress != nil {
		select {
		case progress <- 0.0:
		default:
		}
	}
	body := map[string]string{"model": model.UpstreamModelName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to marshal request", Cause: err}
	}

	resp, err := s.llmAPIClient.PostWithAPIKey(ctx, s.baseURL+"/v1/models/start", jsonBody)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to unload model", Cause: err}
	}

	if resp.UpstreamResponse.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to unload model: status %d", resp.UpstreamResponse.StatusCode)}
	}

	model.State = ModelStateLoaded

	if progress != nil {
		select {
		case progress <- 1.0:
		default:
		}
	}
	return nil
}

func (s *CortexSoService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	if progress != nil {
		select {
		case progress <- 0.0:
		default:
		}
	}
	body := map[string]string{"model": model.UpstreamModelName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to marshal request", Cause: err}
	}

	resp, err := s.llmAPIClient.PostWithAPIKey(ctx, s.baseURL+"/v1/models/stop", jsonBody)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to unload model", Cause: err}
	}

	if resp.UpstreamResponse.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to unload model: status %d", resp.UpstreamResponse.StatusCode)}
	}

	model.State = ModelStateNonLoaded

	if progress != nil {
		select {
		case progress <- 1.0:
		default:
		}
	}
	return nil
}

func (s *CortexSoService) GetCapabilities() *LLMServiceCapabilities {
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
		HuggingfaceModelsSupported: true,
		OllamaModelsSupported:      false,
		Streaming:                  false,
		Embedding:                  false,
	}
}

// Implement other required methods following the same pattern as LMStudioService
func init() {
	config.RegisterLLMServiceConfig(string(CortexSoServiceName), config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "CORTEXSO_API_KEY",
		URLs:         []string{"http://localhost:39281"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  180,
			VerifyCert:      true,
			EnableDiscovery: true,
		},
	})
}
