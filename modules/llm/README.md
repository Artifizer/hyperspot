# LLM Module

The LLM module provides a unified interface for managing and interacting with Large Language Models across multiple AI service providers. It supports dynamic service discovery, model lifecycle management, OpenAI-compatible API endpoints, and comprehensive model operations through an extensible plugin architecture.

## Overview

The LLM module acts as a central hub for language model operations, abstracting away the complexities of different AI service providers behind a unified interface. It supports local services (Ollama, LM Studio, Jan, GPT4All, LocalAI, CortexSo) and cloud services (e.g. OpenAI, Anthropic) with automatic service discovery, model management, and request proxying.

```
LLM Module Architecture
├── Service Discovery & Registry
│   ├── Multi-provider Support (Ollama, OpenAI, Anthropic, etc.)
│   ├── Health Monitoring & Availability Tracking
│   └── Dynamic Service Registration
├── Model Management System
│   ├── Model Installation & Import
│   ├── Model Loading & Unloading
│   ├── Model Lifecycle Operations
│   └── Model State Tracking
├── OpenAI-Compatible API Layer
│   ├── Chat Completions (/v1/chat/completions)
│   ├── Embeddings (/v1/embeddings)
│   └── Streaming Response Support
└── Job System Integration
    ├── Asynchronous Model Operations
    ├── Progress Tracking & Reporting (e.g. for model loading)
    └── Error Handling & Retry Logic
```

## Domain Model

### Core Interfaces

**LLMService**: Unified interface for all AI service providers
- Service identification and capability reporting
- Model discovery and management operations
- Chat completion and embedding generation
- Health monitoring and availability status

**LLMRuntime**: Runtime environment abstraction
- Execution environment identification (llama.cpp, MLX, ONNX, etc.)
- Runtime-specific optimization support

### Core Entities

**LLMModel**: Model representation and metadata
- `Name`: Full qualified model name with service prefix
- `UpstreamModelName`: Original model name from the service
- `Type`: Model type (llm, vlm, embeddings, vision, unknown)
- `State`: Current model state (loaded, not loaded, downloading, error, etc.)
- `Architecture`: Model architecture (qwen, llama2, llama3, gpt, etc.)
- `Quantization`: Quantization method applied to the model
- `MaxTokens`: Maximum context length supported
- `Size`: Model size in bytes
- Model capabilities: `Streaming`, `Instructed`, `Coding`, `Tooling`
- Format flags: `IsMLX`, `IsGGUF`
- Service integration: `ServicePtr`, `RuntimePtr`

**LLMServiceCapabilities**: Service feature matrix
- Model management: `InstallModel`, `ImportModel`, `UpdateModel`, `LoadModel`, `UnloadModel`, `DeleteModel`
- Operation tracking: `LoadModelProgress`, `LoadModelCancel`, `LoadModelTimeout`
- Model ecosystem: `HuggingfaceModelsSupported`, `OllamaModelsSupported`
- API features: `Inference`, `Streaming`, `Embedding`

**Service Configurations**:
- `ConfigLLMService`: Service-specific settings (API format, keys, URLs, timeouts)
- `ConfigUpstream`: Upstream connection parameters (timeouts, certificate verification, discovery)

## Features

### Core Functionality
- **Multi-provider support** with unified interface across 9+ AI service types
- **Automatic service discovery** with health monitoring and availability tracking
- **Model lifecycle management** including installation, loading, unloading, and deletion
- **OpenAI-compatible API** for seamless integration with existing tools
- **Streaming response support** for real-time chat completions
- **Embedding generation** for semantic search and RAG applications
- **Job system integration** for asynchronous model operations
- **Progress tracking** for long-running model operations
- **Error handling and retry logic** with comprehensive error reporting

### Service Provider Support
- **Local Services**: Ollama, LM Studio, Jan, GPT4All, LocalAI, CortexSo
- **Cloud Services**: OpenAI, Anthropic
- **Mock Service**: Testing and development support
- **External Service Authentication**: API key management via config params or environment variables
- **Service Health Monitoring**: Automatic ping and availability detection

### API Integration Features
- **OpenAI-compatible endpoints** for chat completions and embeddings
- **Request proxying** to appropriate backend services
- **Model name resolution** with fallback to first loaded model
- **Response transformation** with unified model naming
- **Streaming support** with chunked transfer encoding
- **Content-type handling** for different response formats

### Model Management Features
- Model installation from remote repositories (Hugging Face, Ollama Hub)
- Model import from local files
- Model loading into memory with progress tracking
- Model unloading
- Model updates from upstream sources
- Model permanent deletion
- Model capability detection based on name and metadata

## Configuration Settings

The LLM module is configured through the `llm` section in `config.yaml`:

### Global LLM Configuration
```yaml
llm:
  default_service: "ollama"
  temperature: 0.7
  max_tokens: 4096
  services:
    # Service configurations
```

### Service Configuration Structure
```yaml
llm:
  services:
    ollama:
      api_format: "openai"
      urls: ["http://localhost:11434"]
      upstream:
        short_timeout_sec: 5
        long_timeout_sec: 180
        verify_cert: true
        enable_discovery: true

    openai:
      api_format: "openai"
      api_key_env_var: "OPENAI_API_KEY"
      urls: ["https://api.openai.com"]
      upstream:
        short_timeout_sec: 10
        long_timeout_sec: 300
        verify_cert: true
        enable_discovery: false
```

