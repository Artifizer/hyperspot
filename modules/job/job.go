package job

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/hypernetix/hyperspot/libs/utils"
	"gorm.io/gorm"
)

// JobStatus represents the current status of a job
type JobStatus string

const (
	// Initial states
	StatusInit     = "initializing"
	StatusWaiting  = "waiting"
	StatusResuming = "resuming"

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
)

const (
	// Initial states
	JobStatusInit     JobStatus = JobStatus(StatusInit)
	JobStatusWaiting  JobStatus = JobStatus(StatusWaiting)
	JobStatusResuming JobStatus = JobStatus(StatusResuming)

	// Running states
	JobStatusRunning    JobStatus = JobStatus(StatusRunning)
	JobStatusCanceling  JobStatus = JobStatus(StatusCanceling)
	JobStatusSuspending JobStatus = JobStatus(StatusSuspending)
	JobStatusLocked     JobStatus = JobStatus(StatusLocked)

	// Intermediate states
	JobStatusSuspended JobStatus = JobStatus(StatusSuspended)
	JobStatusRetrying  JobStatus = JobStatus(StatusRetrying)

	// Final immutable states
	JobStatusSkipped   JobStatus = JobStatus(StatusSkipped)
	JobStatusCanceled  JobStatus = JobStatus(StatusCanceled)
	JobStatusFailed    JobStatus = JobStatus(StatusFailed)
	JobStatusTimedOut  JobStatus = JobStatus(StatusTimedOut)
	JobStatusCompleted JobStatus = JobStatus(StatusCompleted)
	JobStatusDeleted   JobStatus = JobStatus(StatusDeleted)
)

const JobGroupSeparator = ":"

type JobGroup struct {
	Name        string          `json:"name" gorm:"primaryKey"`
	QueueName   JobQueueName    `json:"queue_name"`
	Queue       *JobQueueConfig `json:"-" gorm:"-"`
	Description string          `json:"description"`
}

type JobType struct {
	TypeID                    string                            `json:"type_id" gorm:"primaryKey"`
	Description               string                            `json:"description"`
	Group                     string                            `json:"group"`
	GroupPtr                  *JobGroup                         `json:"-" gorm:"foreignKey:Group;references:Name"`
	Name                      string                            `json:"name"`
	TimeoutSec                int                               `json:"timeout_sec"`
	MaxRetries                int                               `json:"max_retries"`     // Maximum allowed retries
	RetryDelaySec             int                               `json:"retry_delay_sec"` // Delay between retries
	Params                    interface{}                       `json:"params" gorm:"-"`
	ParamsSchema              string                            `json:"params_schema" gorm:"-"`
	WorkerInitCallback        JobWorkerParamsValidationCallback `json:"-" gorm:"-"`           // called within synchronous API call for job creation
	WorkerExecutionCallback   JobWorkerExecutionCallback        `json:"-" gorm:"-"`           // called in a go-routine as asynchronous job worker
	WorkerStateUpdateCallback JobWorkerStateUpdateCallback      `json:"-" gorm:"-"`           // optional callback for status update, called from job execution worker
	WorkerIsSuspendable       bool                              `json:"suspendable" gorm:"-"` // whether the job can be suspended/resumed
}

// JobIface is used for API and DB interfaces, it has exported fields for proper serialization
type Job struct {
	JobID           uuid.UUID    `json:"id" gorm:"index"`
	TenantID        uuid.UUID    `json:"tenant_id" gorm:"index"`
	UserID          uuid.UUID    `json:"user_id" gorm:"index"`
	IdempotencyKey  uuid.UUID    `json:"idempotency_key" gorm:"index"`
	Type            string       `json:"type" gorm:"index"`
	TypePtr         *JobType     `json:"-" gorm:"-"`
	QueueName       JobQueueName `json:"queue_name" gorm:"index"`
	ScheduledAtMs   int64        `json:"scheduled_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	UpdatedAtMs     int64        `json:"updated_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	StartedAtMs     int64        `json:"started_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	ETAMs           int64        `json:"eta" gorm:"index" readOnly:"true" doc:"unix timestamp in milliseconds"`
	LockedBy        uuid.UUID    `json:"locked_by" gorm:"index" readOnly:"true"`
	Progress        float32      `json:"progress" readOnly:"true"`
	ProgressDetails string       `json:"progress_details" readOnly:"true"`
	Status          JobStatus    `json:"status" gorm:"index" readOnly:"true"`
	Details         string       `json:"details"`

	TimeoutSec    int `json:"timeout_sec,omitempty"` // Default is taken from JobType
	MaxRetries    int `json:"max_retries,omitempty"` // Default is taken from JobType
	RetryDelaySec int `json:"retry_delay_sec,omitempty"`

	Retries int `json:"retries" readOnly:"true"` // Track current retry count

	Error     string      `json:"error,omitempty" readOnly:"true"`
	Result    string      `json:"result,omitempty" readOnly:"true"`
	Params    string      `json:"params,omitempty"` // job parameters as a JSON string
	ParamsPtr interface{} `json:"-" gorm:"-"`       // job parameters as a struct

	WorkerSnapshot string `json:"worker_snapshot"` // custom persistable worker snapshot

	//	Result interface{} `json:"result,omitempty" gorm:"type:json;serializer:json" readOnly:"true"`
	//	Params interface{} `json:"params,omitempty" gorm:"type:json;serializer:json"`
}

// JobObj has unexported fields and getter/setter APIs
// It is used for internal job operations and object safety
type JobObj struct {
	mu     utils.DebugMutex
	cancel context.CancelFunc
	priv   Job // private job data, not visible to the interfaces using JobObj
}

