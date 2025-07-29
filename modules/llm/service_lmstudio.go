package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/lmstudio-go/pkg/lmstudio"
)

var LMStudioServiceName = LLMServiceName("lm_studio")
var useWebSocketParam = "use_web_socket"

type lmstudioLogger struct {
	logger *logging.Logger
}

func (l *lmstudioLogger) SetLevel(level lmstudio.LogLevel) {
	// ignore
}

func (l *lmstudioLogger) Error(format string, v ...interface{}) {
	l.logger.Error(format, v...)
}

func (l *lmstudioLogger) Warn(format string, v ...interface{}) {
	l.logger.Warn(format, v...)
}

func (l *lmstudioLogger) Info(format string, v ...interface{}) {
	l.logger.Info(format, v...)
}

func (l *lmstudioLogger) Debug(format string, v ...interface{}) {
	l.logger.Debug(format, v...)
}

func (l *lmstudioLogger) Trace(format string, v ...interface{}) {
	l.logger.Trace(format, v...)
}

// LMStudioService implements the Service interface for LM Studio
type LMStudioService struct {
	*BaseLLMService
	logger               *logging.Logger
	lmstudioClientLogger *lmstudioLogger
	lmstudioClient       *lmstudio.LMStudioClient
}

// NewLMStudioService creates a new LM Studio service
func NewLMStudioService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *LMStudioService {
	service := &LMStudioService{}
	service.BaseLLMService = NewBaseLLMService(service, string(LMStudioServiceName), "LM Studio", baseURL, serviceConfig)
	service.logger = logger.WithField(logging.ServiceField, string(LMStudioServiceName))
	service.lmstudioClientLogger = &lmstudioLogger{logger: service.logger}
	service.lmstudioClient = lmstudio.NewLMStudioClient(baseURL, service.lmstudioClientLogger)
	return service
}

func (s *LMStudioService) GetName() string {
	return string(LMStudioServiceName)
}

func (s *LMStudioService) initModelFromAPIResponse(data map[string]interface{}) (*LLMModel, error) {
	id, ok := data["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is not a string")
	}

	model := NewLLMModel(id, s, data)

	if mtype, ok := data["type"].(string); ok {
		if mtype == "llm" {
			model.Type = ModelTypeLLM
		} else if mtype == "vlm" {
			model.Type = ModelTypeVLM
		} else if mtype == "embeddings" {
			model.Type = ModelTypeEmbeddings
		} else if mtype == "vision" {
			model.Type = ModelTypeVision
		} else {
			s.logger.Debug("failed to recognize '%s' service model type: %s", s.GetName(), mtype)
			model.Type = ModelTypeUnknown
		}
	}
	if publisher, ok := data["publisher"].(string); ok {
		model.Publisher = publisher
		if strings.Contains(strings.ToLower(publisher), "mlx-community") {
			model.IsMLX = true
		}
	}
	if architecture, ok := data["arch"].(string); ok {
		model.Architecture = architecture
	}
	if compatibilityType, ok := data["compatibility_type"].(string); ok {
		if compatibilityType == "mlx" {
			model.IsMLX = true
		} else if compatibilityType == "gguf" {
			model.IsGGUF = true
		}
	}
	if quantization, ok := data["quantization"].(string); ok {
		model.Quantization = quantization
	}
	if state, ok := data["state"].(string); ok {
		if state == "loaded" {
			model.State = ModelStateLoaded
		} else {
			model.State = ModelStateNonLoaded
		}
	}
	if maxTokens, ok := data["max_context_length"].(float64); ok {
		model.MaxTokens = int(maxTokens)
	} else if maxTokensInt, ok := data["max_context_length"].(int); ok {
		model.MaxTokens = maxTokensInt
	}

	return model, nil
}

func (s *LMStudioService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	resp, err := s.llmAPIClient.GetWithAPIKey(ctx, "/api/v0/models")
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(resp.BodyBytes, &result); err != nil {
		return nil, &LLMServiceError{Service: s.GetName(), Message: "failed to decode response", Cause: err}
	}

	// Put data into a hash - model name -> model info
	modelInfoMap := make(map[string]*lmstudio.Model)
	lmstudioModels, err := s.lmstudioClient.ListDownloadedModels()
	if err != nil {
		s.logger.Debug("failed to get models from lmstudio-go client: %v", err)
		// Continue with API response data even if lmstudio-go client fails
	} else {
		// Create a map of model name to model info
		for _, lmModel := range lmstudioModels {
			modelInfoMap[lmModel.ModelKey] = &lmModel
		}
	}

	models := make([]*LLMModel, 0)
	for _, bytes := range result.Data {
		model, err := s.initModelFromAPIResponse(bytes)
		if err != nil {
			s.logger.Debug("failed to init model: %v", err)
			continue
		}

		// Enrich model with additional information from lmstudio-go client
		if lmModel, exists := modelInfoMap[model.UpstreamModelName]; exists {
			// Enrich with additional information
			model.Size = lmModel.Size
			if lmModel.Format != "" {
				if model.Architecture == "" || model.Architecture == "unknown" {
					model.Architecture = lmModel.Format
				}
			}
			model.FilePath = lmModel.Path

			// Set max context length if not already set
			if model.MaxTokens == 0 && lmModel.MaxContextLength > 0 {
				model.MaxTokens = lmModel.MaxContextLength
			}
		}

		models = append(models, model)
	}

	return api.PageAPIPaginate(models, pageRequest), nil
}

