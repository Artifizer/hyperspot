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
	StatusSuspended = "suspended" // dequeued from the 'running' queue

	// Final immutable states
	StatusSkipped   = "skipped"
	StatusCanceled  = "canceled"
	StatusFailed    = "failed"
	StatusTimedOut  = "timeout"
	StatusCompleted = "completed"
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

	// Final immutable states
	JobStatusSkipped   JobStatus = JobStatus(StatusSkipped)
	JobStatusCanceled  JobStatus = JobStatus(StatusCanceled)
	JobStatusFailed    JobStatus = JobStatus(StatusFailed)
	JobStatusTimedOut  JobStatus = JobStatus(StatusTimedOut)
	JobStatusCompleted JobStatus = JobStatus(StatusCompleted)
)

const JobGroupSeparator = ":"

type JobGroup struct {
	Name        string       `json:"name" gorm:"primaryKey"`
	Queue       JobQueueName `json:"queue_name"`
	Description string       `json:"description"`
}

type JobType struct {
	TypeID                    string                       `json:"type_id" gorm:"primaryKey"`
	Description               string                       `json:"description"`
	Group                     string                       `json:"group"`
	GroupPtr                  *JobGroup                    `json:"-" gorm:"foreignKey:Group;references:Name"`
	Name                      string                       `json:"name"`
	TimeoutSec                int                          `json:"timeout_sec"`
	MaxRetries                int                          `json:"max_retries"`     // Maximum allowed retries
	RetryDelaySec             int                          `json:"retry_delay_sec"` // Delay between retries
	Params                    interface{}                  `json:"params" gorm:"-"`
	ParamsSchema              string                       `json:"params_schema" gorm:"-"`
	WorkerInitCallback        JobWorkerInitCallback        `json:"-" gorm:"-"`           // called within synchronous API call for job creation
	WorkerExecutionCallback   JobWorkerExecutionCallback   `json:"-" gorm:"-"`           // called in a go-routine as asynchronous job worker
	WorkerStateUpdateCallback JobWorkerStateUpdateCallback `json:"-" gorm:"-"`           // optional callback for status update, called from job execution worker
	WorkerSuspendCallback     JobWorkerSuspendCallback     `json:"-" gorm:"-"`           // optional callback for job suspension, called synchronously via API
	WorkerResumeCallback      JobWorkerResumeCallback      `json:"-" gorm:"-"`           // optional callback for job resume, called synchronously via API
	WorkerIsSuspendable       bool                         `json:"suspendable" gorm:"-"` // whether the job can be suspended/resumed
}

// JobIface is used for API and DB interfaces, it has exported fields for proper serialization
type Job struct {
	JobID           uuid.UUID `json:"id" gorm:"index"`
	TenantID        uuid.UUID `json:"tenant_id" gorm:"index"`
	UserID          uuid.UUID `json:"user_id" gorm:"index"`
	IdempotencyKey  uuid.UUID `json:"idempotency_key" gorm:"index"`
	Type            string    `json:"type" gorm:"index"`
	TypePtr         *JobType  `json:"-" gorm:"-"`
	ScheduledAtMs   int64     `json:"scheduled_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	UpdatedAtMs     int64     `json:"updated_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	StartedAtMs     int64     `json:"started_at" gorm:"index" doc:"unix timestamp in milliseconds"`
	ETAMs           int64     `json:"eta" gorm:"index" readOnly:"true" doc:"unix timestamp in milliseconds"`
	LockedBy        uuid.UUID `json:"locked_by" gorm:"index" readOnly:"true"`
	Progress        float32   `json:"progress" readOnly:"true"`
	ProgressDetails string    `json:"progress_details" readOnly:"true"`
	Status          JobStatus `json:"status" gorm:"index" readOnly:"true"`
	Details         string    `json:"details"`

	TimeoutSec    int `json:"timeout_sec,omitempty"` // Default is taken from JobType
	MaxRetries    int `json:"max_retries,omitempty"` // Default is taken from JobType
	RetryDelaySec int `json:"retry_delay_sec,omitempty"`

	Retries int `json:"retries" readOnly:"true"` // Track current retry count

	Error     string      `json:"error,omitempty" readOnly:"true"`
	Result    string      `json:"result,omitempty" readOnly:"true"`
	Params    string      `json:"params,omitempty"` // job parameters as a JSON string
	ParamsPtr interface{} `json:"-" gorm:"-"`       // job parameters as a struct

	//	Result interface{} `json:"result,omitempty" gorm:"type:json;serializer:json" readOnly:"true"`
	//	Params interface{} `json:"params,omitempty" gorm:"type:json;serializer:json"`
}