//
// Public JobObj interface that can be used by external code such as benchmarks,
// LLM managers, etc
//

// GetJobID returns the unique identifier (UUID) for this job.
//
// This method provides access to the immutable job identifier that uniquely
// identifies this job instance across the entire system. The job ID is
// generated when the job is created and never changes throughout the job's
// lifecycle. It can be used for job lookup, tracking, and correlation with
// other system components.
//
// Returns:
//   - uuid.UUID: The unique identifier of the job
func (j *JobObj) GetJobID() uuid.UUID {
	// immutable, no need to refresh from DB
	return j.priv.JobID
}

// GetTenantID returns the tenant identifier associated with this job.
//
// This method provides access to the immutable tenant ID that identifies
// which tenant owns this job. The tenant ID is used for multi-tenancy
// isolation and access control. Jobs are always scoped to a specific tenant
// and can only be accessed by users within that tenant.
//
// Returns:
//   - uuid.UUID: The tenant identifier that owns this job
func (j *JobObj) GetTenantID() uuid.UUID {
	// immutable, no need to refresh from DB
	return j.priv.TenantID
}

// GetUserID returns the identifier of the user who owns this job.
//
// This method provides access to the immutable user ID that identifies
// which user created or owns this job. The user ID is used for access control
// and attribution. It allows the system to track which user initiated a
// particular job and may be used for permissions checking.
//
// Returns:
//   - uuid.UUID: The user identifier that owns this job
func (j *JobObj) GetUserID() uuid.UUID {
	// immutable, no need to refresh from DB
	return j.priv.UserID
}

// GetType returns the string identifier for this job's type.
//
// This method provides access to the immutable job type string that identifies
// the kind of job this is. The job type determines the behavior, parameters,
// and execution logic for the job. It is a string in the format "group:name"
// where group is the job group and name is the specific job type within that group.
//
// Returns:
//   - string: The job type identifier string
func (j *JobObj) GetType() string {
	// immutable, no need to refresh from DB
	return j.priv.Type
}

// GetTypePtr returns a pointer to the JobType struct associated with this job.
//
// This method provides access to the complete JobType configuration for this job,
// including timeout settings, retry policies, parameter schemas, and worker callbacks.
// The JobType contains all the metadata and configuration needed to execute the job.
// This is a static immutable property and does not require database refresh.
//
// Returns:
//   - *JobType: Pointer to the JobType struct for this job
//
// Panics:
//   - If the job type is not properly initialized (logs an error)
func (j *JobObj) GetTypePtr() *JobType {
	// immutable, no need to refresh from DB
	if j.priv.TypePtr == nil {
		logger.Error("internal error: job type is not initialized!")
	}
	return j.priv.TypePtr
}

// GetParamsPtr returns a pointer to the job parameters struct.
//
// This method provides access to the typed parameters for this job. The returned
// interface{} should be type-asserted to the specific parameter type defined
// for this job type. The parameters contain all the input data needed for the
// job execution and are deserialized from the JSON stored in the database.
//
// Example usage:
//
//	params, ok := job.GetParamsPtr().(*MyJobParams)
//	if !ok {
//	    return errorx.NewErrInternalServerError("invalid job parameters type")
//	}
//
// Returns:
//   - interface{}: Pointer to the job parameters struct, which should be type-asserted
//     to the specific parameter type for this job
func (j *JobObj) GetParamsPtr() interface{} {
	// immutable, no need to refresh from DB
	return j.priv.ParamsPtr
}

// GetTimeoutSec returns the configured timeout duration (in seconds) for this job.
//
// This method retrieves the current timeout setting for the job, which determines
// how long the job is allowed to run before it is automatically timed out.
// The timeout is refreshed from the database on each call to ensure the latest
// value is used, as it can be modified during job execution.
//
// Returns:
//   - time.Duration: The timeout duration in seconds
func (j *JobObj) GetTimeoutSec() time.Duration {
	// mutable & public, always refresh from DB
	j.mu.Lock()
	defer j.mu.Unlock()

	j.dbGetFields(&j.priv.TimeoutSec)
	return time.Duration(j.priv.TimeoutSec) * time.Second
}

// GetStatus returns the current status of the job.
//
// This method retrieves the current job status from the database to ensure
// the most up-to-date value is returned. The status indicates the current
// state of the job in its lifecycle (e.g., waiting, running, completed, failed).
// If the job is not found in the database, JobStatusDeleted is returned.
//
// Returns:
//   - JobStatus: The current status of the job
func (j *JobObj) GetStatus() JobStatus {
	// mutable & public, always refresh from DB
	j.mu.Lock()
	defer j.mu.Unlock()

	status := j.priv.Status
	var dbStatus JobStatus

	errx := j.dbGetFields(&j.priv.Status)
	if errx == nil {
		dbStatus = status
	} else {
		switch errx.(type) {
		case *errorx.ErrNotFound:
			dbStatus = JobStatusDeleted
		default:
			dbStatus = status
		}
	}

	if status != dbStatus {
		panic(fmt.Sprintf("internal error: job status mismatch: %s != %s", status, dbStatus)) // FIXME
	}

	return j.priv.Status
}

// GetStatusErrorProgressSuccess returns multiple job status properties in a single call.
//
// This method provides an efficient way to retrieve several job status-related
// properties in a single database query. It returns the current status string,
// any error message associated with the job, the current progress percentage,
// and a boolean indicating whether the job completed successfully.
//
// Returns:
//   - string: The current job status as a string
//   - string: Any error message associated with the job (empty if no error)
//   - float32: The current progress value (0-100)
//   - bool: Whether the job completed successfully (true if status is "completed")
func (j *JobObj) GetStatusErrorProgressSuccess() (string, string, float32, bool) {
	// mutable & public, always refresh from DB
	j.mu.Lock()
	defer j.mu.Unlock()

	j.dbGetFields(&j.priv.Status, &j.priv.Error, &j.priv.Progress, &j.priv.Status)
	var success bool = false
	if j.priv.Status == JobStatusCompleted {
		success = true
	}
	return string(j.priv.Status), j.priv.Error, j.priv.Progress, success
}

