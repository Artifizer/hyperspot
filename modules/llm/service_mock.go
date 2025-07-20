package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/api_client"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
	"github.com/hypernetix/hyperspot/libs/utils"
)

var MockServiceName = LLMServiceName("mock")

// MockService implements the LLMService interface for testing purposes
type MockService struct {
	*BaseLLMService
	logger         *logging.Logger
	mu             utils.DebugMutex
	PingShouldFail bool
	PingCount      int
}

// NewMockService creates a new mock service
func NewMockService(baseURL string, serviceConfig config.ConfigLLMService, logger *logging.Logger) *MockService {
	service := &MockService{}
	service.BaseLLMService = NewBaseLLMService(service, string(MockServiceName), "Mock Server", baseURL, serviceConfig)
	service.logger = logger.WithField("service", string(MockServiceName))
	return service
}

func (s *MockService) GetName() string {
	return string(MockServiceName)
}

// IsExternal returns false as mock service is internal
func (s *MockService) IsExternal() bool {
	return false
}

var mockModels = map[string]*LLMModel{}

// ListModels overrides the base method to return mock models
func (s *MockService) ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]*LLMModel, error) {
	s.logger.Debug("Listing mock models")

	if len(mockModels) == 0 {
		s.mu.Lock()
		defer s.mu.Unlock()
		mockModels["mock-model-0.5B"] = s.createMockModel("mock-model-0.5B", 500000000, 2048, true, ModelTypeLLM)
		mockModels["mock-model-7B"] = s.createMockModel("mock-model-7B", 7000000000, 4096, true, ModelTypeLLM)
		mockModels["mock-model-30B"] = s.createMockModel("mock-model-30B", 30000000000, 8192, false, ModelTypeLLM)
		mockModels["mock-model-embeddings"] = s.createMockModel("mock-model-embeddings", 0, 0, true, ModelTypeEmbeddings)
	}

	models := make([]*LLMModel, 0, len(mockModels))
	for _, model := range mockModels {
		models = append(models, model)
	}

	return models, nil
}

// createMockModel helper function to create mock model objects
func (s *MockService) createMockModel(
	name string,
	parameters int64,
	maxTokens int,
	loaded bool,
	modelType string,
) *LLMModel {
	rawData := map[string]interface{}{
		"id":          name,
		"name":        name,
		"description": fmt.Sprintf("Mock model with %d parameters", parameters),
		"created":     time.Now().UnixMilli(),
	}

	model := NewLLMModel(name, s, rawData)
	model.UpstreamModelName = name
	model.Parameters = parameters
	model.MaxTokens = maxTokens
	if loaded {
		model.State = ModelStateLoaded
	} else {
		model.State = ModelStateNonLoaded
	}
	model.Type = modelType
	model.Publisher = "MockLLM"
	model.Description = fmt.Sprintf("Mock model with %d parameters", parameters)
	model.Streaming = true
	model.ModifiedAt = time.Now().UnixMilli()
	model.Service = s.GetName()
	model.ServicePtr = s

	// Add specific capabilities based on model size
	if parameters >= 7000000000 {
		model.Coding = true
	}
	if parameters >= 30000000000 {
		model.Tooling = true
	}

	return model
}