// JobObj has unexported fields and getter/setter APIs
// It is used for internal job operations and object safety
type JobObj struct {
	mu utils.DebugMutex
	// goroutineID is the ID of the goroutine processing this job
	// It's used for tracking and debugging purposes
	goroutineID int64
	cancel      context.CancelFunc
	worker      JobWorker
	private     Job // private job data, not visible to the interfaces using JobObj
}

type JobWorker any

var jobTransitionMutex utils.DebugMutex
var jobWorkerMutex utils.DebugMutex

func (j *JobObj) GetJobID() uuid.UUID {
	return j.private.JobID
}

func (j *JobObj) GetTenantID() uuid.UUID {
	return j.private.TenantID
}

func (j *JobObj) GetUserID() uuid.UUID {
	return j.private.UserID
}

func (j *JobObj) GetIdempotencyKey() uuid.UUID {
	return j.private.IdempotencyKey
}

func (j *JobObj) GetQueue() JobQueueName {
	return j.GetTypePtr().GroupPtr.Queue
}

func (j *JobObj) GetType() string {
	return j.private.Type
}

func (j *JobObj) GetTypePtr() *JobType {
	if j.private.TypePtr == nil {
		logging.Error("internal error: job type is not initialized!")
	}
	return j.private.TypePtr
}

func (j *JobObj) GetTimeoutSec() time.Duration {
	return time.Duration(j.private.TimeoutSec) * time.Second
}

func (j *JobObj) GetMaxRetries() int {
	return j.private.MaxRetries
}

func (j *JobObj) GetLockedBy() uuid.UUID {
	j.mu.Lock()
	defer j.mu.Unlock()

	// FIXME: the status read and update can happen in parallel go routines, need a way to sync status and other fields w/o DB

	// If we're in a transaction or the job might have been updated by another process,
	// refresh the LockedBy from the database to ensure we have the latest value
	var jobDB Job
	result := db.DB.Model(&Job{}).Where("tenant_id = ? AND job_id = ?", j.private.TenantID, j.private.JobID).Select("locked_by").First(&jobDB)
	if result.Error == nil {
		j.private.LockedBy = jobDB.LockedBy
	} else {
		// Log the error but don't fail - return the current LockedBy
		j.LogDebug("Failed to refresh job locked_by from database: %v", result.Error)
	}

	return j.private.LockedBy
}

func (j *JobObj) GetRetryDelay() time.Duration {
	return time.Duration(j.private.RetryDelaySec) * time.Second
}

func (j *JobObj) GetParams() string {
	return j.private.Params
}

func (j *JobObj) GetParamsPtr() interface{} {
	return j.private.ParamsPtr
}

func (j *JobObj) getWorker(_ context.Context) interface{} {
	return j.worker
}

func (j *JobObj) GetStatus() JobStatus {
	// If we're in a transaction or the job might have been updated by another process,
	// refresh the status from the database to ensure we have the latest value
	var jobDB Job
	result := db.DB.Model(&Job{}).Where("tenant_id = ? AND user_id = ? AND job_id = ?", j.private.TenantID, j.private.UserID, j.private.JobID).Select("status").First(&jobDB)
	if result.Error == nil {
		j.private.Status = jobDB.Status
	} else {
		// Log the error but don't fail - return the current status
		j.LogDebug("Failed to refresh job status from database: %v", result.Error)
	}

	return j.private.Status
}

func (j *JobObj) GetStatusErrorProgressSuccess() (string, string, float32, bool) {
	// If we're in a transaction or the job might have been updated by another process,
	// refresh the status from the database to ensure we have the latest value
	var jobDB Job
	result := db.DB.Model(&Job{}).Where("tenant_id = ? AND job_id = ?", j.private.TenantID, j.private.JobID).Select("status, error, progress").First(&jobDB)
	if result.Error == nil {
		j.private.Status = jobDB.Status
		j.private.Error = jobDB.Error
		j.private.Progress = jobDB.Progress
	} else {
		// Log the error but don't fail - return the current status
		j.LogDebug("Failed to refresh job status from database: %v", result.Error)
	}

	var success bool
	if j.private.Status == JobStatusCompleted {
		success = true
	} else if j.private.Status == JobStatusFailed || j.private.Status == JobStatusCanceled || j.private.Status == JobStatusTimedOut {
		success = false
	}

	return string(j.private.Status), j.private.Error, j.private.Progress, success
}