// GetProgress returns the current progress percentage of the job.
//
// This method retrieves the current progress value from the database to ensure
// the most up-to-date value is returned. The progress is a float between 0 and 100
// that indicates how far along the job is in its execution. This value is updated
// by the job worker during execution.
//
// Returns:
//   - float32: The current progress percentage (0-100)
func (j *JobObj) GetProgress() float32 {
	// mutable & public, always refresh from DB
	j.mu.Lock()
	defer j.mu.Unlock()

	j.dbGetFields(&j.priv.Progress)
	return j.priv.Progress
}

// SetResult sets the result data for the job.
//
// This method allows the job worker to store the output data or result of the job.
// The result can be any JSON-serializable data structure and will be stored in the
// job record and visible in the API. This is typically called when a job completes
// successfully to provide the output data that can be retrieved by clients.
//
// Parameters:
//   - ctx: The context for the operation, which may include timeout or cancellation signals
//   - result: The result data to store (must be JSON-serializable)
//
// Returns:
//   - errorx.Error: An error if the operation fails, or nil on success
func (j *JobObj) SetResult(ctx context.Context, result interface{}) errorx.Error {
	return JESetResult(ctx, j.GetJobID(), result)
}

// SetSkipped marks the job as skipped with the provided reason.
//
// This method allows the job worker to indicate that the job was intentionally
// not executed. This is different from failure or cancellation and indicates
// that the job was determined to be unnecessary or redundant. The provided
// reason explains why the job was skipped.
//
// Parameters:
//   - ctx: The context for the operation, which may include timeout or cancellation signals
//   - reason: A string explaining why the job was skipped
//
// Returns:
//   - errorx.Error: An error if the operation fails, or nil on success
func (j *JobObj) SetSkipped(ctx context.Context, reason string) errorx.Error {
	return JESetSkipped(ctx, j.GetJobID(), reason)
}

// SetProgress updates the job's progress percentage.
//
// This method allows the job worker to report its progress during execution.
// The progress value should be between 0 and 100, where 0 indicates the job
// has just started and 100 indicates it is complete. This information is used
// for monitoring and displaying job progress to users.
//
// Parameters:
//   - ctx: The context for the operation, which may include timeout or cancellation signals
//   - progress: The current progress percentage (0-100)
//
// Returns:
//   - errorx.Error: An error if the operation fails, or nil on success
func (j *JobObj) SetProgress(ctx context.Context, progress float32) errorx.Error {
	return JESetProgress(ctx, j.GetJobID(), progress)
}

// SetLockedBy locks the job by another job identified by the provided UUID.
//
// This method allows one job to lock another job, creating a dependency relationship.
// When a job is locked, it cannot be executed until it is unlocked. This is used
// for implementing job dependencies where one job needs to wait for another job
// to complete before it can proceed.
//
// Parameters:
//   - ctx: The context for the operation, which may include timeout or cancellation signals
//   - lockedBy: The UUID of the job that is locking this job
//
// Returns:
//   - errorx.Error: An error if the operation fails, or nil on success
func (j *JobObj) SetLockedBy(ctx context.Context, lockedBy uuid.UUID) errorx.Error {
	return JESetLockedBy(ctx, j.GetJobID(), lockedBy)
}

// SetUnlocked removes any lock on the job, allowing it to be processed.
//
// This method releases a lock previously set by SetLockedBy, allowing the job
// to proceed with execution. This is typically called when a dependency has been
// resolved, such as when a parent job completes and allows its dependent jobs
// to start.
//
// Parameters:
//   - ctx: The context for the operation, which may include timeout or cancellation signals
//
// Returns:
//   - errorx.Error: An error if the operation fails, or nil on success
func (j *JobObj) SetUnlocked(ctx context.Context) errorx.Error {
	return JESetUnlocked(ctx, j.GetJobID())
}

// SetRetryPolicy sets the retry policy for the job.
//
// This method allows customizing the retry behavior for a job during its execution.
// It specifies how long to wait between retry attempts, how many times to retry,
// and the overall timeout for the job. This can be used to override the default
// retry policy defined in the job type.
//
// Parameters:
//   - ctx: The context for the operation, which may include timeout or cancellation signals
//   - retryDelay: The duration to wait between retry attempts
//   - maxRetries: The maximum number of retry attempts allowed
//   - timeout: The overall timeout for the job execution
//
// Returns:
//   - errorx.Error: An error if the operation fails, or nil on success
func (j *JobObj) SetRetryPolicy(ctx context.Context, retryDelay time.Duration, maxRetries int, timeout time.Duration) errorx.Error {
	return jeSetRetryPolicy(ctx, j, retryDelay, maxRetries, timeout)
}

// SaveWorkerSnapshot persists a custom worker state snapshot to the database.
//
// This method allows a job worker to save its current execution state as a JSON-serializable
// object. This is particularly useful for suspendable jobs that need to save their
// progress before being suspended and later resumed. The snapshot can contain any
// data needed to resume the job from where it left off.
//
// Parameters:
//   - snapshot: Any JSON-serializable object representing the worker's current state
//
// Returns:
//   - errorx.Error: An error if serialization or database operation fails, or nil on success
func (j *JobObj) SaveWorkerSnapshot(snapshot any) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	jsonData, err := json.Marshal(snapshot)
	if err != nil {
		return errorx.NewErrInternalServerError("failed to marshal job worker snapshot: %s", err.Error())
	}
	j.priv.WorkerSnapshot = string(jsonData)
	return j.dbSaveFields(&j.priv.WorkerSnapshot)
}