// ChatCompletionsRaw overrides the base method to return mock completions
func (s *MockService) ChatCompletionsRaw(
	ctx context.Context,
	model *LLMModel,
	req openapi_client.OpenAICompletionRequestRaw,
) (*api_client.HTTPResponse, error) {
	modelName := model.UpstreamModelName
	s.logger.Debug("Generating mock completion for model: %s", modelName)

	// Create a mock response
	mockResponse := openapi_client.OpenAICompletionResponseRaw{
		"model": modelName,
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": fmt.Sprintf("This is a mock response from %s model", modelName),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}

	// Convert to JSON
	responseBytes, err := json.Marshal(mockResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mock response: %w", err)
	}

	// Create HTTP response
	httpResp := &api_client.HTTPResponse{
		BodyBytes: responseBytes,
		UpstreamResponse: &http.Response{
			StatusCode: http.StatusOK,
		},
	}

	// Simulate a delay in the response
	time.Sleep(200 * time.Millisecond)

	return httpResp, nil
}

// GetModel overrides the base method to return a specific mock model
func (s *MockService) GetModel(ctx context.Context, name string) (*LLMModel, error) {
	if mockModels == nil {
		// Make initial load
		_, err := s.ListModels(ctx, nil)
		if err != nil {
			return nil, err
		}
	}

	s.logger.Debug("Getting mock model: %s", name)

	model, ok := mockModels[name]
	if !ok {
		return nil, fmt.Errorf("model '%s' not found", name)
	}

	return model, nil
}

// ChatCompletions implements the LLMService interface method
func (s *MockService) ChatCompletions(
	ctx context.Context,
	model *LLMModel,
	req *openapi_client.OpenAICompletionRequest,
) (*openapi_client.OpenAICompletionResponse, error) {
	s.logger.Debug("Mock ChatCompletions for model: %s", req.ModelName)

	// Call the Raw version and then parse the response
	resp, err := s.ChatCompletionsRaw(ctx, model, req.ToMap())
	if err != nil {
		return nil, err
	}

	var respObj openapi_client.OpenAICompletionResponse
	respObj.DurationSec = float32(resp.Duration.Seconds())

	if err := json.Unmarshal(resp.BodyBytes, &respObj); err != nil {
		return nil, fmt.Errorf("error decoding mock response: %v %s", err, resp.BodyBytes)
	}

	respObj.HttpResp = resp

	return &respObj, nil
}

// ChatCompletionsStreamRaw implements the LLMService interface method for streaming
func (s *MockService) ChatCompletionsStreamRaw(
	ctx context.Context,
	model *LLMModel,
	req openapi_client.OpenAICompletionRequestRaw,
) (*http.Response, error) {
	modelName, _ := req["model"].(string)
	s.logger.Debug("Generating mock streaming completion for model: %s", modelName)

	// Create a mock pipe to simulate a streaming response
	pr, pw := io.Pipe()

	// Simulate streaming response in a separate goroutine
	go func() {
		defer pw.Close()

		// Write a few streaming messages with delays to simulate real streaming
		messages := []string{
			"This ", "is ", "a ", "mock ", "streaming ", "response ",
			"from ", modelName, " ", "model", "."}

		for _, part := range messages {
			// Create a mock delta message
			deltaEvent := map[string]interface{}{
				"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
				"object":  "chat.completion.chunk",
				"created": time.Now().UnixMilli(),
				"model":   modelName,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"content": part,
						},
						"finish_reason": nil,
					},
				},
			}

			data, _ := json.Marshal(deltaEvent)
			if _, err := pw.Write([]byte("data: ")); err != nil {
				s.logger.Error("Failed to write data prefix: %v", err)
				return
			}
			if _, err := pw.Write(data); err != nil {
				s.logger.Error("Failed to write data: %v", err)
				return
			}
			if _, err := pw.Write([]byte("\n\n")); err != nil {
				s.logger.Error("Failed to write data suffix: %v", err)
				return
			}

			// Add a small delay between chunks to simulate real streaming
			time.Sleep(50 * time.Millisecond)
		}

		// Final "done" message
		if _, err := pw.Write([]byte("data: [DONE]\n\n")); err != nil {
			s.logger.Error("Failed to write done message: %v", err)
		}
	}()

	// Create a mock HTTP response
	mockResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     make(http.Header),
	}
	mockResp.Header.Set("Content-Type", "text/event-stream")

	return mockResp, nil
}

// Mock Embedding API
func (s *MockService) GetEmbeddings(
	ctx context.Context,
	model *LLMModel,
	req *openapi_client.OpenAIEmbeddingRequest,
) (*openapi_client.OpenAIEmbeddingResponse, error) {
	if model == nil {
		return nil, fmt.Errorf("model is nil")
	}

	if model.Type != ModelTypeEmbeddings {
		return nil, fmt.Errorf("model '%s' is not an embedding model", model.Name)
	}

	if model.State != ModelStateLoaded {
		return nil, fmt.Errorf("model '%s' is not loaded", model.Name)
	}

	if model.Name == "" {
		return nil, fmt.Errorf("model name is empty")
	}

	data := []openapi_client.OpenAIEmbeddingResponseData{}
	var inputs []string
	var ok bool

	inputs, ok = req.Input.([]string)
	if !ok {
		inputs = append(inputs, req.Input.(string))
	}

	dimensions := req.Dimensions
	if dimensions == 0 {
		dimensions = 768
	}

	resp := openapi_client.OpenAIEmbeddingResponse{
		Model: model.Name,
		Usage: openapi_client.OpenAIEmbeddingUsage{
			PromptTokens: 10 * len(inputs),
			TotalTokens:  10 * len(inputs),
		},
	}

	for n := range len(inputs) {
		embedding := []float64{}
		for d := 0; d < dimensions; d++ {
			embedding = append(embedding, float64(rand.Float64()))
		}
		data = append(data, openapi_client.OpenAIEmbeddingResponseData{
			Object:    "embedding",
			Index:     n,
			Embedding: embedding,
		})
	}

	resp.Data = data

	return &resp, nil
}

