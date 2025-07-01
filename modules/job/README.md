# Job Module

The Job module provides a comprehensive job scheduling and execution system for HyperSpot, enabling asynchronous task management with queue-based processing, retry mechanisms, status tracking, and suspend/resume capabilities. It supports multi-tenant job execution across different queue types with configurable timeouts, retries, and logging.

## Overview

The job module is built around a hierarchical structure:

```
Job Groups
├── Job Types (registered per group)
│   ├── Worker Callbacks
│   ├── Timeout & Retry Policies
│   └── Parameter Schemas
└── Job Queues (compute, maintenance, download)
    ├── Job Workers (configurable capacity)
    └── Job Instances
        ├── Status Management (waiting → running → completed/failed)
        ├── Progress Tracking
        ├── Parameter Handling
        └── Result Storage
```

## Domain Model

### Core Entities

**JobGroup**: Logical grouping of related job types
- `Name` (Primary Key): Group identifier
- `Queue`: Target queue name (compute/maintenance/download)
- `Description`: Human-readable description

**JobType**: Template defining job behavior and constraints
- `TypeID` (Primary Key): Unique type identifier
- `Group`: Reference to JobGroup
- `Name`: Display name
- `TimeoutSec`: Default execution timeout
- `MaxRetries`: Default retry limit
- `RetryDelaySec`: Default delay between retries
- Worker callbacks for initialization, execution, suspend/resume
- Parameter schema definition

**Job**: Individual job instance
- `JobID` (UUID): Unique job identifier
- `TenantID` (UUID): Multi-tenant isolation
- `UserID` (UUID): Job owner
- `IdempotencyKey` (UUID): Prevents duplicate creation
- `Type`: Reference to JobType
- Status progression: init → waiting → running → final states
- Progress tracking (0.0 to 1.0)
- Parameter storage (JSON)
- Result storage (JSON)
- Error information
- Timing metadata

**JobQueue**: Execution queue with worker pool
- Queue types: compute (capacity: 1), maintenance (capacity: 5), download (capacity: 10)
- Worker pool management
- Waiting job queue
- Running job tracking
- Job cancellation support

### Job Lifecycle

```
[Created] → [Waiting] → [Running] → [Completed/Failed/Canceled]
     ↓           ↓       ↓     ↓
  [Skipped]     [Suspended]  [Retrying] → [Running] → [Completed/Failed/Canceled]
                     ↓
                 [Resumed] → [Running] → [Completed/Failed/Canceled]
```

## Features

### Core Functionality
- **Multi-tenant job execution** with proper isolation
- **Queue-based job processing** with dedicated worker pools
- **Asynchronous job execution** with progress tracking
- **Idempotency support** to prevent duplicate job creation
- **Status management** with comprehensive state transitions
- **Retry mechanisms** with configurable delays and limits
- **Suspend/resume capabilities** for long-running jobs
- **Timeout handling** with automatic job termination
- **Progress reporting** with real-time updates
- **Result storage** with JSON serialization
- **Comprehensive API** with REST endpoints for all operations

### Job Status Management
- **Initial States**: initializing, waiting, resuming
- **Running States**: running, canceling, suspending, locked
- **Intermediate States**: suspended
- **Final States**: skipped, canceled, failed, timeout, completed

### Queue Types
- **Compute Queue**: Single-threaded execution for CPU-intensive tasks (capacity: 1)
- **Maintenance Queue**: Multi-threaded for system maintenance tasks (capacity: 5)
- **Download Queue**: High-concurrency for download operations (capacity: 10)

### Worker Callbacks
- **WorkerInitCallback**: Called on job start or resume
- **WorkerExecutionCallback**: Main job execution logic
- **WorkerStateUpdateCallback**: Optional status update notifications
- **WorkerSuspendCallback**: Called before job suspension
- **WorkerResumeCallback**: Called after job resume

## Configuration Settings

The job module supports configuration through the `config.yaml` file under the `job_logger` section for logging settings. Job-specific configurations are handled through JobType registration and individual job parameters.

### Job Type Configuration
Job types are registered programmatically with the following parameters:
- **Group**: Associated JobGroup
- **Name**: Display name
- **Description**: Job type description
- **Timeout**: Default execution timeout
- **RetryDelay**: Default delay between retries
- **MaxRetries**: Default maximum retry attempts
- **WorkerIsSuspendable**: Whether jobs can be suspended/resumed
- **Params**: Parameter schema for validation

### Job Instance Configuration
Individual jobs can override defaults:
- **TimeoutSec**: Custom timeout for this job
- **MaxRetries**: Custom retry limit for this job
- **RetryDelaySec**: Custom retry delay for this job

## Logging Configuration

```yaml
job_logger:
  console_level: "info"     # Console log level
  file_level: "debug"       # File log level
  file: "logs/job.log"      # Log file path
  max_size_mb: 1000         # Max log file size before rotation
  max_backups: 3            # Number of backup log files to keep
  max_age_days: 28          # Max age of log files before deletion
```

## API Endpoints

### Job Management

#### Schedule Job
```bash
curl -X POST /jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "job_type_id",
    "idempotency_key": "uuid",
    "params": {}
  }'
```

#### List Jobs
```bash
curl -X GET "/jobs?page=1&limit=10&status=running,completed&order=-scheduled_at"
```

#### Get Job
```bash
curl -X GET /jobs/{job_id}
```

#### Cancel Job
```bash
curl -X POST /jobs/{job_id}/cancel
```

#### Suspend Job
```bash
curl -X POST /jobs/{job_id}/suspend
```

#### Resume Job
```bash
curl -X POST /jobs/{job_id}/resume
```

#### Delete Job
```bash
curl -X DELETE /jobs/{job_id}
```

### Job Types

#### List Job Types
```bash
curl -X GET "/job-types?page=1&limit=10"
```

#### Get Job Type
```bash
curl -X GET /job-types/{job_type_id}
```

### Job Groups

#### List Job Groups
```bash
curl -X GET "/job-groups?page=1&limit=10"
```

#### Get Job Group
```bash
curl -X GET /job-groups/{job_group_id}
```

## Database Schemas

### Job Table
```sql
jobs:
  job_id           UUID (indexed, primary key)
  tenant_id        UUID (indexed)
  user_id          UUID (indexed)
  idempotency_key  UUID (indexed)
  type             VARCHAR (indexed)
  scheduled_at_ms  BIGINT (indexed)
  updated_at_ms    BIGINT (indexed)
  started_at_ms    BIGINT (indexed)
  eta_ms           BIGINT (indexed)
  locked_by        UUID (indexed)
  progress         FLOAT
  progress_details TEXT
  status           VARCHAR (indexed)
  details          TEXT
  timeout_sec      INTEGER
  max_retries      INTEGER
  retry_delay_sec  INTEGER
  retries          INTEGER
  error            TEXT
  result           TEXT
  params           TEXT (JSON)
```

### JobType Table
```sql
job_types:
  type_id          VARCHAR (primary key)
  description      TEXT
  group_name       VARCHAR (foreign key)
  name             VARCHAR
  timeout_sec      INTEGER
  max_retries      INTEGER
  retry_delay_sec  INTEGER
```

### JobGroup Table
```sql
job_groups:
  name            VARCHAR (primary key)
  queue_name      VARCHAR
  description     TEXT
```

## Dependencies

The job module serves as a foundation for other modules that implement specific job types:
- **benchmark**: Performance benchmarking jobs
- **benchmark_hw**: Hardware benchmark jobs
- **benchmark_llm**: LLM benchmarking jobs
- **dataset**: Data processing jobs
- **models_registry**: Model management jobs
- **test_module**: Testing and validation jobs