// LoadWorkerSnapshot retrieves a previously saved worker state snapshot from the database.
//
// This method allows a job worker to retrieve its previously saved execution state
// when resuming a suspended job. The snapshot is deserialized from JSON into a generic
// interface{} that should be type-asserted by the caller to the expected type.
//
// Returns:
//   - any: The deserialized worker snapshot, which should be type-asserted by the caller
//   - errorx.Error: An error if deserialization or database operation fails, or nil on success
func (j *JobObj) LoadWorkerSnapshot() (any, errorx.Error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if err := j.dbGetFields(&j.priv.WorkerSnapshot); err != nil {
		return nil, err
	}

	// If the snapshot field is empty, return nil
	if j.priv.WorkerSnapshot == "" {
		return nil, nil
	}

	var snapshot any
	err := json.Unmarshal([]byte(j.priv.WorkerSnapshot), &snapshot)
	if err != nil {
		return nil, errorx.NewErrInternalServerError("failed to unmarshal job worker snapshot: %s", err.Error())
	}
	return snapshot, nil
}

//
// Private interface accessible from the job executor only
//

func (j *JobObj) getStartedAt() int64 {
	return j.priv.StartedAtMs
}

func (j *JobObj) getScheduledAt() int64 {
	return j.priv.ScheduledAtMs
}

func (j *JobObj) getRetries() int {
	return j.priv.Retries
}

func (j *JobObj) getETA() int64 {
	return j.priv.ETAMs
}

func (j *JobObj) getError() string {
	return j.priv.Error
}

func (j *JobObj) getQueueName() JobQueueName {
	return j.GetTypePtr().GroupPtr.Queue.Name
}

func (j *JobObj) getMaxRetries() int {
	return j.priv.MaxRetries
}

func (j *JobObj) getLockedBy() uuid.UUID {
	return j.priv.LockedBy
}

func (j *JobObj) getRetryDelay() time.Duration {
	return time.Duration(j.priv.RetryDelaySec) * time.Second
}

func (j *JobObj) getParams() string {
	return j.priv.Params
}

func (j *JobObj) getIdempotencyKey() uuid.UUID {
	// immutable, no need to refresh from DB
	return j.priv.IdempotencyKey
}

func (j *JobObj) setStatus(status JobStatus, statusErr string, fields ...interface{}) errorx.Error {
	var currentStatus JobStatus

	j.mu.Lock()
	defer j.mu.Unlock()

	if logger.GetMinLogLevel() >= logging.DebugLevel {
		errx := j.dbGetFields(&j.priv.Status)
		if errx != nil {
			j.LogError("failed to get job status from DB: %s, stack: %s", errx.Error(), string(debug.Stack()))
		}
		currentStatus = j.priv.Status
		j.LogTrace("setStatus(%s@%p) - status is '%s', stack: %s", j.priv.JobID, j, status, string(debug.Stack()))
	}

	j.priv.Status = status
	if statusErr == "" {
		j.LogDebug("%s -> %s", currentStatus, status)
		return j.dbSaveFields(append([]interface{}{&j.priv.Status}, fields...)...)
	} else {
		j.priv.Error = statusErr
		j.LogDebug("%s -> %s (%s)", currentStatus, status, statusErr)
		return j.dbSaveFields(append([]interface{}{&j.priv.Status, &j.priv.Error}, fields...)...)
	}
}

func (j *JobObj) getStatus() JobStatus {
	j.mu.Lock()
	defer j.mu.Unlock()

	currentStatus := j.priv.Status
	errx := j.dbGetFields(&j.priv.Status)
	if errx != nil {
		j.LogError("failed to get job status from DB: %s, stack: %s", errx.Error(), string(debug.Stack()))
	}
	if currentStatus != j.priv.Status {
		j.LogError("internal error: job status mismatch: %s != %s, stack: %s", currentStatus, j.priv.Status, string(debug.Stack()))
	}
	j.LogDebug("getStatus(%s@%p) - status is '%s', stack: %s", j.priv.JobID, j, j.priv.Status, string(debug.Stack()))
	return j.priv.Status
}

func (j *JobObj) setResult(result interface{}) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var err error
	j.priv.Result, err = utils.StructToJSONString(result)
	if err != nil {
		j.LogError("failed to marshal job result: %s", err.Error())
		return errorx.NewErrInternalServerError("failed to marshal job result: %s", err.Error())
	}
	return j.dbSaveFields(&j.priv.Result)
}

func (j *JobObj) setProgress(progress float32) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if progress < 0 {
		return errorx.NewErrBadRequest("invalid progress value: %.1f", progress)
	}

	if progress > 100 {
		// Sometimes timer-based progress can report value > 100
		progress = 100
	}

	if progress == j.priv.Progress {
		return nil
	}

	j.priv.Progress = progress

	if progress < 100 && time.Now().UnixMilli()-j.priv.UpdatedAtMs < 1 {
		return nil
	}

	j.logProgress(progress)

	errx := j.dbSaveFields(&j.priv.Progress)
	if errx != nil {
		j.LogError("failed to update job progress: %s", errx.Error())
		return errx
	}

	return nil
}

func (j *JobObj) setSkipped(reason string) errorx.Error {
	err := j.setStatus(JobStatusSkipped, reason)
	if err != nil {
		return err
	}

	return j.setProgress(100)
}