func (s *LMStudioService) GetModel(ctx context.Context, modelName string) (*LLMModel, error) {
	// LM Studio manages model installation through its GUI
	resp, err := s.llmAPIClient.GetWithAPIKey(ctx, fmt.Sprintf("/api/v0/models/%s", modelName))
	if err != nil {
		return nil, err
	}
	var modelData map[string]interface{}
	if err := json.Unmarshal(resp.BodyBytes, &modelData); err != nil {
		return nil, &LLMServiceError{Service: s.GetName(), Message: "failed to decode response", Cause: err}
	}
	return s.initModelFromAPIResponse(modelData)
}

func (s *LMStudioService) loadModelThroughHTTP(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	body := map[string]string{"model": model.UpstreamModelName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to marshal request", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/models/load", bytes.NewBuffer(jsonBody))
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to create request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.makeHTTPCall(ctx, req)

	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to load model", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to load model: status %d", resp.StatusCode)}
	}

	// Send 100% progress at the end if channel is available
	if progress != nil {
		select {
		case progress <- 100.0:
			// Final progress sent successfully
		default:
			// Channel is full or closed
		}
	}

	return nil
}

func (s *LMStudioService) loadModelThroughWebSocket(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	// Use the lmstudio-go library to load the model
	err := s.lmstudioClient.LoadModelWithProgressContext(ctx, 300*time.Second, model.UpstreamModelName, func(progr float64, modelInfo *lmstudio.Model) {
		if progress != nil {
			// Report progress as a percentage from 0.0 to 100.0
			progressValue := float32(progr) * 100
			select {
			case progress <- progressValue:
				// Progress sent successfully
			default:
				// Channel is full or closed, just log and continue
				s.logger.Debug("Could not send progress update: %v", progressValue)
			}

			// Log the progress information
			s.logger.Debug("Loading model %s: %d%% complete", model.UpstreamModelName, progress)
		}
	})
	if err != nil {
		s.logger.Debug("failed to load model '%s' through WebSocket: %v, %s", model.UpstreamModelName, err, err.Error())
		return &LLMServiceError{Service: s.GetName(), Message: "failed to load model", Cause: err}
	}

	// Send 100% progress at the end if channel is available
	if progress != nil {
		select {
		case progress <- 100.0:
			// Final progress sent successfully
		default:
			// Channel is full or closed
		}
	}

	return nil
}

func (s *LMStudioService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	if s.ConfigParams != nil {
		if useWebSocket, ok := s.ConfigParams[useWebSocketParam].(bool); ok && useWebSocket {
			return s.loadModelThroughWebSocket(ctx, model, progress)
		}
	}
	return s.loadModelThroughHTTP(ctx, model, progress)
}

func (s *LMStudioService) unloadModelThroughHTTP(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/models/unload", nil)
	if err != nil {
		return &LLMServiceError{Service: s.GetName(), Message: "failed to create request", Cause: err}
	}

	resp, err := s.makeHTTPCall(ctx, req)
	if err != nil {
		s.logger.Debug("failed to unload model '%s' through HTTP: %v", model.UpstreamModelName, err)
		return &LLMServiceError{Service: s.GetName(), Message: "failed to unload model", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &LLMServiceError{Service: s.GetName(), Message: fmt.Sprintf("failed to unload model: status %d", resp.StatusCode)}
	}

	return nil
}

func (s *LMStudioService) unloadModelThroughWebSocket(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	// Use the lmstudio-go library to load the model
	err := s.lmstudioClient.UnloadModel(model.UpstreamModelName)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no model found") {
			// Model not found, ignore
			s.logger.Trace("model '%s' not loaded, ignoring unload request", model.UpstreamModelName)
		} else {
			s.logger.Debug("failed to unload model '%s' through WebSocket: %v", model.UpstreamModelName, err)
			return &LLMServiceError{Service: s.GetName(), Message: "failed to unload model", Cause: err}
		}
	}
	if progress != nil {
		select {
		case progress <- 100.0:
			// Progress sent successfully
		default:
			// Channel is full or closed, just log and continue
			s.logger.Debug("Could not send 100%% progress update on model '%s' unload", model.UpstreamModelName)
		}
	}

	return nil
}

func (s *LMStudioService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	if s.ConfigParams != nil {
		if useWebSocket, ok := s.ConfigParams[useWebSocketParam].(bool); ok && useWebSocket {
			return s.unloadModelThroughWebSocket(ctx, model, progress)
		}
	}
	return s.unloadModelThroughHTTP(ctx, model, progress)
}

func (s *LMStudioService) DeleteModel(ctx context.Context, modelName string, progress chan<- float32) error {
	// LM Studio manages model deletion through its GUI
	return &LLMServiceError{Service: s.GetName(), Message: "model deletion not supported via API"}
}

// Ping checks if the LM Studio service is available
func (s *LMStudioService) Ping(ctx context.Context) error {
	err := s.llmAPIClient.Ping(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *LMStudioService) GetCapabilities() *LLMServiceCapabilities {
	return &LLMServiceCapabilities{
		InstallModel:               false,
		ImportModel:                false,
		UpdateModel:                false,
		LoadModel:                  true,
		LoadModelProgress:          true,
		LoadModelCancel:            true,
		LoadModelTimeout:           true,
		UnloadModel:                true,
		DeleteModel:                false,
		HuggingfaceModelsSupported: true,
		OllamaModelsSupported:      false,
		Streaming:                  true,
		Embedding:                  true,
	}
}

func init() {
	config.RegisterLLMServiceConfig(string(LMStudioServiceName), config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "LMSTUDIO_API_KEY",
		URLs:         []string{"http://localhost:1234", "http://localhost:12345"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 5,
			LongTimeoutSec:  180,
			VerifyCert:      true,
			EnableDiscovery: true,
		},
		Params: map[string]interface{}{
			useWebSocketParam: true, // use web socket by default
		},
	})
}
