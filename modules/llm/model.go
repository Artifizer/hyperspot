package llm

import (
	"strconv"
	"strings"
)

type ModelState string

const (
	ModelStateUnknown     ModelState = "unknown"
	ModelStateLoaded      ModelState = "loaded"
	ModelStateNonLoaded   ModelState = "not loaded"
	ModelStateDownloading ModelState = "downloading"
	ModelStateError       ModelState = "error"
	ModelStateRemoving    ModelState = "removing"
	ModelStateRemoved     ModelState = "removed"
	ModelStateAny         ModelState = "?"
)

const (
	ModelTypeLLM        = "llm"
	ModelTypeVLM        = "vlm"
	ModelTypeEmbeddings = "embeddings"
	ModelTypeVision     = "vision"
	ModelTypeUnknown    = "unknown"
)

const ServiceNameSeparator = "~"

// ModelInfo represents information about an LLM model
type LLMModel struct {
	Type               string                 `json:"type" gorm:"index"`
	State              ModelState             `json:"state"`
	Description        string                 `json:"description"`
	Publisher          string                 `json:"publisher"`
	ModifiedAt         int64                  `json:"modified_at"`
	Architecture       string                 `json:"arch"`
	Quantization       string                 `json:"quantization"`
	Name               string                 `json:"name" gorm:"index"`
	UpstreamModelName  string                 `json:"-"`
	MaxTokens          int                    `json:"max_tokens" doc:"max number of context tokens supported by the model"`
	Size               int64                  `json:"size" gorm:"index" doc:"model size in bytes"`
	Streaming          bool                   `json:"streaming" doc:"openai /v1/completions API streaming is supported"`
	Instructed         bool                   `json:"instructed" doc:"model can be used for instruction-following"`
	Coding             bool                   `json:"coding" doc:"model is optimized for coding tasks"`
	Tooling            bool                   `json:"tooling" doc:"model supports tooling"`
	IsMLX              bool                   `json:"is_mlx" gorm:"index" doc:"model has Apple MLX format"`
	IsGGUF             bool                   `json:"is_gguf" gorm:"index" doc:"model has GGUF format"`
	Parameters         int64                  `json:"parameters"`
	RuntimeName        string                 `json:"runtime_name"`
	Service            string                 `json:"service"`
	FilePath           string                 `json:"file_path"`
	RuntimePtr         LLMRuntime             `json:"-" gorm:"-"`
	ServicePtr         LLMService             `json:"-" gorm:"-"`
	rawOpenAPIResponse map[string]interface{} `json:"-" gorm:"-"`
}

func NewLLMModel(name string, service LLMService, rawResponse map[string]interface{}) *LLMModel {
	if service == nil {
		return nil
	}
	serviceName := service.GetName()
	m := &LLMModel{
		Name:               serviceName + ServiceNameSeparator + name,
		UpstreamModelName:  name,
		Service:            serviceName,
		ServicePtr:         service,
		rawOpenAPIResponse: rawResponse,
		Streaming:          service.GetCapabilities().Streaming,
	}
	m.UpdateDetailsFromName()
	return m
}

func ServiceNameFromModelName(modelName string) string {
	return strings.Split(modelName, ServiceNameSeparator)[0]
}

func (m *LLMModel) UpdateDetailsFromName() {
	// Split the model name by hyphens
	parts := strings.Split(strings.ToLower(m.Name), "-")

	// Initialize heuristics
	for _, part := range parts {
		switch {
		case strings.Contains(part, "instruct"):
			m.Instructed = true
		case strings.Contains(part, "tooling"):
			m.Tooling = true
		case strings.Contains(part, "mlx"):
			m.IsMLX = true
		case strings.Contains(part, "gguf"):
			m.IsGGUF = true
		case strings.HasSuffix(part, "b"):
			// Extract size in billions
			size, err := strconv.Atoi(strings.TrimSuffix(part, "b"))
			if err == nil {
				m.Parameters = int64(size) * 1000000000
			}
		case strings.Contains(part, "hf"):
			m.Quantization = "hf"
		case strings.Contains(part, "arch"):
			m.Architecture = part
		case strings.Contains(part, "code"):
			m.Coding = true
		}
	}

	// Check for known architectures in the model name
	knownArchitectures := []string{"qwen", "llama2", "llama3", "deepseek", "falcon", "gpt", "anthropic", "claude", "o1", "gemma", "gemini"}
	for _, arch := range knownArchitectures {
		if strings.Contains(m.Name, arch) {
			m.Architecture = arch
			break
		}
	}

	// Set default values if not already set
	if m.Architecture == "" {
		m.Architecture = "unknown"
	}
	if m.Quantization == "" {
		m.Quantization = "none"
	}

	// Check if model name contains indicators of specific types
	modelNameLower := strings.ToLower(m.UpstreamModelName)

	// Check for embedding models
	if strings.Contains(modelNameLower, "embed") ||
		strings.Contains(modelNameLower, "embedding") {
		m.Type = ModelTypeEmbeddings
	} else if strings.Contains(modelNameLower, "vision") ||
		strings.Contains(modelNameLower, "visual") ||
		strings.Contains(modelNameLower, "clip") {
		m.Type = ModelTypeVision
	} else if strings.Contains(modelNameLower, "bge-") ||
		strings.Contains(modelNameLower, "all-minilm") ||
		strings.Contains(modelNameLower, "multilingual-e5") ||
		strings.Contains(modelNameLower, "paraphrase-multilingual") {
		m.Type = ModelTypeEmbeddings
	}
}

func (m *LLMModel) GetRawOpenAPIResponse() map[string]interface{} {
	return m.rawOpenAPIResponse
}

// ListModelsResponse represents the response for listing models
type LLMModelsList struct {
	Models []LLMModel `json:"models"`
}