func (j *JobObj) delete() errorx.Error {
	j.setStatus(JobStatusDeleted, "")

	j.mu.Lock()
	defer j.mu.Unlock()

	err := db.DB().Delete(&Job{}, "tenant_id = ? AND user_id = ? AND job_id = ?", j.priv.TenantID, j.priv.UserID, j.priv.JobID).Error
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	return nil
}

func (j *JobObj) statusIsFinal(status JobStatus) bool {
	return status == JobStatusSkipped ||
		status == JobStatusCanceled ||
		status == JobStatusFailed ||
		status == JobStatusTimedOut ||
		status == JobStatusCompleted ||
		status == JobStatusDeleted
}

// shouldRetryError determines if an error should trigger a retry
func shouldRetryError(err error) bool {
	// Check for database locking errors
	return err != nil && strings.Contains(err.Error(), "database table is locked")
}

func (j *JobObj) dumpStack() {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("Stack trace:\n")
	buf.WriteString(string(debug.Stack()))
	j.LogError("%s", buf.String())
}

func (j *JobObj) setRetryPolicy(retryDelay time.Duration, maxRetries int, timeout time.Duration) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if retryDelay.Seconds() < 0 {
		retryDelay = 0
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	if timeout.Seconds() < 0 {
		timeout = 0
	}

	j.priv.RetryDelaySec = int(retryDelay.Seconds())
	j.priv.MaxRetries = maxRetries
	j.priv.TimeoutSec = int(timeout.Seconds())

	return j.dbSaveFields(&j.priv.RetryDelaySec, &j.priv.TimeoutSec, &j.priv.MaxRetries)
}

func (j *JobObj) initType(typeStr string) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var ok bool

	j.priv.Type = typeStr
	if typeStr == "" {
		return errorx.NewErrBadRequest("job type is not set for job %s", j.priv.JobID.String())
	}

	j.priv.TypePtr, ok = jobTypesMap[typeStr]
	if !ok {
		return errorx.NewErrBadRequest("failed to get job type: %s", typeStr)
	}
	j.priv.QueueName = j.priv.TypePtr.GroupPtr.Queue.Name

	return nil
}

// initParams initializes job parameters by merging provided parameters with defaults.
// If paramsStr is empty, it uses the default parameters from the job type.
// If paramsStr is provided, it validates the JSON, applies defaults for any missing fields,
// and then unmarshals the merged result into the job's parameters.
func (j *JobObj) initParams(paramsStr string) errorx.Error {
	jobType := j.GetTypePtr()
	if jobType == nil {
		return errorx.NewErrBadRequest("job type is not set for job %s", j.priv.JobID.String())
	}

	if jobType.Params == nil {
		if paramsStr != "" {
			return errorx.NewErrBadRequest("job type '%s' doesn't support parameters", jobType.TypeID)
		}
		return nil
	}

	// Create a new instance of the params struct for the job type.
	var paramsObj interface{}
	paramsType := reflect.TypeOf(jobType.Params)
	if paramsType.Kind() == reflect.Ptr {
		paramsObj = reflect.New(paramsType.Elem()).Interface()
	} else {
		paramsObj = reflect.New(paramsType).Interface()
	}

	// Use the generic merging function (which applies defaults, merges incoming JSON,
	// validates and unmarshals the result).
	mergedJSON, err := utils.MergeJSONWithDefaults(paramsObj, paramsStr)
	if err != nil {
		j.LogError(err.Error())
		return errorx.NewErrBadRequest(err.Error())
	}

	j.priv.Params = mergedJSON
	j.priv.ParamsPtr = paramsObj
	return nil
}

func (j *JobObj) setParams(paramsStr string) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if errx := j.initParams(paramsStr); errx != nil {
		return errx
	}
	return j.dbSaveFields(&j.priv.Params)
}

func (j *JobObj) dbSaveFields(fields ...interface{}) errorx.Error {
	if j.mu.TryLock() {
		panic("JobObj.dbSaveFields() must be called with the job mutex locked")
	}

	j.priv.UpdatedAtMs = time.Now().UnixMilli()
	fields = append(fields, &j.priv.UpdatedAtMs)

	pkFields := map[string]interface{}{
		"job_id":    j.priv.JobID,
		"user_id":   j.priv.UserID,
		"tenant_id": auth.GetTenantID(),
	}

	errx := orm.OrmUpdateObjFields(&j.priv, pkFields, fields...)
	if errx != nil {
		msg := fmt.Sprintf("failed to update job: %s", errx.Error())
		j.LogError(msg)
		j.priv.Error = msg
		return errorx.NewErrInternalServerError(msg)
	}

	// Check if any of the updated fields are progress, status, error, or result
	// These are the fields that should trigger the WorkerStateUpdateCallback
	shouldCallCallback := false
	progressPtr := &j.priv.Progress
	statusPtr := &j.priv.Status
	errorPtr := &j.priv.Error
	resultPtr := &j.priv.Result

	for _, field := range fields {
		if field == progressPtr || field == statusPtr || field == errorPtr || field == resultPtr {
			shouldCallCallback = true
			break
		}
	}

	// If job progress, status, error or result is updated, call the callback
	if shouldCallCallback {
		j.mu.Unlock()
		defer j.mu.Lock()

		if j.GetTypePtr() != nil && j.GetTypePtr().WorkerStateUpdateCallback != nil {
			errx := j.GetTypePtr().WorkerStateUpdateCallback(j)
			if errx != nil {
				j.LogError("failed to update job status: %s", errx.Error())
				return errx
			}
		}
	}

	return nil
}

