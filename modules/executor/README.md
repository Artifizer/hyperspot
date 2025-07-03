# Executor Module

The Executor module provides a unified code execution system for HyperSpot, enabling secure and isolated execution of code across different environments including Docker containers, SSH remote hosts, and local systems. It supports multiple runtime environments with configurable resource limits, timeout controls, and result tracking for benchmark jobs and other automated code execution tasks.

## Overview

The executor module is designed around a factory pattern with pluggable execution backends:

```
Code Executor Registry
├── Docker Executor (isolated container execution)
│   ├── Resource Limits (CPU, memory, PIDs)
│   ├── Image Management (pull/download jobs)
│   └── Container Lifecycle Management
├── SSH Executor (remote system execution)
│   ├── SSH Connection Management
│   ├── Remote Authentication (password/key)
│   └── Remote Runtime Installation
└── Local Executor (localhost execution)
    ├── Working Directory Management
    ├── Temporary File Handling
    └── Local Runtime Detection
```

## Domain Model

### Core Interfaces

**ExecutorType**: Enumeration of supported executor types
- `docker`: Container-based execution
- `ssh`: Remote SSH execution
- `localhost`: Local system execution

### Core Entities

**CodeExecutionResult**: Execution result tracking
- `ID` (UUID): Unique execution identifier
- `ExecutorType`: Type of executor used
- `ExecutorId`: Executor-specific identifier
- `RuntimeId`: Runtime environment identifier
- `Name`: Human-readable executor name
- `ConfigId`: Reference to executor configuration
- Status flags: `Completed`, `Success`
- `ExitCode`: Process exit code
- Output streams: `Stdout`, `Stderr`
- Timing: `StartTimeMs`, `EndTimeMs`, `DurationSec`
- Error information and hostname

**Executor Configurations**:
- `ConfigCodeExecutorDocker`: Docker-specific settings (image, resource limits)
- `ConfigCodeExecutorSSH`: SSH connection parameters (host, credentials)
- `ConfigCodeExecutorLocal`: Local execution settings (executor path, timeout)

## Features

### Core Functionality
- **Multi-environment code execution** with Docker, SSH, and local support
- **Unified execution interface** across all executor types
- **Resource management** with configurable CPU, memory, and process limits (Docker)
- **Timeout controls** with per-executor timeout configuration
- **Runtime installation** with automatic dependency management
- **Result tracking** with comprehensive execution metadata
- **Error handling** with detailed error reporting and exit codes
- **Stream capture** for both stdout and stderr output
- **Security isolation** through containerization and sandboxing

### Docker Executor Features
- **Container isolation** with resource limits (CPU cores, memory MB, PID limits)
- **Image management** with automatic image pulling via job system
- **Resource constraints** configurable per executor instance
- **Automatic cleanup** of containers after execution
- **Progress tracking** for image download operations

### SSH Executor Features
- **Remote execution** on SSH-accessible systems
- **Authentication support** for both password and key-based auth
- **Connection management** with configurable host and port
- **Remote runtime installation** capabilities (planned)

### Local Executor Features
- **Localhost execution** in isolated working directories
- **Temporary file management** with automatic cleanup
- **Working directory isolation** per execution
- **Direct Python interpreter usage**

### Job System Integration
- **Docker image download jobs** for automated image management
- **Progress reporting** during long-running operations
- **Job locking** during dependency installation
- **Comprehensive logging** through job system

## Configuration Settings

The executor module is configured through the `code_executors` section in `config.yaml`:

### Docker Executor Configuration
```yaml
code_executors:
  docker:
    docker_python_default:
      code_executor_path: "/usr/local/bin/python3.11"
      timeout_sec: 30
      image: "python:3.11-alpine"
      mem_limit_mb: 1024
      cpus_limit_cores: 1
      pids_limit: 50
```

### SSH Executor Configuration
```yaml
code_executors:
  ssh:
    ssh_python_default:
      code_executor_path: "/usr/local/bin/python3.11"
      timeout_sec: 30
      host: "localhost"
      port: 22
      username: "user"
      password: "password"
      private_key: "~/.ssh/id_rsa"
```

### Local Executor Configuration
```yaml
code_executors:
  local:
    local_python_default:
      code_executor_path: "/usr/local/bin/python3.11"
      timeout_sec: 30
```

### Configuration Parameters
- **code_executor_path**: Path to Python interpreter
- **timeout_sec**: Execution timeout in seconds
- **image**: Docker image name (Docker only)
- **mem_limit_mb**: Memory limit in megabytes (Docker only)
- **cpus_limit_cores**: CPU core limit (Docker only)
- **pids_limit**: Process limit (Docker only)
- **host/port**: SSH connection details (SSH only)
- **username/password/private_key**: SSH authentication (SSH only)

## Logging Configuration

The executor module uses the main logger system. Logging configuration is inherited from the global logging settings in `config.yaml`. Executor-specific logging can be configured through the main logging section.

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

## API Endpoints

The executor module does not expose direct HTTP API endpoints. Code execution is performed programmatically through the `CodeExecutorIface` interface. However, Docker image download operations are managed through the job system API:

### Docker Image Download (via Job API)
```bash
# Schedule Docker image download job
curl -X POST /jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "docker_image.download",
    "params": {
      "code_executor": "docker_python_default"
    }
  }'
```

### Job Management for Executor Operations
```bash
# Monitor executor job status
curl -X GET /jobs/{job_id}

# Cancel long-running executor job
curl -X POST /jobs/{job_id}/cancel
```

## Database Schemas

### CodeExecutionResult Table
```sql
code_execution_results:
  id                UUID (primary key)
  executor_type     VARCHAR (docker/ssh/localhost)
  executor_id       VARCHAR (executor instance identifier)
  runtime_id        VARCHAR (runtime environment identifier)
  name              VARCHAR (executor name)
  config_id         UUID (configuration reference)
  completed         BOOLEAN (execution completion status)
  success           BOOLEAN (execution success status)
  exit_code         INTEGER (process exit code)
  stdout            TEXT (standard output)
  stderr            TEXT (standard error output)
  hostname          VARCHAR (execution hostname)
  start_time_ms     BIGINT (execution start timestamp)
  end_time_ms       BIGINT (execution end timestamp)
  duration_sec      FLOAT (execution duration)
  error             TEXT (error details)
```

### Executor Configuration Tables
```sql
docker_executors:
  docker_executor_id  UUID (primary key)
  code_executor       UUID (foreign key to CodeExecutionResult)
  image               VARCHAR (Docker image name)

ssh_executors:
  ssh_executor_id     UUID (primary key)
  code_executor       UUID (foreign key to CodeExecutionResult)
  host                VARCHAR (SSH hostname)
  port                INTEGER (SSH port)
  username            VARCHAR (SSH username)

local_executors:
  local_executor_id   UUID (primary key)
  code_executor       UUID (foreign key to CodeExecutionResult)
  executor_path       VARCHAR (local Python path)
  timeout_sec         FLOAT (execution timeout)
  work_dir            VARCHAR (working directory)
```

## Dependencies

### Module Dependencies
- **job**: Job scheduling and execution for Docker image downloads and executor management
- **benchmark_llm**: Primary consumer for code execution in LLM benchmarking scenarios

### External Dependencies
- **Docker Engine**: Required for Docker executor functionality
- **SSH Client**: Required for SSH executor remote connections
- **Python Runtime**: Required on target systems for code execution
- **File System Access**: Required for local executor temporary directory management