func (j *JobObj) GetStartedAt() int64 {
	return j.private.StartedAtMs
}

func (j *JobObj) GetScheduledAt() int64 {
	return j.private.ScheduledAtMs
}

func (j *JobObj) GetProgress() float32 {
	return j.private.Progress
}

func (j *JobObj) GetRetries() int {
	return j.private.Retries
}

func (j *JobObj) GetETA() int64 {
	return j.private.ETAMs
}

func (j *JobObj) GetError() string {
	return j.private.Error
}

func (j *JobObj) setStatus(ctx context.Context, status JobStatus, statusErr error) error {
	maxRetries := 3
	backoff := 10 * time.Millisecond

	var lastDbErr error

	// Retry loop
	for attempt := 0; attempt < maxRetries; attempt++ {
		jobTransitionMutex.Lock()

		needToUpdateDb, transitionErr := j.CheckStatusTransition(status)
		lastDbErr = nil

		if needToUpdateDb {
			j.private.Status = status
			if statusErr == nil {
				lastDbErr = j.dbSaveFields(&j.private.Status)
			} else {
				j.private.Error = statusErr.Error()
				lastDbErr = j.dbSaveFields(&j.private.Status, &j.private.Error)
			}
		}

		jobTransitionMutex.Unlock()

		if transitionErr != nil {
			j.LogError("failed to transition job status: %s", transitionErr.Error())
			return transitionErr
		}

		if lastDbErr == nil || !shouldRetryError(lastDbErr) {
			// Call the status update callback if it exists
			if needToUpdateDb && j.GetTypePtr() != nil && j.GetTypePtr().WorkerStateUpdateCallback != nil {
				err := j.GetTypePtr().WorkerStateUpdateCallback(ctx, j)
				if err != nil {
					err = fmt.Errorf("failed to update job status: %w", err)
					j.LogError(err.Error())
					return err
				}
			}
			return lastDbErr
		}

		j.LogDebug("job %s: retrying status update to '%s' due to error: %s (attempt %d/%d)", j.private.JobID.String(), status, lastDbErr.Error(), attempt+1, maxRetries)

		time.Sleep(backoff)
		backoff *= 2
	}

	msg := fmt.Errorf("job %s: failed to set status to '%s' after %d attempts: %w", j.private.JobID.String(), status, maxRetries, lastDbErr)
	j.LogError("%s", msg.Error())
	return msg
}

func (j *JobObj) statusIsFinal() bool {
	return j.private.Status == JobStatusSkipped ||
		j.private.Status == JobStatusCanceled ||
		j.private.Status == JobStatusFailed ||
		j.private.Status == JobStatusTimedOut ||
		j.private.Status == JobStatusCompleted
}

// shouldRetryError determines if an error should trigger a retry
func shouldRetryError(err error) bool {
	// Check for database locking errors
	return err != nil && strings.Contains(err.Error(), "database table is locked")
}