func (j *JobObj) dbGetFields(fields ...interface{}) errorx.Error {
	if j.mu.TryLock() {
		panic("JobObj.dbSaveFields() must be called with the job mutex locked")
	}

	pkFields := map[string]interface{}{
		"job_id":    j.priv.JobID,
		"user_id":   j.priv.UserID,
		"tenant_id": auth.GetTenantID(),
	}
	if errx := orm.OrmGetObjFields(&j.priv, pkFields, fields...); errx != nil {
		j.LogError("failed to get job fields: %s, stack: %s", errx.Error(), string(debug.Stack()))
		return errx
	}
	return nil
}

func (j *JobObj) schedule() errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now().UTC()

	// Convert params to JSON string for logging
	paramsJSON, _ := json.MarshalIndent(j.priv.ParamsPtr, "", "  ")
	j.LogInfo("scheduling with params: %s", string(paramsJSON))

	j.priv.Status = JobStatusWaiting
	j.priv.ScheduledAtMs = now.UnixMilli()
	j.priv.ETAMs = now.Add(time.Duration(j.priv.TimeoutSec) * time.Second).UnixMilli()

	return j.dbSaveFields(&j.priv.Status, &j.priv.ScheduledAtMs, &j.priv.ETAMs)
}

func (j *JobObj) setLockedBy(lockedBy uuid.UUID) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.priv.Status = JobStatusLocked
	j.priv.LockedBy = lockedBy
	j.priv.UpdatedAtMs = time.Now().UnixMilli()

	if lockedBy == j.priv.JobID {
		return errorx.NewErrBadRequest("trying to lock job by itself")
	}

	// Recursively check if there's a circular lock dependency
	visited := make(map[uuid.UUID]bool)
	visited[j.priv.JobID] = true

	var checkLockChain func(currentLockedBy uuid.UUID) error
	checkLockChain = func(currentLockedBy uuid.UUID) error {
		if currentLockedBy == uuid.Nil {
			return nil
		}

		// If we've seen this job ID before, we have a cycle
		if visited[currentLockedBy] {
			return fmt.Errorf("circular lock dependency detected")
		}

		// Mark this job as visited
		visited[currentLockedBy] = true

		// Get the job that holds the lock using GetJob
		lockingJob, err := getJob(currentLockedBy)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // Job not found, end of chain
			}
			return fmt.Errorf("failed to check locking job: %w", err)
		}

		// Recursively check the job that locked this one
		return checkLockChain(lockingJob.priv.LockedBy)
	}

	if err := checkLockChain(lockedBy); err != nil {
		return errorx.NewErrBadRequest(err.Error())
	}

	j.LogDebug("locked by job %s", lockedBy.String())

	return j.dbSaveFields(&j.priv.Status, &j.priv.LockedBy, &j.priv.UpdatedAtMs)
}

func (j *JobObj) setUnlocked() errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.priv.Status = JobStatusRunning
	j.priv.LockedBy = uuid.Nil
	j.priv.UpdatedAtMs = time.Now().UnixMilli()
	j.LogDebug("unlocked")

	return j.dbSaveFields(&j.priv.Status, &j.priv.LockedBy, &j.priv.UpdatedAtMs)
}

func (j *JobObj) setRunning() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now().UTC()

	j.priv.Status = JobStatusRunning
	j.priv.StartedAtMs = now.UnixMilli()
	j.LogDebug("running ...")

	return j.dbSaveFields(&j.priv.Status, &j.priv.StartedAtMs)
}

// This is internal method called by JobExecutor to initiate job cancellation
// Job workers must use setCanceled
func (j *JobObj) setCanceling(reason string) errorx.Error {
	return j.setStatus(JobStatusCanceling, reason)
}

func (j *JobObj) setCanceled(reason string) errorx.Error {
	j.priv.LockedBy = uuid.Nil
	return j.setStatus(JobStatusCanceled, reason, &j.priv.LockedBy)
}

func (j *JobObj) setTimedOut(reason string) errorx.Error {
	return j.setStatus(JobStatusTimedOut, reason)
}

func (j *JobObj) setCompleted(msg string) errorx.Error {
	if msg == "" {
		msg = "completed successfully"
		j.LogInfo(msg)
	} else {
		j.LogInfo("completed with reason: %s", msg)
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	j.priv.Status = JobStatusCompleted
	j.priv.Progress = 100
	j.priv.Details = msg
	j.priv.LockedBy = uuid.Nil

	return j.dbSaveFields(&j.priv.Status, &j.priv.Progress, &j.priv.Details, &j.priv.LockedBy)
}

func (j *JobObj) setFailed(reason string) errorx.Error {
	return j.setStatus(JobStatusFailed, reason)
}

func (j *JobObj) setRetrying(reason string) errorx.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.priv.Retries++
	j.priv.ScheduledAtMs = time.Now().UTC().Add(time.Duration(j.priv.RetryDelaySec) * time.Second).UnixMilli()
	j.priv.Status = JobStatusRetrying
	j.priv.Error = fmt.Sprintf("Retrying attempt %d/%d: %s", j.priv.Retries, j.GetTypePtr().MaxRetries, reason)

	if reason == "" {
		j.LogDebug("retrying ...")
	} else {
		j.LogDebug("retrying with reason: %s ...", reason)
	}

	return j.dbSaveFields(&j.priv.Retries, &j.priv.Status, &j.priv.ScheduledAtMs, &j.priv.Error)
}

// setSuspending sets the job status to 'suspending'
func (j *JobObj) setSuspending(reason string) errorx.Error {
	if !j.GetTypePtr().WorkerIsSuspendable {
		panic(fmt.Sprintf("job of type %s doesn't support suspend operation", j.GetTypePtr().TypeID))
	}

	return j.setStatus(JobStatusSuspending, reason)
}

// SetSuspended sets the job status to suspended
func (j *JobObj) setSuspended(reason string) errorx.Error {
	return j.setStatus(JobStatusSuspended, reason)
}

