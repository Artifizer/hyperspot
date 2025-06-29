package openapi_client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/api_client"
)

type OpenAICompletionRequestRaw map[string]interface{}
type OpenAICompletionResponseRaw map[string]interface{}

type OpenAICompletionRequest struct {
	ModelName   string
	Messages    []OpenAICompletionMessage
	Temperature float64
	MaxTokens   int
	Stream      bool
}

// ToMap converts the OpenAICompletionRequest struct to map[string]interface{}
func (r *OpenAICompletionRequest) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"model":       r.ModelName,
		"messages":    r.Messages,
		"temperature": r.Temperature,
		"max_tokens":  r.MaxTokens,
		"stream":      r.Stream,
	}
}

type OpenAICompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAICompletionResponse struct {
	DurationSec float32                  `json:"duration_sec"`
	Error       *OpenAIResponseError     `json:"error"`
	Model       string                   `json:"model"`
	Choices     []OpenAICompletionChoice `json:"choices"`
	Usage       OpenAICompletionUsage    `json:"usage"`
	HttpResp    *api_client.HTTPResponse `json:"-"`
}

type OpenAIResponseError struct {
	Message string `json:"message"`
}

type OpenAICompletionChoice struct {
	Message OpenAICompletionMessage `json:"message"`
}

type OpenAICompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Embeddings
type OpenAIEmbeddingRequestRaw map[string]interface{}

type OpenAIEmbeddingRequest struct {
	ModelName      string `json:"model"`
	Input          any    `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
}

type OpenAIEmbeddingRequestSingleInput struct {
	ModelName      string `json:"model"`
	Input          string `json:"input"`
	EncodingFormat string `json:"encoding_format"`
	Dimensions     int    `json:"dimensions"`
}

type OpenAIEmbeddingRequestMultipleInput struct {
	ModelName      string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
	Dimensions     int      `json:"dimensions"`
}

type OpenAIEmbeddingResponseData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

type OpenAIEmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type OpenAIEmbeddingResponse struct {
	Object      string                        `json:"object"`
	DurationSec float32                       `json:"-"`
	Error       *OpenAIResponseError          `json:"-"`
	Model       string                        `json:"model"`
	Data        []OpenAIEmbeddingResponseData `json:"data"` // Changed to array to match LM Studio's response format
	Usage       OpenAIEmbeddingUsage          `json:"usage"`
	HttpResp    *api_client.HTTPResponse      `json:"-"`
}

// OpenAIAPIClient implements LLMAPIClient for OpenAI's API
type OpenAIAPIClient struct {
	*api_client.BaseAPIClient
	APIKeyEnvVar string
	apiKey       string
}

// NewOpenAIAPIClient creates a new OpenAI API client
func NewOpenAIAPIClient(
	name string,
	baseURL string,
	apiKeyDirect string,
	apiKeyEnvVar string,
	shortTimeoutSec int,
	longTimeoutSec int,
	verifyCert bool,
	enableDiscovery bool,
) *OpenAIAPIClient {
	// First try to get API key from environment variable
	apiKey := os.Getenv(apiKeyEnvVar)

	// If environment variable is not set, use direct API key if provided
	if apiKey == "" && apiKeyDirect != "" {
		apiKey = apiKeyDirect
	}

	// Default fallback value if no API key is available
	if apiKey == "" {
		apiKey = "api-key-is-not-set"
	}

	return &OpenAIAPIClient{
		BaseAPIClient: api_client.NewBaseAPIClient(
			name,
			baseURL,
			shortTimeoutSec,
			longTimeoutSec,
			verifyCert,
			enableDiscovery,
		),
		APIKeyEnvVar: apiKeyEnvVar,
		apiKey:       apiKey,
	}
}

func (c *OpenAIAPIClient) Request(ctx context.Context, method string, path string, body []byte) (*http.Response, error) {
	return c.BaseAPIClient.Request(ctx, method, path, c.apiKey, body, true)
}

func (c *OpenAIAPIClient) RequestWithLongTimeout(ctx context.Context, method string, path string, body []byte) (*http.Response, error) {
	return c.BaseAPIClient.RequestWithLongTimeout(ctx, method, path, c.apiKey, body, true)
}

func (c *OpenAIAPIClient) GetWithAPIKey(ctx context.Context, path string) (*api_client.HTTPResponse, error) {
	return c.BaseAPIClient.Get(ctx, path, c.apiKey)
}

func (c *OpenAIAPIClient) PostWithAPIKey(ctx context.Context, path string, body []byte) (*api_client.HTTPResponse, error) {
	// Return raw response body without parsing
	return c.BaseAPIClient.Post(ctx, path, c.apiKey, body)
}

func (c *OpenAIAPIClient) PostWithAPIKeyAndLongTimeout(ctx context.Context, path string, body []byte) (*api_client.HTTPResponse, error) {
	// Return raw response body without parsing
	return c.BaseAPIClient.PostWithLongTimeout(ctx, path, c.apiKey, body)
}

// ListModels implements LLMAPIClient
func (c *OpenAIAPIClient) ListModels(ctx context.Context) (*api_client.HTTPResponse, error) {
	return c.GetWithAPIKey(ctx, "/v1/models")
}

// GetModel implements LLMAPIClient
func (c *OpenAIAPIClient) GetModel(ctx context.Context, modelID string) (*api_client.HTTPResponse, error) {
	return c.GetWithAPIKey(ctx, fmt.Sprintf("/v1/models/%s", modelID))
}

// CreateChatCompletion implements LLMAPIClient
func (c *OpenAIAPIClient) ChatCompletionsRaw(ctx context.Context, req OpenAICompletionRequestRaw) (*api_client.HTTPResponse, error) {
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return c.PostWithAPIKeyAndLongTimeout(ctx, "/v1/chat/completions", jsonBody)
}

// Get embeddings
func (c *OpenAIAPIClient) GetEmbeddings(ctx context.Context, req OpenAIEmbeddingRequest) (*OpenAIEmbeddingResponse, error) {
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.PostWithAPIKeyAndLongTimeout(ctx, "/v1/embeddings", jsonBody)
	if err != nil {
		return nil, err
	}

	var respObj OpenAIEmbeddingResponse
	respObj.DurationSec = float32(resp.Duration.Seconds())

	if err := json.Unmarshal(resp.BodyBytes, &respObj); err != nil {
		return nil, fmt.Errorf("error decoding response: %v %s", err, resp.BodyBytes)
	}
	respObj.HttpResp = resp

	return &respObj, nil
}

// Ping implements LLMAPIClient
func (c *OpenAIAPIClient) Ping(ctx context.Context) error {
	_, err := c.GetWithAPIKey(ctx, "/v1/models")
	return err
}

// LLMAPIClient defines the interface for LLM API clients
type LLMAPIClient interface {
	// Model Management
	ListModels(ctx context.Context, pageRequest *api.PageAPIRequest) ([]byte, error)
	GetModel(ctx context.Context, modelID string) (*api_client.HTTPResponse, error)

	// Completion APIs
	ChatCompletionsRaw(ctx context.Context, req OpenAICompletionRequestRaw) (*OpenAICompletionResponse, error)
	GetEmbeddings(ctx context.Context, req OpenAIEmbeddingRequest) (*OpenAIEmbeddingResponse, error)

	// Health Check
	LikelyIsAlive() bool
	Ping(ctx context.Context) error

	// Configuration
	GetEndpoint() string
	GetAPIKeyEnvVar() string
	GetName() string
	GetBaseURL() string
}
