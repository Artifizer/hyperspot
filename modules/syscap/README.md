# SysCap Module Overview

The SysCap (System Capabilities) module provides comprehensive system capability detection and caching across hardware, software, operating systems, LLM services, and HyperSpot modules. It offers a unified interface for discovering and monitoring system resources with intelligent caching strategies and human-friendly presentation. SysCap module provides both machine-friendly APIs and human-readable information about what capabilities are available on the current system, making it useful for dependency tracking and system monitoring.

```
SysCap Module Architecture
├── Capability Detection Engine
│   ├── Hardware Detection (GPU, CPU, RAM, Battery)
│   ├── OS Detection (macOS, Windows, Linux variants)
│   ├── Software Detection (Docker, containers, services)
│   ├── LLM Service Detection (Ollama, LM Studio, GPT4All, etc.)
│   └── Module Capability Detection
├── Intelligent Caching System
│   ├── Per-category Cache TTL (hardware: 10min, software: 10sec, etc.)
│   ├── Auto-refresh on Cache Expiration
│   └── Manual Cache Invalidation
├── REST API Layer
│   ├── List All Capabilities (/syscaps)
│   ├── Category-filtered Listing (/syscaps/{category})
│   ├── Individual Capability Lookup (/syscaps/{category}/{name})
│   └── Cache Management (/syscaps/refresh)
└── CLI Integration
    ├── Human-friendly Output Formatting
    ├── JSON Output Support
    └── Interactive Capability Browsing
```

## Domain Model

### Core Entities

**SysCap**: Represents a single system capability
- `Key`: Unique identifier in "category:name" format (e.g., "hardware:gpu")
- `Category`: Capability category (hardware, software, os, llm_service, module)
- `Name`: Machine-friendly name (lowercase, underscores)
- `DisplayName`: Human-friendly name (proper capitalization, spaces)
- `Present`: Boolean indicating if capability is available
- `Version`: Optional version string for the capability
- `Amount`: Optional numeric value (RAM size, GPU count, etc.)
- `AmountDimension`: Unit for amount (GB, count, percent, cores, etc.)
- `Details`: Optional detailed information string
- `CacheTTLMsec`: Cache timeout in milliseconds
- `CachedAtMs`: Timestamp when capability was last detected

**SysCapCategory**: Capability classification
- `hardware`: Physical system components (GPU, CPU, RAM, Battery)
- `software`: Installed software and services (Docker, containers)
- `os`: Operating system information (macOS, Windows, Linux)
- `llm_service`: AI/LLM service availability (Ollama, LM Studio, etc.)
- `module`: Application module capabilities

**SysCapDetector**: Function interface for capability detection
- Stateless detection functions that populate SysCap instances
- Called on cache miss or manual refresh
- Return current capability state and metadata


##  Core Functionality
- **Multi-category detection** across 5 capability categories
- **Intelligent caching** with per-category TTL optimization
- **Real-time detection** with automatic cache refresh
- **Human-friendly presentation** with display names and detailed descriptions
- **Machine-readable APIs** with consistent key formats
- **Comprehensive metadata** including versions, amounts, and detailed information

## Configuration Settings

The SysCap module uses detector-specific configuration embedded in the detection functions. No external configuration files are supported at the moment.

## API Endpoints

### GET /syscaps
List all system capabilities across all categories.

```bash
curl -X GET "http://localhost:8087/syscaps"
```

**Response**:
```json
[
  {
    "key": "hardware:gpu",
    "category": "hardware",
    "name": "gpu",
    "display_name": "GPU",
    "present": true,
    "amount": 2,
    "amount_dimension": "count",
    "details": "NVIDIA RTX 4090 (24 GB), AMD RX 7900 XTX (24 GB)",
    "version": null
  },
  {
    "key": "os:macos",
    "category": "os",
    "name": "macos",
    "display_name": "macOS",
    "present": true,
    "version": "14.5",
    "details": "Platform: macOS, Version: 14.5, Architecture: arm64 [Apple Silicon]"
  }
]
```

### GET /syscaps/{category}
List capabilities filtered by category.

```bash
curl -X GET "http://localhost:8087/syscaps/hardware"
```

### GET /syscaps/{category}/{name}
Get a specific capability by category and name.

```bash
curl -X GET "http://localhost:8087/syscaps/hardware/gpu"
```

### GET /syscaps/{key}
Get a capability by its key (category:name format).

```bash
curl -X GET "http://localhost:8087/syscaps/hardware:gpu"
```

### POST /syscaps/refresh
Refresh all capabilities (clear cache and re-detect).

```bash
curl -X POST "http://localhost:8087/syscaps/refresh"
```

### POST /syscaps/{category}/refresh
Refresh capabilities for a specific category.

```bash
curl -X POST "http://localhost:8087/syscaps/hardware/refresh"
```

## Developers Guide

### Using SysCap in Other Modules

```go
import "github.com/hypernetix/hyperspot/modules/syscap"

// Check if a capability is present
gpu, err := syscap.GetSysCap(syscap.NewSysCapKey(syscap.CategoryHardware, syscap.SysCapHwNameGPU))
if err != nil {
    return fmt.Errorf("failed to get GPU capability: %v", err)
}

if gpu.Present {
    fmt.Printf("GPU available: %s\n", *gpu.Details)
    if gpu.Amount != nil {
        fmt.Printf("GPU count: %.0f\n", *gpu.Amount)
    }
}

// List all hardware capabilities
hardwareCategory := syscap.CategoryHardware
capabilities, err := syscap.ListSysCaps(hardwareCategory)
if err != nil {
    return fmt.Errorf("failed to list hardware capabilities: %v", err)
}

for _, cap := range capabilities {
    fmt.Printf("%s: %t\n", cap.DisplayName, cap.Present)
}

// Register a custom capability detector
syscap.RegisterSysCap(
    syscap.CategoryModule,
    "my_capability",
    "My Capability",
    true,  // present
    10000, // cache TTL in ms
    func(cap *syscap.SysCap) error {
        // Custom detection logic
        cap.SetPresent(checkMyFeature()) // set true/false if capability enabled/disabled
        cap.SetDetails("Custom feature details")
        return nil
    },
)
```

### Adding New Capability Detectors

```go
// Create a new detector function
func detectMyCapability(cap *syscap.SysCap) error {
    // Perform detection logic
    available := checkSomething()

    cap.SetPresent(available)  // set true/false if capability enabled/disabled
    if available {
        cap.SetVersion("1.0.0")
        cap.SetAmount(42.0, syscap.DimensionCount)
        cap.SetDetails("My capability is working perfectly")
    }
    return nil
}

// Register the capability detector during module initialization
func init() {
    syscap.RegisterSysCap(
        syscap.CategorySoftware,
        "my_service",
        "My Service",
        false, // will be set by detector
        10000, // 10 second cache
        detectMyCapability,
    )
}
```

## Database Schemas

The SysCap module does not use persistent database storage. All capabilities are detected dynamically and cached in memory with configurable TTL periods.