func newJob(
	ctx context.Context,
	idempotencyKey uuid.UUID,
	jobType *JobType,
	paramsStr string,
) (*JobObj, errorx.Error) {
	if jobType == nil {
		return nil, errorx.NewErrBadRequest("job type is not set")
	}

	if _, ok := jobTypesMap[jobType.TypeID]; !ok {
		panic(fmt.Sprintf("job type '%s' must be registered before creating a job", jobType.Name))
	}

	j := &JobObj{
		priv: Job{
			TenantID:       auth.GetTenantID(),
			UserID:         auth.GetUserID(),
			JobID:          uuid.New(),
			IdempotencyKey: idempotencyKey,
			Status:         JobStatusInit,
			LockedBy:       uuid.Nil,
			UpdatedAtMs:    time.Now().UnixMilli(),
			ScheduledAtMs:  time.Now().UnixMilli(),
			Progress:       0,
			Retries:        0,
			MaxRetries:     jobType.MaxRetries,
			Error:          "",
			RetryDelaySec:  jobType.RetryDelaySec,
			TimeoutSec:     jobType.TimeoutSec,
		},
	}

	if err := j.initType(jobType.TypeID); err != nil {
		return nil, err
	}

	if err := j.initParams(paramsStr); err != nil {
		return nil, err
	}

	if db.DB() == nil {
		return nil, errorx.NewErrInternalServerError("database is not initialized")
	}

	if jobType.WorkerInitCallback != nil {
		// FIXME: need to store private data in memory...
		errx := jobType.WorkerInitCallback(ctx, j)
		if errx != nil {
			return nil, errorx.NewErrInternalServerError("failed to initialize job '%s': %s", jobType.TypeID, errx.Error())
		}
	}

	// Start a transaction to ensure we don't lock the database
	tx := db.DB().Begin()
	if tx.Error != nil {
		msg := fmt.Sprintf("failed to begin transaction: %s", tx.Error.Error())
		j.LogError(msg)
		return nil, errorx.NewErrInternalServerError(msg)
	}

	// Create the job within the transaction
	err := tx.Create(&j.priv).Error
	if err != nil {
		tx.Rollback() // Roll back on error
		msg := fmt.Sprintf("failed to create job in DB: %s", err.Error())
		j.LogError(msg)
		return nil, errorx.NewErrInternalServerError(msg)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		msg := fmt.Sprintf("failed to commit transaction: %s", err.Error())
		j.LogError(msg)
		return nil, errorx.NewErrInternalServerError(msg)
	}

	return j, nil
}

// JobWorkerStateUpdateCallback is a callback function that updates target worker properties,
// such as Progress, Status, Error and Success
type JobWorkerStateUpdateCallback func(job *JobObj) errorx.Error

// JobWorkerParamsValidationCallback is a callback function for job paramemeter initialisation.
// The function returns target worker object associated with current job and an error if any.
// It's being called from the synchronous API handler creating the job, so if error
// is returned, the job creation will fail
type JobWorkerParamsValidationCallback func(ctx context.Context, job *JobObj) errorx.Error

// JobWorkerExecutionCallback is a callback function for target worker execution.
// It run asynchronously in a dedicated go-routine, if error is returned then
// the job will be marked as failed. Otherwise it will be marked as completed successfully.
// This callback is called when the job is first started and again after resume.
// On resume, the worker is responsible for worker state snasphot loading and
// setting the job progress to the last proper value.
type JobWorkerExecutionCallback func(ctx context.Context, job *JobObj) errorx.Error

type JobTypeParams struct {
	Group                          *JobGroup
	Name                           string
	Description                    string
	Params                         interface{}
	WorkerParamsValidationCallback JobWorkerParamsValidationCallback // Initial job initialisation callback that is called on job start or resume
	WorkerExecutionCallback        JobWorkerExecutionCallback        // main worker callback that is called on job start or after resume
	WorkerStateUpdateCallback      JobWorkerStateUpdateCallback      // optional callback for linked objects state updates
	WorkerIsSuspendable            bool                              // if true, the job can be suspended and resumed
	Timeout                        time.Duration
	RetryDelay                     time.Duration
	MaxRetries                     int
}

// jobTypes holds worker functions for each job type.
var jobTypesLock utils.DebugMutex
var jobGroupsLock utils.DebugMutex
var jobTypes = []*JobType{}
var jobGroups = []*JobGroup{}
var jobTypesMap = map[string]*JobType{}
var jobGroupsMap = map[string]*JobGroup{}

func RegisterJobGroup(group *JobGroup) {
	jobGroupsLock.Lock()
	defer jobGroupsLock.Unlock()

	if _, ok := jobGroupsMap[group.Name]; ok {
		return
	}

	if group.Queue == nil {
		panic("job group queue is not set")
	}

	group.QueueName = group.Queue.Name

	_, err := jeGetJobQueue(group.QueueName)
	if err != nil {
		panic(fmt.Sprintf("the job queue '%s' is not registered: %s", group.QueueName, err.Error()))
	}

	jobGroupsMap[group.Name] = group
	jobGroups = append(jobGroups, group)
}

