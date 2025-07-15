# Job Module

The Job module provides a comprehensive job scheduling and execution system for **HyperSpot** modules, enabling asynchronous task management with queue-based processing, retry mechanisms, status tracking, and suspend/resume capabilities. It supports parallel job execution across different queue types with configurable parallelism level, timeouts, and retry logic.

The primary actors of the job management system are:

1. **Job API handler** - REST API endpoints for job management such as start, cancel, suspend, resume, list, get, delete
2. **Job Executor** - Global jobs metadata orchestrator that handles jobs scheduling and job states transitions
3. **Job Worker** - Job worker that runs in a separate go routine and handles the specific job execution


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
- Worker callbacks for parameters validation and main worker execution
- Specific job parameters schema definition

**Job**: Individual job instance
- `JobID` (UUID): Unique job identifier
- `TenantID` (UUID): Multi-tenant isolation
- `UserID` (UUID): Job owner
- `IdempotencyKey` (UUID): Prevents duplicate creation
- `Type`: Reference to JobType
- Status progression: init → waiting → running → final states, etc
- Progress tracking (0.0 to 100.0)
- Parameter storage (JSON)
- Result storage (JSON)
- Error information
- Timing metadata

**JobQueue**: Execution queue with worker pool
- Queue types: e.g. compute (capacity: 1), maintenance (capacity: 5), download (capacity: 10)
- Worker pool management
- Waiting job queue
- Running job tracking
- Job timeout and cancellation support

## Features

### Core Functionality
- **Queue-based job processing** with dedicated worker pools
- **Asynchronous job execution** with progress tracking
- **Idempotency support** to prevent duplications on job creation
- **Status management** with comprehensive state transitions
- **Retry mechanisms** with configurable delays and retry limits
- **Suspend/resume capabilities** for long-running jobs
- **Locking** to run nested jobs (e.g. benchmarking job creates and waits for dataset download job to finish)
- **Timeout handling** with automatic job termination
- **Progress reporting** with real-time updates
- **Result storage** with JSON serialization
- **Comprehensive API** with REST endpoints for typical operations - start, cancel, suspend/resumer, etc
- **Automatic job resume** for running jobsafter server restart
- **Callback** mechanism for job parameters validation and main job execution logic

## Job States and Their Transitions

The Job Executor uses the following states to manage job execution. Every job can be in one of the following states:

```
	// Initial states
	StatusInit     = "initializing"
	StatusWaiting  = "waiting"
	StatusResuming = "resuming" // resuming from suspended state

	// Running states
	StatusRunning    = "running"
	StatusCanceling  = "canceling"  // still running, but will be canceled
	StatusSuspending = "suspending" // still running, but will be suspended
	StatusLocked     = "locked"     // almost like running, but just indicates it's locked by another job

	// Intermediate states
	StatusSuspended = "suspended" // snapshot saved, dequeued from the 'running' queue
	StatusRetrying  = "retrying"  // job is retrying after a failure

	// Final immutable states
	StatusSkipped   = "skipped"
	StatusCanceled  = "canceled"
	StatusFailed    = "failed"
	StatusTimedOut  = "timeout"
	StatusCompleted = "completed"
	StatusDeleted   = "deleted"
```

The Job Executor controls the job state transition using a state machine that implements the following rules:

![Job States and Their Transitions](../../docs/img/job_states_transition.png)

## Implementing your new job type in code

### 1. Registering a Job Queue

Job queues define the concurrency level for job execution. Each queue has a capacity that determines how many jobs can run in parallel. There are several default queues:

- `compute`: single-threaded execution for CPU-intensive tasks (capacity: 1)
- `maintenance`: multi-threaded for system maintenance tasks (capacity: 5)
- `download`: high-concurrency for download operations (capacity: 10)

If a modules requires it's own job queue it can register it using the following code:

```go
// Register a job queue with a specific capacity
queueName := job.JobQueueName("my_custom_queue")
queue, err := job.JERegisterJobQueue(&job.JobQueueConfig{
    Name:     queueName,  // Unique queue identifier
    Capacity: 5,          // Number of concurrent jobs allowed
})
```

Queue parameters:
- `Name`: Unique identifier for the queue
- `Capacity`: Maximum number of jobs that can run concurrently in this queue

### 2. Registering a Job Group and Job Type

Job types define the behavior and properties of jobs, such as job input parameters that can be provided via API on job start. The job types are registered with the job system and can be referenced when creating new jobs in API.

Job groups are used to group related job types together and let them share common queue to contol maximum concurrency level.