// RequestAndParse should never be reached in mock service
func (s *MockService) RequestAndParse(ctx context.Context, method, path string, body []byte) (map[string]interface{}, error) {
	s.logger.Error("RequestAndParse called on mock service - this should never happen")
	return nil, fmt.Errorf("RequestAndParse should never be called on mock service")
}

// Ping overrides the base method to always return success for mock service
func (s *MockService) Ping(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Debug("Pinging mock service")
	s.PingCount++
	if s.PingShouldFail {
		return fmt.Errorf("mock service ping failed as requested")
	}
	s.setLikelyIsAlive()
	return nil
}

func (s *MockService) ImportModel(ctx context.Context, name string, progress chan<- float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("Importing mock model: %s", name)

	mockModels[name] = s.createMockModel(name, 0, 0, false, ModelTypeLLM)

	return nil
}

func (s *MockService) InstallModel(ctx context.Context, name string, progress chan<- float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("Installing mock model: %s", name)

	if _, ok := mockModels[name]; ok {
		s.logger.Debug("model '%s' already exists", name)
		return nil
	}

	fakeSteps := 10

	for i := 1; i <= fakeSteps; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			jobProgress := float32(i) * 100 / float32(fakeSteps)
			progress <- jobProgress
		}
	}

	mockModels[name] = s.createMockModel(name, 0, 0, false, ModelTypeLLM)

	return nil
}

func (s *MockService) UpdateModel(ctx context.Context, name string, progress chan<- float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("Updating mock model: %s", name)

	return nil
}

// LoadModel overrides the base method to always return success for mock service
func (s *MockService) LoadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	s.logger.Debug("Loading mock model: %s", model.Name)

	model.State = ModelStateLoaded

	return nil
}

// UnloadModel overrides the base method to always return success for mock service
func (s *MockService) UnloadModel(ctx context.Context, model *LLMModel, progress chan<- float32) error {
	s.logger.Debug("Unloading mock model: %s", model.Name)

	model.State = ModelStateNonLoaded

	return nil
}

func (s *MockService) DeleteModel(ctx context.Context, modelName string, progress chan<- float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Debug("Deleting mock model: %s", modelName)

	if _, ok := mockModels[modelName]; !ok {
		s.logger.Debug("model '%s' not found", modelName)
		return nil
	}

	delete(mockModels, modelName)

	return nil
}

func (s *MockService) GetCapabilities() *LLMServiceCapabilities {
	return &LLMServiceCapabilities{
		InstallModel:               true,
		ImportModel:                true,
		UpdateModel:                true,
		LoadModel:                  true,
		LoadModelProgress:          false,
		LoadModelCancel:            false,
		LoadModelTimeout:           false,
		UnloadModel:                true,
		DeleteModel:                true,
		HuggingfaceModelsSupported: true,
		OllamaModelsSupported:      true,
		Streaming:                  true,
		Embedding:                  true,
	}
}

var registeredMockService = false

func RegisterMockService(ctx context.Context) {
	if registeredMockService {
		return
	}

	registeredMockService = true

	config.RegisterLLMServiceConfig(string(MockServiceName), config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "MOCK_API_KEY",
		URLs:         []string{"http://localhost:8000"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 3,
			LongTimeoutSec:  10,
			VerifyCert:      false,
			EnableDiscovery: true,
		},
	})

	cfg := config.Get()

	cfg.LLM.LLMServices[string(MockServiceName)] = config.ConfigLLMService{
		APIFormat:    "openai",
		APIKeyEnvVar: "MOCK_API_KEY",
		URLs:         []string{"http://localhost:8000"},
		Upstream: config.ConfigUpstream{
			ShortTimeoutSec: 3,
			LongTimeoutSec:  10,
			VerifyCert:      false,
			EnableDiscovery: true,
		},
	}
}

// RegisterMockServiceForUnitTests registers the mock service with the config, do not use it for production
func RegisterMockServiceForUnitTests(ctx context.Context) error {
	if registeredMockService {
		return nil
	}

	if _, err := config.Load(); err != nil {
		return fmt.Errorf("failed to load config: %s", err.Error())
	}
	cfg := config.Get()
	cfg.LLM.LLMServices = map[string]config.ConfigLLMService{}

	RegisterMockService(ctx)

	if err := CreateServiceInstances(ctx); err != nil {
		return fmt.Errorf("failed to create service instances: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	return nil
}