// CheckStatusTransition implements safe job transition checks for concurrent
// job operations. Returns true/false if we need to update status and error in case of false
func (j *JobObj) CheckStatusTransition(desiredStatus JobStatus) (bool, error) {
	var dbStatus JobStatus

	var job Job

	var err error
	retryTimeout := time.Now().Add(5 * time.Second)

	for {
		err = db.DB.Model(&Job{}).
			Where("job_id = ? AND tenant_id = ? AND user_id = ?", j.private.JobID, j.private.TenantID, j.private.UserID).
			Select("status").
			First(&job).Error

		if err == nil || !db.DatabaseIsLocked(err) || time.Now().After(retryTimeout) {
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dbStatus = JobStatusInit
		} else {
			return false, fmt.Errorf("failed to get job status: %w", err)
		}
	} else {
		dbStatus = job.Status
	}

	if desiredStatus == dbStatus {
		// If the desired status is the same as the current status, we can return the current status
		return false, nil
	}

	if dbStatus == JobStatusCanceled && desiredStatus == JobStatusCanceled {
		// If the job is canceled, we can only transition to canceled
		return false, nil
	}

	errMsg := fmt.Errorf("job transition from '%s' to '%s' is not allowed", dbStatus, desiredStatus)

	switch desiredStatus {
	case JobStatusInit:
		if dbStatus != JobStatusInit {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusWaiting:
		if dbStatus != JobStatusInit && dbStatus != JobStatusLocked && dbStatus != JobStatusResuming {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusResuming:
		if dbStatus == JobStatusInit || dbStatus == JobStatusWaiting || dbStatus == JobStatusRunning {
			// Job is already in a terminal state, resuming can be just ignored
		} else if dbStatus != JobStatusSuspended {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusRunning:
		if dbStatus != JobStatusWaiting && dbStatus != JobStatusLocked {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusCanceling:
		if dbStatus == JobStatusSkipped || dbStatus == JobStatusCanceled || dbStatus == JobStatusFailed || dbStatus == JobStatusTimedOut || dbStatus == JobStatusCompleted {
			// Job is already in a terminal state, canceling can be just ignored
			return false, nil
		}
	case JobStatusSuspending:
		if dbStatus == JobStatusSkipped || dbStatus == JobStatusCanceled || dbStatus == JobStatusFailed || dbStatus == JobStatusTimedOut || dbStatus == JobStatusCompleted {
			// Job is already in a terminal state, suspending can be just ignored
			return false, nil
		}
	case JobStatusLocked:
		if dbStatus != JobStatusRunning {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusSuspended:
		// Only initializing, waiting, running, and suspended jobs can be suspended
		if dbStatus != JobStatusSuspending {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusSkipped:
		if dbStatus != JobStatusRunning {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusCanceled:
		if dbStatus != JobStatusCanceling {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusFailed:
		if dbStatus != JobStatusRunning && dbStatus != JobStatusSuspending && dbStatus != JobStatusResuming {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusTimedOut:
		if dbStatus != JobStatusRunning {
			j.dumpStack()
			return false, errMsg
		}
	case JobStatusCompleted:
		if dbStatus != JobStatusRunning {
			j.dumpStack()
			return false, errMsg
		}
	default:
		j.dumpStack()
		return false, fmt.Errorf("invalid job status: %s", dbStatus)
	}

	j.LogDebug("state transition: %s -> %s", dbStatus, desiredStatus)

	return true, nil
}

func (j *JobObj) dumpStack() {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("Stack trace:\n")
	buf.WriteString(string(debug.Stack()))
	j.LogError("%s", buf.String())
}

func (j *JobObj) setWorker(worker JobWorker) {
	j.worker = worker
}

func (j *JobObj) SetRetryPolicy(retryDelay time.Duration, maxRetries int, timeout time.Duration) error {
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

	j.private.RetryDelaySec = int(retryDelay.Seconds())
	j.private.MaxRetries = maxRetries
	j.private.TimeoutSec = int(timeout.Seconds())

	return j.dbSaveFields(&j.private.RetryDelaySec, &j.private.TimeoutSec, &j.private.MaxRetries)
}

func (j *JobObj) initType(typeStr string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	var ok bool

	j.private.Type = typeStr
	if typeStr == "" {
		return fmt.Errorf("job type is not set for job %s", j.private.JobID.String())
	}

	j.private.TypePtr, ok = jobTypesMap[typeStr]
	if !ok {
		return fmt.Errorf("failed to get job type: %s", typeStr)
	}

	return nil
}

// initParams initializes job parameters by merging provided parameters with defaults.
// If paramsStr is empty, it uses the default parameters from the job type.
// If paramsStr is provided, it validates the JSON, applies defaults for any missing fields,
// and then unmarshals the merged result into the job's parameters.
func (j *JobObj) initParams(paramsStr string) error {
	jobType := j.GetTypePtr()
	if jobType == nil {
		return fmt.Errorf("job type is not set for job %s", j.private.JobID.String())
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if jobType.Params == nil {
		if paramsStr != "" {
			return fmt.Errorf("job type '%s' doesn't support parameters", jobType.TypeID)
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
		return err
	}

	j.private.Params = mergedJSON
	j.private.ParamsPtr = paramsObj
	return nil
}

func (j *JobObj) setParams(paramsStr string) error {
	if err := j.initParams(paramsStr); err != nil {
		return err
	}
	return j.dbSaveFields(&j.private.Params)
}

func (j *JobObj) SetResult(ctx context.Context, result interface{}) error {
	var err error
	j.private.Result, err = utils.StructToJSONString(result)
	if err != nil {
		j.LogError("failed to marshal job result: %s", err.Error())
		return err
	}
	return j.dbSaveFields(&j.private.Result)
}

func NewJob(
	ctx context.Context,
	idempotencyKey uuid.UUID,
	jobType *JobType,
	paramsStr string,
) (*JobObj, error) {
	if jobType == nil {
		return nil, fmt.Errorf("job type is not set")
	}

	if _, ok := jobTypesMap[jobType.TypeID]; !ok {
		panic(fmt.Sprintf("job type '%s' must be registered before creating a job", jobType.Name))
	}

	j := &JobObj{
		private: Job{
			TenantID:       auth.GetTenantID(),
			UserID:         auth.GetUserID(),
			JobID:          uuid.New(),
			IdempotencyKey: idempotencyKey,
			Status:         JobStatusInit,
			LockedBy:       uuid.Nil,
			UpdatedAtMs:    time.Now().UnixMilli(),
			Progress:       0,
			Retries:        0,
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

	if db.DB == nil {
		return nil, fmt.Errorf("database is not initialized")
	}

	if jobType.WorkerInitCallback != nil {
		jobWorkerMutex.Lock()
		defer jobWorkerMutex.Unlock()

		worker, err := jobType.WorkerInitCallback(ctx, j)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize job '%s': %s", jobType.TypeID, err.Error())
		}
		j.setWorker(worker)
	}

	err := db.DB.Create(&j.private).Error
	if err == nil {
		return j, nil
	}

	msg := fmt.Sprintf("failed to create job in DB: %s", err.Error())
	j.LogError(msg)
	return nil, fmt.Errorf("%s", msg)
}

func (j *JobObj) dbSaveFields(fields ...interface{}) error {
	j.private.UpdatedAtMs = time.Now().UnixMilli()
	fields = append(fields, &j.private.UpdatedAtMs)
	// Use job_id as primary key field
	pkFields := map[string]interface{}{
		"job_id":    j.private.JobID,
		"tenant_id": auth.GetTenantID(),
	}

	err := orm.OrmUpdateObjFields(&j.private, pkFields, fields...)
	if err != nil {
		msg := fmt.Sprintf("failed to update job: %s", err.Error())
		j.LogError(msg)
		j.private.Error = msg
		return fmt.Errorf("%s", msg)
	}

	return nil
}

func (j *JobObj) schedule(ctx context.Context) error {
	now := time.Now().UTC()

	// Convert params to JSON string for logging
	paramsJSON, _ := json.MarshalIndent(j.private.ParamsPtr, "", "  ")
	j.LogInfo("scheduling with params: %s", string(paramsJSON))

	j.setStatus(ctx, JobStatusWaiting, nil)
	j.private.ScheduledAtMs = now.UnixMilli()
	j.private.ETAMs = now.Add(time.Duration(j.private.TimeoutSec) * time.Second).UnixMilli()

	return j.dbSaveFields(&j.private.Status, &j.private.ScheduledAtMs, &j.private.ETAMs)
}

func (j *JobObj) SetLockedBy(ctx context.Context, lockedBy uuid.UUID) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.setStatus(ctx, JobStatusLocked, nil)
	j.private.LockedBy = lockedBy
	j.private.UpdatedAtMs = time.Now().UnixMilli()

	if lockedBy == j.private.JobID {
		return fmt.Errorf("trying to lock job by itself")
	}

	// Recursively check if there's a circular lock dependency
	visited := make(map[uuid.UUID]bool)
	visited[j.private.JobID] = true

	var checkLockChain func(ctx context.Context, currentLockedBy uuid.UUID) error
	checkLockChain = func(ctx context.Context, currentLockedBy uuid.UUID) error {
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
		lockingJob, err := getJob(ctx, currentLockedBy)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // Job not found, end of chain
			}
			return fmt.Errorf("failed to check locking job: %w", err)
		}

		// Recursively check the job that locked this one
		return checkLockChain(ctx, lockingJob.private.LockedBy)
	}

	if err := checkLockChain(context.Background(), lockedBy); err != nil {
		return err
	}

	j.LogDebug("locked by job %s", lockedBy.String())

	return j.dbSaveFields(&j.private.Status, &j.private.LockedBy, &j.private.UpdatedAtMs)
}

func (j *JobObj) SetUnlocked(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.private.Status != JobStatusCanceling && j.private.Status != JobStatusCanceled {
		j.setStatus(ctx, JobStatusRunning, nil)
	}

	j.private.LockedBy = uuid.Nil
	j.private.UpdatedAtMs = time.Now().UnixMilli()
	j.LogDebug("unlocked")

	return j.dbSaveFields(&j.private.Status, &j.private.LockedBy, &j.private.UpdatedAtMs)
}

func (j *JobObj) setRunning(ctx context.Context) error {
	now := time.Now().UTC()

	err := j.setStatus(ctx, JobStatusRunning, nil)
	if err != nil {
		return err
	}

	j.private.StartedAtMs = now.UnixMilli()
	j.LogDebug("running ...")

	return j.dbSaveFields(&j.private.StartedAtMs)
}

func (j *JobObj) SetSkipped(ctx context.Context, reason string) error {
	j.LogInfo("skipped")

	err := j.setStatus(ctx, JobStatusSkipped, errors.New(reason))
	if err != nil {
		return err
	}

	return j.SetProgress(ctx, 100)
}

func (j *JobObj) SetProgress(ctx context.Context, progress float32) error {
	if progress < 0 {
		return fmt.Errorf("invalid progress value: %.1f", progress)
	}

	if progress > 100 {
		// Sometimes timer-based progress can report value > 100
		progress = 100
	}

	if progress == j.private.Progress {
		return nil
	}

	j.private.Progress = progress

	if progress < 100 && time.Now().UnixMilli()-j.private.UpdatedAtMs < 1 {
		return nil
	}

	j.logProgress(progress)

	err := j.dbSaveFields(&j.private.Progress)
	if err != nil {
		j.LogError("failed to update job progress: %s", err.Error())
		return err
	}

	if j.GetTypePtr().WorkerStateUpdateCallback != nil {
		err := j.GetTypePtr().WorkerStateUpdateCallback(ctx, j)
		if err != nil {
			j.LogError("failed to update job status: %s", err.Error())
			return err
		}
	}

	return nil
}

// This is internal method called by JobExecutor to initiate job cancellation
// Job workers must use setCancelled
func (j *JobObj) setCanceling(ctx context.Context, reason error) error {
	if reason != nil {
		j.LogDebug("canceling with reason: %s ...", reason.Error())
	} else {
		j.LogDebug("canceling ...")
	}

	return j.setStatus(ctx, JobStatusCanceling, reason)
}

func (j *JobObj) setCancelled(ctx context.Context, reason error) error {
	reasonMsg := ""
	if reason != nil {
		reasonMsg = reason.Error()
	}

	j.LogInfo("canceled with reason: %s", reasonMsg)

	if j.private.Error != "" {
		reason = nil
	}

	return j.setStatus(ctx, JobStatusCanceled, reason)
}

func (j *JobObj) setTimedOut(ctx context.Context, err error) error {
	j.LogInfo("timed out")

	return j.setStatus(ctx, JobStatusTimedOut, err)
}

func (j *JobObj) setCompleted(ctx context.Context, msg string) error {
	if msg == "" {
		msg = "completed successfully"
	}

	err := j.setStatus(ctx, JobStatusCompleted, nil)
	if err != nil {
		return err
	}

	j.private.Progress = 100
	j.private.Details = msg
	j.LogInfo(msg)

	return j.dbSaveFields(&j.private.Status, &j.private.Progress, &j.private.Details)
}

func (j *JobObj) setFailed(ctx context.Context, reason error) error {
	status := j.GetStatus()

	// Do not override cancalleation statuses

	if status == JobStatusCanceled {
		j.LogTrace("job is canceled, skipping error: %s", reason.Error())
		return nil
	}
	if status == JobStatusCanceling {
		j.LogDebug("job is canceling, skipping error: %s", reason.Error())
		return nil
	}
	if status == JobStatusSuspending {
		j.LogTrace("job is suspended, skipping error: %s", reason.Error())
		return nil
	}

	// Do not override timeout statuses

	if j.private.TimeoutSec > 0 && j.private.StartedAtMs > 0 && time.Since(time.UnixMilli(j.private.StartedAtMs)) > time.Duration(j.private.TimeoutSec)*time.Second {
		// It seems like the error was caused by timeout
		j.LogTrace("job timed out, skipping error: %s", reason.Error())
		return j.setTimedOut(ctx, reason)
	}

	// OK, seems like it's real failure

	j.LogError("failed with reason: %s", reason.Error())

	return j.setStatus(ctx, JobStatusFailed, reason)
}

func (j *JobObj) setRetrying(ctx context.Context, reason error) error {
	j.private.Retries++
	j.private.ScheduledAtMs = time.Now().UTC().Add(time.Duration(j.private.RetryDelaySec) * time.Second).UnixMilli()

	j.LogDebug("retried")
	j.setStatus(ctx, JobStatusWaiting, fmt.Errorf("Attempt %d/%d: %v", j.private.Retries, j.GetTypePtr().MaxRetries, reason.Error()))

	return j.dbSaveFields(&j.private.Status, &j.private.ScheduledAtMs, &j.private.Error)
}

// setSuspending sets the job status to 'suspending'
func (j *JobObj) setSuspending(ctx context.Context, reason error) error {
	if reason != nil {
		j.LogDebug("suspending with reason: %s ...", reason.Error())
	} else {
		j.LogDebug("suspending ...")
	}

	if !j.GetTypePtr().WorkerIsSuspendable {
		return fmt.Errorf("job of type %s doesn't support suspend operation", j.GetTypePtr().TypeID)
	}

	// Now check the standard transition rules
	needToUpdateDB, err := j.CheckStatusTransition(JobStatusSuspending)
	if !needToUpdateDB {
		return err
	}

	j.LogInfo(StatusSuspending)
	err = j.setStatus(ctx, JobStatusSuspending, reason)
	if err != nil {
		return err
	}

	// Call the suspend callback if defined
	if j.GetTypePtr().WorkerSuspendCallback != nil {
		callbackErr := j.GetTypePtr().WorkerSuspendCallback(ctx, j)
		if callbackErr != nil {
			msg := fmt.Errorf("suspend callback failed: %s", callbackErr.Error())

			j.LogError(msg.Error())
			j.setFailed(ctx, msg)

			return callbackErr
		}
	}

	return j.dbSaveFields(&j.private.Status)
}

// SetSuspended sets the job status to suspended
func (j *JobObj) setSuspended(ctx context.Context, reason error) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.LogInfo(StatusSuspended)
	err := j.setStatus(ctx, JobStatusSuspended, reason)
	if err != nil {
		return err
	}

	return j.dbSaveFields(&j.private.Status)
}

// SetResumed sets the job status back to waiting
func (j *JobObj) setResumed(ctx context.Context) error {
	typePtr := j.GetTypePtr()

	if !typePtr.WorkerIsSuspendable {
		return fmt.Errorf("job of type %s doesn't support resume operation", typePtr.TypeID)
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	j.LogInfo("resuming")
	err := j.setStatus(ctx, JobStatusResuming, nil)
	if err != nil {
		return err
	}

	// Call the resume callback if defined
	if typePtr.WorkerResumeCallback != nil {
		worker, callbackErr := typePtr.WorkerResumeCallback(ctx, j)
		if callbackErr != nil {
			msg := fmt.Errorf("resume callback failed: %s", callbackErr.Error())
			j.LogError(msg.Error())
			j.setFailed(ctx, msg)
			return callbackErr
		}
		j.setWorker(worker)
	}

	j.LogInfo("resumed")
	err = j.setStatus(ctx, JobStatusWaiting, nil)
	if err != nil {
		return err
	}

	// Clear the error on resume
	j.private.Error = ""

	return j.dbSaveFields(&j.private.Status, &j.private.Error)
}

// JobWorkerStateUpdateCallback is a callback function that updates target worker properties,
// such as Progress, Status, Error and Success
type JobWorkerStateUpdateCallback func(ctx context.Context, job *JobObj) error

// JobWorkerInitCallback is a callback function for job paramemeter initialisation.
// The function returns target worker object associated with current job and an error if any.
// It's being called from the synchronous API handler creating the job, so if error
// is returned, the job creation will fail
type JobWorkerInitCallback func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error)

// JobWorkerExecutionCallback is a callback function for target worker execution.
// It run asynchronously in a dedicated go-routine, if error is returned then
// the job will be marked as failed. Otherwise it will be marked as completed successfully.
// This callback is called when the job is first started and again after resume.
// The progress channel is used to report progress updates to the job executor.
// On resume, the worker is responsible for setting the job progress to the last reported value.
type JobWorkerExecutionCallback func(ctx context.Context, worker JobWorker, progress chan<- float32) error

// A callback function for job suspension, called from syncrhonous API handler. If error is
// returned the suspend operation will fail.
type JobWorkerSuspendCallback func(ctx context.Context, job *JobObj) errorx.Error

// A callback function for job resume after suspension, called from syncrhonous API handler
// If error is returned the resume operation will fail.
type JobWorkerResumeCallback func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error)

type JobTypeParams struct {
	Group                     *JobGroup
	Name                      string
	Description               string
	Params                    interface{}
	WorkerInitCallback        JobWorkerInitCallback        // Initial job initialisation callback that is called on job start or resume
	WorkerExecutionCallback   JobWorkerExecutionCallback   // main worker callback that is called on job start or after resume
	WorkerStateUpdateCallback JobWorkerStateUpdateCallback // optional callback for linked objects state updates
	WorkerSuspendCallback     JobWorkerSuspendCallback     // optional callback called before job suspend
	WorkerResumeCallback      JobWorkerResumeCallback      // optional callback called after job resume, but before InitCallback
	WorkerIsSuspendable       bool                         // if true, the job can be suspended and resumed
	Timeout                   time.Duration
	RetryDelay                time.Duration
	MaxRetries                int
}

// jobTypes holds worker functions for each job type.
var jobTypes = []*JobType{}
var jobGroups = []*JobGroup{}
var jobTypesMap = map[string]*JobType{}
var jobGroupsMap = map[string]*JobGroup{}

func RegisterJobGroup(group *JobGroup) {
	if _, ok := jobGroupsMap[group.Name]; ok {
		return
	}

	if group.Queue == "" {
		panic("job group queue is not set")
	}

	_, err := GetJobQueue(group.Queue)
	if err != nil {
		panic(fmt.Sprintf("the job queue '%s' is not registered: %s", group.Queue, err.Error()))
	}

	jobGroupsMap[group.Name] = group
	jobGroups = append(jobGroups, group)
}

func RegisterJobType(params JobTypeParams) *JobType {
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
		WorkerInitCallback:        params.WorkerInitCallback,
		WorkerExecutionCallback:   params.WorkerExecutionCallback,
		WorkerStateUpdateCallback: params.WorkerStateUpdateCallback,
		WorkerSuspendCallback:     params.WorkerSuspendCallback,
		WorkerResumeCallback:      params.WorkerResumeCallback,
		WorkerIsSuspendable:       params.WorkerIsSuspendable,
	}

	jobTypesMap[jt.TypeID] = jt
	jobTypes = append(jobTypes, jt)

	return jt
}

func GetJobType(type_id string) (*JobType, bool) {
	jt, ok := jobTypesMap[type_id]
	return jt, ok
}

func GetJobTypes(ctx context.Context, pageRequest *api.PageAPIRequest) []*JobType {
	return api.PageAPIPaginate(jobTypes, pageRequest)
}

func GetJobGroup(name string) (*JobGroup, bool) {
	group, ok := jobGroupsMap[name]
	return group, ok
}

func GetJobGroups(ctx context.Context, pageRequest *api.PageAPIRequest) []*JobGroup {
	return api.PageAPIPaginate(jobGroups, pageRequest)
}

func ListJobs(ctx context.Context, pageRequest *api.PageAPIRequest, status string) ([]*JobObj, error) {
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
		jobObj := &JobObj{private: *job}
		jobObj.initType(job.Type)
		jobObj.initParams(job.Params)
		jobsObj = append(jobsObj, jobObj)
	}

	return jobsObj, nil
}

func getJob(_ context.Context, jobID uuid.UUID) (*JobObj, errorx.Error) {
	var jobData Job
	if err := db.DB.Where("tenant_id = ? AND user_id = ? AND job_id = ?", auth.GetTenantID(), auth.GetUserID(), jobID).First(&jobData).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errorx.NewErrNotFound("job not found")
		}
		return nil, errorx.NewErrInternalServerError(err.Error())
	}

	job := &JobObj{private: jobData}
	job.initType(jobData.Type)
	job.initParams(jobData.Params)
	return job, nil
}

func init() {
	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			&Job{},
		},
		InitAPIRoutes: registerJobAPIRoutes,
		Name:          "jobs",
		InitMain:      initJobQueues,
	})
}