```go
// Define a job group
jobGroup := &job.JobGroup{
    Name:        "my_job_group",
    Description: "My custom job group",
    Queue:       &job.JobQueueConfig{Capacity: 5, Name: queueName},
}

// Register the job group
job.RegisterJobGroup(jobGroup)

// Define custom job parameters struct for given type of job
type MyJobParams struct {
    InputFile  string `json:"input_file"`
    OutputFile string `json:"output_file"`
}

// Register the job type
myJobType := job.RegisterJobType(job.JobTypeParams{
    Group:                          jobGroup,
    Name:                           "process_file",
    Description:                    "Process a file and generate output",
    Params:                         &MyJobParams{},
    WorkerParamsValidationCallback: MyJobParamsValidator,
    WorkerExecutionCallback:        MyJobWorker,
    WorkerStateUpdateCallback:      nil,
    Timeout:                        30 * time.Minute,
    MaxRetries:                     3,
    RetryDelay:                     time.Minute,
    WorkerIsSuspendable:            true,
})
```

Job type parameters:
- `Group`: Associated JobGroup that defines queue and grouping
- `Name`: Unique name within the group
- `Description`: Human-readable description
- `Params`: Parameter struct type with JSON tags
- `WorkerParamsValidationCallback`: Function to validate parameters
- `WorkerExecutionCallback`: Function that performs the actual work
- `WorkerStateUpdateCallback`: Optional callback for state changes
- `Timeout`: Maximum execution time before job times out
- `MaxRetries`: Number of retry attempts on failure
- `RetryDelay`: Time to wait between retries
- `WorkerIsSuspendable`: Whether the job can be suspended and resumed

### 3. Writing a Job Parameter Validator Callback

The `WorkerParamsValidationCallback()` callback ensures that job parameters are valid before the job is scheduled via the /jobs REST API. The validator can return an error to prevent the job from being scheduled and so that error would be returned to the client as API response. In this case the HTTP status code will be taken from the corresponding errorx.Error type.

The callback is called from the main job executor worker when the job is being scheduled for execution. It accepts `ctx` and the `JobObj` interface instance can be used to retrieve the job id, parameters, tenant or user ID and other job properties.

```go
func MyWorkerParamsValidationCallback(ctx context.Context, job *job.JobObj) errorx.Error {
    // Get the parameters as the correct type
    params, ok := job.GetParamsPtr().(*MyJobParams)
    if !ok {
        return errorx.NewErrInternalServerError("invalid job parameters type")
    }

    // Validate required fields
    if params.InputFile == "" {
        return errorx.NewErrBadRequest("input_file is required")
    }

    if params.OutputFile == "" {
        // Set default output file if not provided
        params.OutputFile = params.InputFile + ".out"
    }

    // Log validation success with parameter details
    job.LogDebug("Parameters validated: input=%s, output=%s", params.InputFile, params.OutputFile)

    return nil
}
```

### 4. Writing a Job Worker

The `WorkerExecutionCallback()` callback contains the actual implementation of the job's functionality. The worker is started in a one of the worker pool go-routines in the background so that different job workers can run in parallel with their own `JobObj` instances.

The worker callback is called on the job start, but also when the job is resumed after suspension. The suspend/resume logic would be handled by the job system automatically if appropriate job type has `WorkerIsSuspendable` set to true.

The worker must handle the `ctx.Done()` channel to detect when the job is being canceled or suspended by the client or by timeout.

The worker must return `nil` in case of successful job completion, or an error in case of failure or `ctx.Done()` channel is closed. Additionally, the worker can report progress calling `job.SetProgress()` method or update the job result using `job.SetResult()` method.

```go
func MyWorkerExecutionCallback(ctx context.Context, job *job.JobObj) errorx.Error {
    // Get the parameters as the correct type
    params, ok := job.GetParamsPtr().(*MyJobParams)
    if !ok {
        return errorx.NewErrInternalServerError("invalid job parameters type")
    }

    // Initialize progress
    err := job.SetProgress(ctx, 0)
    if err != nil {
        return err
    }

    // Perform the actual work
    // This is where your job's main functionality goes
    for i := 0; i < 10; i++ {
        // Check if job was canceled
        select {
        case <-ctx.Done():
            return errorx.NewErrCanceled("job was canceled")
        default:
            // Continue execution
        }

        // Update progress
        progress := float32(i+1) / 10.0 * 100.0
        err = job.SetProgress(ctx, progress)
        if err != nil {
            return err
        }

        // Do some work...
        time.Sleep(time.Second)
    }

    // Set the final result that can be any JSON-serializable data
    result := struct {
        ProcessedFile string `json:"processed_file"`
        OutputSize    int64  `json:"output_size"`
    }{
        ProcessedFile: params.InputFile,
        OutputSize:    1024,
    }

    err = job.SetResult(ctx, &result)
    if err != nil {
        return err
    }

    return nil
}
```