func RegisterJobType(params JobTypeParams) *JobType {
	jobTypesLock.Lock()
	defer jobTypesLock.Unlock()

	if params.Group == nil {
		panic("internal error: trying to register jobType w/o GroupPtr set")
	}

	typeID := params.Group.Name + JobGroupSeparator + params.Name
	if _, ok := jobTypesMap[typeID]; ok {
		panic(fmt.Sprintf("internal error: trying to register jobType %s, but it already exists", typeID))
	}

	if _, ok := jobGroupsMap[params.Group.Name]; !ok {
		RegisterJobGroup(params.Group)
	}

	jt := &JobType{
		TypeID:                    typeID,
		GroupPtr:                  params.Group,
		Group:                     params.Group.Name,
		Name:                      params.Name,
		Description:               params.Description,
		Params:                    params.Params,
		ParamsSchema:              utils.GenerateSchemaForPOSTPUTString(params.Params),
		TimeoutSec:                int(params.Timeout.Seconds()),
		MaxRetries:                params.MaxRetries,
		RetryDelaySec:             int(params.RetryDelay.Seconds()),
		WorkerInitCallback:        params.WorkerParamsValidationCallback,
		WorkerExecutionCallback:   params.WorkerExecutionCallback,
		WorkerStateUpdateCallback: params.WorkerStateUpdateCallback,
		WorkerIsSuspendable:       params.WorkerIsSuspendable,
	}

	jobTypesMap[jt.TypeID] = jt
	jobTypes = append(jobTypes, jt)

	return jt
}

func getJobType(type_id string) (*JobType, bool) {
	jt, ok := jobTypesMap[type_id]
	return jt, ok
}

func getJobTypes(ctx context.Context, pageRequest *api.PageAPIRequest) []*JobType {
	return api.PageAPIPaginate(jobTypes, pageRequest)
}

func getJobGroup(name string) (*JobGroup, bool) {
	group, ok := jobGroupsMap[name]
	return group, ok
}

func getJobGroups(ctx context.Context, pageRequest *api.PageAPIRequest) []*JobGroup {
	return api.PageAPIPaginate(jobGroups, pageRequest)
}

func listJobs(ctx context.Context, pageRequest *api.PageAPIRequest, status string) ([]*JobObj, error) {
	query, err := orm.GetBaseQuery(&Job{}, auth.GetTenantID(), uuid.Nil, pageRequest)
	if err != nil {
		return nil, err
	}

	// (Optional) Apply a status filter if provided.
	if status != "" {
		statuses := strings.Split(status, ",")
		query = query.Where("status IN (?)", statuses)
	}

	var jobs []*Job
	var jobsObj []*JobObj

	if err := query.Find(&jobs).Error; err != nil {
		return nil, err
	}

	for _, job := range jobs {
		jobObj := &JobObj{priv: *job}
		if err := jobObj.initType(job.Type); err != nil {
			logger.Warn("Failed to init job type for job %s: %v", job.JobID, err)
		}
		if err := jobObj.initParams(job.Params); err != nil {
			logger.Warn("Failed to init job params for job %s: %v", job.JobID, err)
		}
		jobsObj = append(jobsObj, jobObj)
	}

	return jobsObj, nil
}

func getJob(jobID uuid.UUID) (*JobObj, errorx.Error) {
	var jobData Job
	if err := db.DB().Where("tenant_id = ? AND user_id = ? AND job_id = ?", auth.GetTenantID(), auth.GetUserID(), jobID).First(&jobData).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errorx.NewErrNotFound("job not found")
		}
		return nil, errorx.NewErrInternalServerError(err.Error())
	}

	job := &JobObj{priv: jobData}
	if err := job.initType(jobData.Type); err != nil {
		return nil, errorx.NewErrInternalServerError("failed to init job type: %v", err)
	}
	if err := job.initParams(jobData.Params); err != nil {
		return nil, errorx.NewErrInternalServerError("failed to init job params: %v", err)
	}
	return job, nil
}

func getFirstWaitingJob(queueName string) (uuid.UUID, errorx.Error) {
	var jobIDStr string

	// Retry logic for transient database issues
	maxRetries := 3
	backoff := 10 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		// Start a transaction to avoid locking issues
		tx := db.DB().Begin()
		if tx.Error != nil {
			if attempt < maxRetries-1 && shouldRetryError(tx.Error) {
				continue
			}
			return uuid.Nil, errorx.NewErrInternalServerError("failed to begin transaction: %s", tx.Error.Error())
		}

		// Select the oldest scheduled job with waiting status and return only the job ID as a string
		err := tx.Model(&Job{}).Select("job_id").Where("tenant_id = ? AND user_id = ? AND queue_name = ? AND status = ?",
			auth.GetTenantID(), auth.GetUserID(), queueName, JobStatusWaiting).Order("scheduled_at_ms ASC").First(&jobIDStr).Error

		if err != nil {
			tx.Rollback()
			if err == gorm.ErrRecordNotFound {
				return uuid.Nil, errorx.NewErrNotFound("job not found")
			}

			// Check if this is a retryable error (e.g., "no such table" during test startup)
			if attempt < maxRetries-1 && (shouldRetryError(err) || strings.Contains(err.Error(), "no such table")) {
				continue
			}

			return uuid.Nil, errorx.NewErrInternalServerError(err.Error())
		}

		// Commit the transaction
		if err := tx.Commit().Error; err != nil {
			if attempt < maxRetries-1 && shouldRetryError(err) {
				continue
			}
			return uuid.Nil, errorx.NewErrInternalServerError("failed to commit transaction: %s", err.Error())
		}

		// Parse the string into a UUID
		jobID, parseErr := uuid.Parse(jobIDStr)
		if parseErr != nil {
			return uuid.Nil, errorx.NewErrInternalServerError("failed to parse job ID: %s", parseErr.Error())
		}

		return jobID, nil
	}

	return uuid.Nil, errorx.NewErrInternalServerError("failed to get first waiting job after %d attempts", maxRetries)
}

func init() {
	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			&Job{},
		},
		InitAPIRoutes: registerJobAPIRoutes,
		Name:          "jobs",
		InitMain: func() error {
			return JEInit()
		},
	})
}