### Configuration Parameters
- **default_service**: Default LLM service name for model operations
- **temperature**: Default temperature for completions
- **max_tokens**: Default maximum tokens for completions
- **api_format**: API format specification (currently "openai")
- **api_key_env_var**: Environment variable containing API key for external services
- **api_key**: Direct API key specification (not recommended for production)
- **urls**: List of service endpoint URLs for load balancing
- **short_timeout_sec**: Timeout for health checks and service discovery
- **long_timeout_sec**: Timeout for completion and embedding requests
- **verify_cert**: TLS certificate verification flag
- **enable_discovery**: Enable automatic model discovery from service

## Logging Configuration

The LLM module uses the main logger system. Logging configuration is inherited from the global logging settings in `config.yaml`. LLM-specific logging can be configured through the main logging section.

```yaml
logging:
  main:
    console_level: "info"
    file_level: "debug"
    file: "logs/main.log"
    max_size_mb: 1000
    max_backups: 3
    max_age_days: 28
```

### Logging Behavior
- **Service Discovery**: Logs service detection, availability, and configuration issues
- **Model Operations**: Tracks model loading, unloading, and lifecycle events
- **API Requests**: Debug-level logging for completion and embedding requests
- **Error Handling**: Comprehensive error logging with service context
- **Performance Metrics**: Request duration and response size tracking

## API Endpoints

### LLM Service Management
```bash
# List all available LLM services
curl -X GET /llm/services

# List models for a specific service
curl -X GET /llm/services/{service_name}/models

# Get model details from a service
curl -X GET /llm/services/{service_name}/models/{model_name}

# Check service availability
curl -X GET /llm/services/{service_name}/ping

# Get service capabilities
curl -X GET /llm/services/{service_name}/capabilities
```

### Model Discovery and Information
```bash
# List all models across all services
curl -X GET /llm/models

# Get details about a specific model
curl -X GET /llm/models/{model_name}
```

### OpenAI-Compatible API
```bash
# Create chat completion
curl -X POST /v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ollama~llama3.2:3b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": false
  }'

# Create embedding
curl -X POST /v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ollama~all-minilm",
    "input": "The quick brown fox"
  }'
```

### Model Operations (via Job API)
```bash
# Install model from remote repository
curl -X POST /jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "install",
    "params": {
      "service_name": "ollama",
      "model_name": "llama3.2:3b"
    }
  }'

# Load model into memory
curl -X POST /jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "load",
    "params": {
      "model_name": "ollama~llama3.2:3b"
    }
  }'
```

## Database Schemas

### LLMModel Table
```sql
llm_models:
  type                VARCHAR (model type: llm/vlm/embeddings/vision/unknown)
  state               VARCHAR (model state: loaded/not loaded/downloading/error/etc.)
  description         TEXT (model description)
  publisher           VARCHAR (model publisher/organization)
  modified_at         BIGINT (last modification timestamp)
  architecture        VARCHAR (model architecture: qwen/llama2/llama3/gpt/etc.)
  quantization        VARCHAR (quantization method applied)
  name                VARCHAR (full qualified model name with service prefix)
  max_tokens          INTEGER (maximum context length)
  size                BIGINT (model size in bytes)
  streaming           BOOLEAN (streaming API support)
  instructed          BOOLEAN (instruction-following capability)
  coding              BOOLEAN (coding task optimization)
  tooling             BOOLEAN (tool/function calling support)
  is_mlx              BOOLEAN (Apple MLX format)
  is_gguf             BOOLEAN (GGUF format)
  parameters          BIGINT (model parameter count)
  runtime_name        VARCHAR (runtime environment name)
  service             VARCHAR (service name)
  file_path           VARCHAR (local file path if applicable)
```

### LLMRegistryModel Table (Models Registry Integration)
```sql
llm_registry_models:
  -- Inherits all LLMModel fields
  db_key              VARCHAR (unique database key)
  registry            VARCHAR (registry source: huggingface/ollama/cortex)
  id                  VARCHAR (registry model ID)
  model_id            VARCHAR (model identifier in registry)
  likes               INTEGER (community likes/stars)
  trending_score      INTEGER (trending popularity score)
  downloads           INTEGER (download count)
  created_at_ms       BIGINT (model creation timestamp)
  tags                TEXT (comma-separated model tags)
  url                 VARCHAR (model URL in registry)
```

### Service Configuration Tables
```sql
llm_service_configs:
  service_name        VARCHAR (service type name)
  api_format          VARCHAR (API format specification)
  api_key_env_var     VARCHAR (environment variable for API key)
  urls                JSON (array of service URLs)
  short_timeout_sec   INTEGER (short operation timeout)
  long_timeout_sec    INTEGER (long operation timeout)
  verify_cert         BOOLEAN (TLS certificate verification)
  enable_discovery    BOOLEAN (automatic model discovery)
```

## Dependencies

### Module Dependencies
- **job**: Asynchronous model operations including installation, loading, unloading, and management
- **models_registry**: Model discovery and metadata from external registries (Hugging Face, Ollama Hub)

### Core Library Dependencies
- **api**: HTTP API framework and response handling
- **config**: Configuration management and service parameter loading
- **logging**: Centralized logging and debugging support
- **errorx**: Structured error handling and HTTP status mapping
- **api_client**: HTTP client utilities for upstream service communication
- **openapi_client**: OpenAI-compatible API client implementation