### 5. Writing a Suspendable/Resumable Job Worker

Suspendable jobs are useful for long-running tasks that may need to be paused and resumed later, such as batch processing jobs, resource-intensive computations, or jobs that need to wait for external events.

#### 5.1 Registering a Suspendable Job Type

To create a suspendable job, set the `WorkerIsSuspendable` flag to `true` when registering the job type:

```go
myJobType := RegisterJobType(JobTypeParams{
    Group: &JobGroup{
        Name:        "my-group",
        Queue:       JobQueueCompute,
        Description: "My job group",
    },
    Name:                           "my-suspendable-job",
    Description:                    "A job that can be suspended and resumed",
    Params:                         &MyJobParams{},
    WorkerParamsValidationCallback: validateMyJobParams,
    WorkerExecutionCallback:        executeMyJob,
    WorkerIsSuspendable:            true, // Mark this job as suspendable
})
```

#### 5.2 Using Worker Snapshots

The job system provides `SaveWorkerSnapshot` and `LoadWorkerSnapshot` methods to persist and retrieve job state. These methods allow your job worker to:

1. Save its current state before being suspended
2. Load the saved state when resuming execution
3. Continue processing from where it left off

Worker snapshots can contain any JSON-serializable data structure, including:
- Progress tracking information
- Partially processed data
- Intermediate results
- Iteration counters
- Processing state machines

#### 5.3 Example: Batch Processing with Snapshots

Here's an example of a suspendable job that processes items in batches and can be suspended/resumed:

```go
type BatchProcessorParams struct {
    Items []string `json:"items"`
    BatchSize int  `json:"batch_size"`
}

type BatchProcessorSnapshot struct {
    ProcessedCount int      `json:"processed_count"`
    LastProcessed  string   `json:"last_processed"`
    Results        []string `json:"results"`
    Errors         []string `json:"errors"`
}

func executeBatchProcessorJob(ctx context.Context, job *JobObj) errorx.Error {
    // Get job parameters
    params, ok := job.GetParamsPtr().(*BatchProcessorParams)
    if !ok {
        return errorx.NewErrInternalServerError("invalid job parameters type")
    }

    totalItems := len(params.Items)
    if totalItems == 0 {
        return nil // Nothing to process
    }

    // Initialize or load snapshot
    var snapshot BatchProcessorSnapshot
    loadedSnapshot, err := job.LoadWorkerSnapshot()
    if err != nil {
        return err
    }

    if loadedSnapshot != nil {
        // Convert the generic snapshot back to our specific type
        snapshotJSON, _ := json.Marshal(loadedSnapshot)
        if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
            return errorx.NewErrInternalServerError("failed to unmarshal snapshot: %s", err.Error())
        }

        job.LogInfo("Resuming batch processing from item %d/%d",
            snapshot.ProcessedCount, totalItems)
    } else {
        // Initialize a new snapshot
        snapshot = BatchProcessorSnapshot{
            ProcessedCount: 0,
            Results:        make([]string, 0),
            Errors:         make([]string, 0),
        }
    }

    // Process items starting from where we left off
    for i := snapshot.ProcessedCount; i < totalItems; i++ {
        // Check for cancellation
        select {
        case <-ctx.Done():
            job.LogInfo("Job canceled, saving progress...")
            snapshot.ProcessedCount = i
            if err := job.SaveWorkerSnapshot(snapshot); err != nil {
                job.LogError("Failed to save snapshot: %s", err.Error())
            }
            return errorx.NewErrCanceled("job was canceled")
        default:
            // Continue processing
        }

        // Process the current item
        item := params.Items[i]
        result, err := processItem(item)

        if err != nil {
            snapshot.Errors = append(snapshot.Errors,
                fmt.Sprintf("Error processing %s: %s", item, err.Error()))
        } else {
            snapshot.Results = append(snapshot.Results, result)
        }

        snapshot.ProcessedCount = i + 1
        snapshot.LastProcessed = item

        // Update progress
        err = job.SetProgress(ctx, float32(i+1)/float32(totalItems)*100.0)
        if err != nil {
            return err
        }

        // Save snapshot periodically (e.g., after each batch)
        if (i+1) % params.BatchSize == 0 || i == totalItems-1 {
            if err := job.SaveWorkerSnapshot(snapshot); err != nil {
                job.LogError("Failed to save snapshot: %s", err.Error())
            }

            job.LogInfo("Processed %d/%d items", i+1, totalItems)

            // Simulate potential suspension point
            // In a real scenario, the job executor might suspend the job here
            // and resume it later
        }
    }

    // Set final result
    result := struct {
        ProcessedCount int      `json:"processed_count"`
        SuccessCount   int      `json:"success_count"`
        ErrorCount     int      `json:"error_count"`
        Results        []string `json:"results"`
        Errors         []string `json:"errors"`
    }{
        ProcessedCount: snapshot.ProcessedCount,
        SuccessCount:   len(snapshot.Results),
        ErrorCount:     len(snapshot.Errors),
        Results:        snapshot.Results,
        Errors:         snapshot.Errors,
    }

    return job.SetResult(ctx, result)
}

func processItem(item string) (string, error) {
    // Simulate processing an item
    time.Sleep(100 * time.Millisecond)
    return "Processed: " + item, nil
}
```

#### 5.4 Best Practices for Suspendable Jobs

1. **Idempotent Processing**: Ensure that your job can safely process the same item multiple times without side effects.

2. **Regular Snapshots**: Save snapshots at logical boundaries (e.g., after processing each batch) to minimize lost work.

3. **Snapshot Size Management**: Keep snapshots reasonably sized. For very large datasets, consider storing references or summaries rather than complete data.

4. **Progress Tracking**: Update the job progress regularly to provide visibility into the job's status.

5. **Cancellation Handling**: Always check for context cancellation and save your snapshot before exiting when canceled.

6. **Snapshot Validation**: Validate loaded snapshots to ensure they're in a consistent state before resuming.

7. **Logging**: Log when snapshots are saved and loaded to aid in debugging.

#### 5.5 Handling Job Suspension

When a job is suspended:

1. The job executor will call `setSuspending()` on the job
2. The job worker should detect this (via context cancellation) and save its state
3. When the job is later resumed, it will be scheduled with a new worker
4. The new worker loads the snapshot and continues from where the previous worker left off

This mechanism allows for graceful handling of system maintenance, resource constraints, or administrative actions without losing work progress.

### 6. JobObj Go-lang Interface Available for Workers

The `JobObj` interface provides several methods that can be used within job workers:

#### Accessing Job Information
- `GetJobID()`: Returns the unique job ID
- `GetTenantID()`: Returns the tenant ID associated with the job
- `GetUserID()`: Returns the user ID that created the job
- `GetType()`: Returns the job type name string
- `GetTypePtr()`: Returns a pointer to the `JobType` instance
- `GetParamsPtr()`: Returns a pointer to the job parameters provided through the REST API
- `GetTimeoutSec()`: Returns the job timeout duration in seconds
- `GetStatus()`: Returns the current job status as string
- `GetProgress()`: Returns the current progress percentage (0-100)
- `LoadWorkerSnapshot()`: Returns the current worker snapshot if any

#### Updating Job State
The methods below can be used to update the job state and typically would lock the job for the duration of the update to prevent concurrent updates from the job worker and from the job executor.

- `SetProgress(ctx, progress)`: Updates the job progress (0-100)
- `SetResult(ctx, result)`: Sets the job result data
- `SetSkipped(ctx, reason)`: Marks the job as skipped with a reason
- `SetLockedBy(ctx, uuid)`: Locks the job by another job
- `SetUnlocked(ctx)`: Unlocks a previously locked job
- `SetRetryPolicy(ctx, retryDelay, maxRetries, timeout)`: Updates retry policy
- `SaveWorkerSnapshot(snapshot any)`: Saves a worker snapshot to the database


#### Logging
- `LogDebug(format, args...)`: Logs debug information
- `LogInfo(format, args...)`: Logs informational messages
- `LogWarn(format, args...)`: Logs warning messages
- `LogError(format, args...)`: Logs error messages

#### Example Usage
```go
// Update progress
job.SetProgress(ctx, 50.0)

// Log information
job.LogInfo("Processing file %s", filename)

// Set final result
job.SetResult(ctx, &MyJobResult{
    Status: "success",
    Count:  42,
})
```

## Configuration Settings

The job module supports configuration through the `config.yaml` file under the `job_logger` section for logging settings. Job-specific configurations are handled through JobType registration and individual job parameters.

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
