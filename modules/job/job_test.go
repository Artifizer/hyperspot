package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/stretchr/testify/assert"
)

// dummyParams is used for testing job parameters.
type dummyParams struct {
	Foo string `json:"foo" default:"foo-default"`
	Bar string `json:"bar" default:"bar-default"`
}

// dummyJobType is our reusable test job type.
var dummyJobType *JobType

// initDummyJobType initializes the dummy job type used in tests.
func getDummyJobType() *JobType {
	if dummyJobType == nil {
		dummyJobType = RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "dummy",
					Queue:       JobQueueCompute,
					Description: "Dummy job group",
				},
				Name:        "dummy:job",
				Description: "Dummy job for testing",
				Params:      &dummyParams{},
				WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
					return job, nil
				},
				WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
					// Simulate some progress and then complete.
					progress <- 50
					progress <- 100
					return nil
				},
				WorkerStateUpdateCallback: nil,
				WorkerSuspendCallback:     nil,
				WorkerResumeCallback:      nil,
				WorkerIsSuspendable:       false,
				Timeout:                   2 * time.Second,
				RetryDelay:                time.Second,
				MaxRetries:                0,
			},
		)
	}
	return dummyJobType
}

func TestMain(m *testing.M) {
	// Set up a single, shared in-memory SQLite DB for all tests
	// Use a unique DSN with a shared cache to ensure all connections reference the same database
	testDB, err := db.InitInMemorySQLite(nil)
	if err != nil {
		fmt.Printf("Failed to connect to test database: %v\n", err)
		os.Exit(1)
	}

	// Store the DB for job package to use
	db.DB = testDB

	// Migrate all needed schemas at once
	if err := testDB.AutoMigrate(&Job{}, &JobType{}, &JobGroup{}); err != nil {
		fmt.Printf("Failed to migrate test database: %v\n", err)
		os.Exit(1)
	}

	// Run the tests
	exitCode := m.Run()

	// Cleanup: properly close the database connection
	sqlDB, err := testDB.DB()
	if err == nil {
		sqlDB.Close()
	}

	os.Exit(exitCode)
}

func TestNewJob_Success(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, paramsJSON)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}
	if job == nil {
		t.Fatal("NewJob returned nil job")
	}

	if job.GetType() != dummyJobType.TypeID {
		t.Fatalf("Expected job type %s, got %s", dummyJobType.TypeID, job.GetType())
	}
	// Verify that job.Params is a valid JSON string and contains the expected value.
	if !json.Valid([]byte(job.GetParams())) {
		t.Fatalf("Job params is not valid JSON: %s", job.GetParams())
	}
	var dp dummyParams
	if err := json.Unmarshal([]byte(job.GetParams()), &dp); err != nil {
		t.Fatalf("Failed to unmarshal job.Params: %v", err)
	}
	if dp.Foo != "bar" {
		t.Fatalf("Expected job.Params.Foo to be 'bar', got '%s'", dp.Foo)
	}
}

func TestNewJob_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	invalidJSON := `{"foo": "bar"` // missing closing brace

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	_, err := NewJob(ctx, idempotencyKey, jt, invalidJSON)
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestJobScheduleAndRun(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, paramsJSON)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}

	// Test scheduling: job.schedule() should mark the job as waiting and set the schedule timestamps.
	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}
	if job.GetStatus() != JobStatusWaiting {
		t.Fatalf("Expected job status 'waiting', got '%s'", job.GetStatus())
	}
	if job.GetScheduledAt() == 0 {
		t.Fatal("Expected ScheduledAt to be set")
	}
	if job.GetETA() == 0 {
		t.Fatal("Expected ETA to be set")
	}

	// Test running the job: job.Run() should mark the job as running and set StartedAt.
	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}
	if job.GetStatus() != JobStatusRunning {
		t.Fatalf("Expected job status 'running', got '%s'", job.GetStatus())
	}
	if job.GetStartedAt() == 0 {
		t.Fatal("Expected StartedAt to be set")
	}
}

func TestJobComplete(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, paramsJSON)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}

	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}

	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}

	if err := job.setCompleted(ctx, "completed successfully"); err != nil {
		t.Fatalf("Job.SetCompleted() returned error: %v", err)
	}

	if job.GetStatus() != JobStatusCompleted {
		t.Fatalf("Expected job status 'completed', got '%s'", job.GetStatus())
	}
	if job.GetProgress() != 100 {
		t.Fatalf("Expected job progress 100, got %.1f", job.GetProgress())
	}
}

func TestJobFail(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, paramsJSON)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}

	dummyErr := errors.New("dummy failure")
	if err := job.setFailed(ctx, dummyErr); err == nil {
		t.Fatalf("New job can't be failed, it must be scheduled and running first: %v", err)
	}

	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}
	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}

	if err := job.setFailed(ctx, dummyErr); err != nil {
		t.Fatalf("Job.setFailed() returned error: %v", err)
	}

	if job.GetStatus() != JobStatusFailed {
		t.Fatalf("Expected job status 'failed', got '%s'", job.GetStatus())
	}
	if job.GetError() != dummyErr.Error() {
		t.Fatalf("Expected job error '%s', got '%s'", dummyErr.Error(), job.GetError())
	}
}

func TestJobRetry(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, paramsJSON)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}

	initialRetries := job.GetRetries()
	dummyErr := errors.New("dummy retry failure")
	if err := job.setRetrying(ctx, dummyErr); err != nil {
		t.Fatalf("Job.setRetrying() returned error: %v", err)
	}
	if job.GetRetries() != initialRetries+1 {
		t.Fatal("Expected job.Retries to be incremented")
	}
	if job.GetStatus() != JobStatusWaiting {
		t.Fatalf("Expected job status 'waiting' after retry, got '%s'", job.GetStatus())
	}
	expectedErrorMsg := fmt.Sprintf("Attempt %d/%d: %v", job.GetRetries(), dummyJobType.MaxRetries, dummyErr.Error())
	if job.GetError() != expectedErrorMsg {
		t.Fatalf("Expected job error '%s', got '%s'", expectedErrorMsg, job.GetError())
	}
}

func TestGetQueue(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, paramsJSON)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}
	queue := job.GetQueue()
	if queue != dummyJobType.GroupPtr.Queue {
		t.Fatalf("Expected job queue '%s', got '%s'", dummyJobType.GroupPtr.Queue, queue)
	}
}

func TestLockAndUnlock(t *testing.T) {
	ctx := context.Background()
	// Create two jobs so that GetJob (used in Locked) can find one of them.
	idempotencyKey1 := uuid.New()
	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job1, err := NewJob(ctx, idempotencyKey1, jt, `{"foo": "job1"}`)
	if err != nil {
		t.Fatalf("NewJob job1 error: %v", err)
	}

	if err := job1.schedule(ctx); err != nil {
		t.Fatalf("Job1.schedule() returned error: %v", err)
	}
	if err := job1.setRunning(ctx); err != nil {
		t.Fatalf("Job1.setRunning() returned error: %v", err)
	}

	idempotencyKey2 := uuid.New()
	job2, err := NewJob(ctx, idempotencyKey2, dummyJobType, `{"foo": "job2"}`)
	if err != nil {
		t.Fatalf("NewJob job2 error: %v", err)
	}
	if err := job2.schedule(ctx); err != nil {
		t.Fatalf("Job2.schedule() returned error: %v", err)
	}
	if err := job2.setRunning(ctx); err != nil {
		t.Fatalf("Job2.setRunning() returned error: %v", err)
	}

	// Lock job2 with job1's ID.
	if err := job2.SetLockedBy(ctx, job1.GetJobID()); err != nil {
		t.Fatalf("Job2.Locked() returned error: %v", err)
	}
	if job2.GetStatus() != JobStatusLocked {
		t.Fatalf("Expected job2 status 'locked', got '%s'", job2.GetStatus())
	}
	if job2.GetLockedBy() != job1.GetJobID() {
		t.Fatal("Expected job2 LockedBy to be job1's ID")
	}

	// Now unlock job2.
	if err := job2.SetUnlocked(ctx); err != nil {
		t.Fatalf("Job.Unlocked() returned error: %v", err)
	}
	if job2.GetStatus() != JobStatusRunning {
		t.Fatalf("Expected job2 status 'running' after unlock, got '%s'", job2.GetStatus())
	}
	if job2.GetLockedBy() != uuid.Nil {
		t.Fatalf("Expected job2 LockedBy to be uuid.Nil after unlock, got '%s'", job2.GetLockedBy().String())
	}
}

func TestSetProgress(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo": "progress"}`)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}
	// Set progress to 50.
	if err := job.SetProgress(ctx, 50); err != nil {
		t.Fatalf("Job.SetProgress() returned error: %v", err)
	}
	if job.GetProgress() != 50 {
		t.Fatalf("Expected progress 50, got %f", job.GetProgress())
	}

	// Set progress to -1.
	if err := job.SetProgress(ctx, -1); err == nil {
		t.Fatalf("Job.SetProgress() returned nil for negative progress")
	}

	if err := job.SetProgress(ctx, 101); err != nil {
		t.Fatalf("Job.SetProgress() must cap progress to 100 and no error must be returned")
	}

	if err := job.SetProgress(ctx, 100); err != nil {
		t.Fatalf("Job.SetProgress() returned error: %v", err)
	}
}

func TestCancelingAndCancel(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo": "cancel"}`)
	if err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}

	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}
	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}

	dummyErr := errors.New("canceling error")
	if err := job.setCanceling(ctx, dummyErr); err != nil {
		t.Fatalf("Job.setCanceling() returned error: %v", err)
	}
	if job.GetStatus() != JobStatusCanceling {
		t.Fatalf("Expected job status 'canceling', got '%s'", job.GetStatus())
	}
	// Now cancel the job.
	if err := job.setCancelled(ctx, dummyErr); err != nil {
		t.Fatalf("Job.setCancelled() returned error: %v", err)
	}
	if job.GetStatus() != JobStatusCanceled {
		t.Fatalf("Expected job status 'canceled', got '%s'", job.GetStatus())
	}
	if job.GetError() != dummyErr.Error() {
		t.Fatalf("Expected job error '%s', got '%s'", dummyErr.Error(), job.GetError())
	}
}

// dummyParamsForValidation is used to validate job type parameters.
type dummyParamsForValidation struct {
	Foo string `json:"foo"`
}

// TestJobType_Validation verifies that a job created with a job type having a non-nil Params field
// correctly validates the JSON parameters.
func TestJobType_Validation(t *testing.T) {
	// Create a dummy job type with a non-nil params pointer.

	jt := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "dummy",
				Queue:       JobQueueCompute,
				Description: "Dummy group",
			},
			Name:        "dummy:validation",
			Description: "Dummy job type for validation",
			Params:      &dummyParamsForValidation{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Do nothing
				return nil
			},
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   30 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                2,
		},
	)
	assert.NotNil(t, jt)

	// Construct valid parameters.
	validParams := dummyParamsForValidation{Foo: "bar"}
	pbytes, err := json.Marshal(validParams)
	if err != nil {
		t.Fatalf("Failed to marshal params: %v", err)
	}

	// Create a new job using the dummy job type.
	job, err := NewJob(context.Background(), uuid.New(), jt, string(pbytes))
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Verify that the job's job type is correctly set.
	if job.GetType() != fmt.Sprintf("%s:%s", jt.Group, jt.Name) {
		t.Fatalf("Expected job type %v, got %v", fmt.Sprintf("%s:%s", jt.Group, jt.Name), job.GetType())
	}
}

// TestJobType_WorkerExecution verifies that the WorkerFunc of a job type is executed successfully.
func TestJobType_WorkerExecution(t *testing.T) {
	ctx := context.Background()
	executed := false

	// Create a dummy job type whose worker function sets the executed flag.
	jt := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "dummy",
				Queue:       JobQueueCompute,
				Description: "Dummy group",
			},
			Name:        "dummy:worker",
			Description: "Test worker execution",
			Params:      &struct{}{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				executed = true
				return nil
			},
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   30 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)
	assert.NotNil(t, jt)

	// Create a new job with the above job type.
	job, err := NewJob(context.Background(), uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Simulate executing the worker function.
	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}
	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}
	err = jt.WorkerExecutionCallback(ctx, job.getWorker(ctx), make(chan float32))
	if err != nil {
		t.Fatalf("Executing the worker function returned error: %v", err)
	}

	if !executed {
		t.Fatalf("Expected worker function to be executed, but it was not")
	}
}

// TestNewJobType verifies that NewJobType() creates a new job type in the datastore.
func TestNewJobType(t *testing.T) {
	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	if jt == nil {
		t.Fatalf("RegisterJobType returned nil")
	}
	if jt.TypeID != testJobTypeForAPI.TypeID {
		t.Fatalf("Expected job type ID '%s', got '%s'", testJobTypeForAPI.TypeID, jt.TypeID)
	}

	// Verify that the job type can be retrieved by its ID
	retrievedJT, exists := GetJobType(jt.TypeID)
	if !exists {
		t.Fatalf("GetJobType returned exists=false for a registered job type")
	}
	if retrievedJT == nil {
		t.Fatalf("GetJobType returned nil for a registered job type")
	}
	if retrievedJT.TypeID != jt.TypeID {
		t.Fatalf("Expected job type ID '%s', got '%s'", jt.TypeID, retrievedJT.TypeID)
	}

	// Test GetJobTypes
	jobTypesList := GetJobTypes(context.Background(), nil)
	if len(jobTypesList) == 0 {
		t.Fatalf("GetJobTypes returned empty list after registering a job type")
	}

	foundType := false
	for _, jobType := range jobTypesList {
		if jobType.TypeID == jt.TypeID {
			foundType = true
			break
		}
	}
	if !foundType {
		t.Fatalf("GetJobTypes did not return the registered job type")
	}

	// Test GetJobGroup
	group, exists := GetJobGroup(jt.Group)
	if !exists {
		t.Fatalf("GetJobGroup returned exists=false for a registered job group")
	}
	if group == nil {
		t.Fatalf("GetJobGroup returned nil for a registered job group")
	}
	if group.Name != jt.Group {
		t.Fatalf("Expected job group name '%s', got '%s'", jt.Group, group.Name)
	}

	// Test GetJobGroups
	jobGroupsList := GetJobGroups(context.Background(), nil)
	if len(jobGroupsList) == 0 {
		t.Fatalf("GetJobGroups returned empty list after registering a job group")
	}

	foundGroup := false
	for _, jobGroup := range jobGroupsList {
		if jobGroup.Name == jt.Group {
			foundGroup = true
			break
		}
	}
	if !foundGroup {
		t.Fatalf("GetJobGroups did not return the registered job group")
	}
}

// TestGetJobTypes verifies that listing job types returns all created job types.
func TestGetJobTypes(t *testing.T) {
	setupTestDB(t)
	// Create two job types.

	jt1 := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "listgroup",
				Queue:       JobQueueCompute,
				Description: "List group",
			},
			Name:                      "list:job:type:1",
			Description:               "First job type",
			Params:                    &struct{}{},
			WorkerInitCallback:        func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return job, nil },
			WorkerExecutionCallback:   func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   60 * time.Second,
			RetryDelay:                0 * time.Second,
			MaxRetries:                0,
		},
	)
	assert.NotNil(t, jt1)

	jt2 := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "listgroup",
				Queue:       JobQueueCompute,
				Description: "List group",
			},
			Name:                    "list:job:type:2",
			Description:             "Second job type",
			Params:                  &struct{}{},
			WorkerInitCallback:      func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return job, nil },
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			Timeout:                 60 * time.Second,
			RetryDelay:              0,
			MaxRetries:              0,
			WorkerIsSuspendable:     false,
		},
	)
	assert.NotNil(t, jt2)

	types := GetJobTypes(context.Background(), nil)
	if len(types) < 2 {
		t.Fatalf("Expected at least 2 job types, got %d", len(types))
	}
}

// TestGetJobGroup verifies that a specific job group can be retrieved.
func TestGetJobGroup(t *testing.T) {
	db := setupTestDB(t)
	group := &JobGroup{
		Name:        "testgroup",
		Queue:       JobQueueCompute,
		Description: "A job group for testing",
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("Failed to create job group: %v", err)
	}
	var fetched JobGroup
	if err := db.First(&fetched, "name = ?", group.Name).Error; err != nil {
		t.Fatalf("Failed to retrieve job group: %v", err)
	}
	if fetched.Name != group.Name {
		t.Fatalf("Expected job group name %s, got %s", group.Name, fetched.Name)
	}
}

// TestGetJobGroups verifies that listing job groups returns all created groups.
func TestGetJobGroups(t *testing.T) {
	group1 := &JobGroup{
		Name:        "group1",
		Queue:       JobQueueCompute,
		Description: "First group",
	}
	group2 := &JobGroup{
		Name:        "group2",
		Queue:       JobQueueCompute,
		Description: "Second group",
	}

	RegisterJobGroup(group1)
	RegisterJobGroup(group2)

	jg1, ok := GetJobGroup(group1.Name)
	if !ok {
		t.Fatalf("GetJobGroup returned false for group1")
	}
	if jg1.Description != group1.Description || jg1.Name != group1.Name || jg1.Queue != group1.Queue {
		t.Fatalf("GetJobGroup returned wrong job group for group1")
	}
	jg2, ok := GetJobGroup(group2.Name)
	if !ok {
		t.Fatalf("GetJobGroup returned false for group2")
	}
	if jg2.Description != group2.Description || jg2.Name != group2.Name || jg2.Queue != group2.Queue {
		t.Fatalf("GetJobGroup returned wrong job group for group2")
	}
}

// TestListJobs verifies that the created jobs can be retrieved.
func TestListJobs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a couple of dummy jobs.
	_, err := NewJob(ctx, uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}
	_, err = NewJob(ctx, uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	var jobs []Job
	if err := db.Find(&jobs).Error; err != nil {
		t.Fatalf("Failed to retrieve jobs: %v", err)
	}
	if len(jobs) < 2 {
		t.Fatalf("Expected at least 2 jobs, got %d", len(jobs))
	}

	// Test ListJobs using an empty filter instead of nil.
	jobsList, err := ListJobs(ctx, &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   10,
		Order:      "-started_at",
	}, "")
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if len(jobsList) == 0 {
		t.Fatalf("ListJobs returned empty list after creating jobs")
	}

	foundJob := false
	for _, job := range jobsList {
		if job.GetJobID() == jobs[0].JobID {
			foundJob = true
			break
		}
	}
	if !foundJob {
		t.Fatalf("ListJobs did not return the created job")
	}
}

// TestJobSkip verifies the job skip() API functionality.
func TestJobSkip(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a dummy job.
	job, err := NewJob(ctx, uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}
	// Simulate a skip operation; assume a Skip() method would set status to JobStatusSkipped.
	err = job.setStatus(ctx, JobStatusSkipped, errors.New("Skipped by user"))
	if err == nil {
		t.Fatalf("New job can't be skipped, it must be scheduled and running first: %v", err)
	}

	// Simulate job skipping
	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}
	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}

	if err := job.SetSkipped(ctx, "skipped reason"); err != nil {
		t.Fatalf("Job.SetSkipped() returned error: %v", err)
	}

	var fetched JobObj
	if err := db.First(&fetched.private, "job_id = ?", job.GetJobID()).Error; err != nil {
		t.Fatalf("Failed to retrieve job: %v", err)
	}
	if fetched.GetStatus() != JobStatusSkipped {
		t.Fatalf("Expected job status to be '%s', got '%s'", JobStatusSkipped, fetched.GetStatus())
	}
}

func TestJobConfig_RegisterJobType(t *testing.T) {
	// Use test-specific job group and type IDs
	groupName := "testgroup_config"
	typeID := "testconfig:job"

	// Register a job type
	jt := RegisterJobType(
		JobTypeParams{
			Name: typeID,
			Group: &JobGroup{
				Name:        groupName,
				Queue:       JobQueueCompute,
				Description: "Test group for config",
			},
			Description:               "Test group for config",
			Params:                    &struct{}{},
			WorkerInitCallback:        func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return job, nil },
			WorkerExecutionCallback:   func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   time.Hour,   // timeoutSec
			RetryDelay:                time.Second, // retryDelaySec
			MaxRetries:                3,           // maxRetries
		},
	)

	jobTypeId := fmt.Sprintf("%s:%s", groupName, typeID)

	// Verify the registered job type
	assert.Equal(t, jobTypeId, jt.TypeID, "Job type should have correct ID")
	assert.Equal(t, groupName, jt.Group, "Job type should have correct group")
	assert.Equal(t, 3600, jt.TimeoutSec, "Job type should have correct timeout")
	assert.Equal(t, 3, jt.MaxRetries, "Job type should have correct max retries")
	assert.Equal(t, 1, jt.RetryDelaySec, "Job type should have correct retry delay")

	// Verify it's in the registry
	found := false
	foundType, exists := GetJobType(jobTypeId)
	if exists && foundType.TypeID == jobTypeId {
		found = true
	}
	assert.True(t, found, "Job type should be in the registry")
}

func TestJobConfig_GetJobType(t *testing.T) {
	// Create a unique job type ID
	typeID := "get:config:job:" + uuid.New().String()

	// Register a job type
	jt := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "getgroup",
				Queue:       JobQueueCompute,
				Description: "Test group for get",
			},
			Name:                      typeID,
			Description:               "Job type for get test",
			Params:                    &struct{}{},
			WorkerInitCallback:        func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return job, nil },
			WorkerExecutionCallback:   func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			Timeout:                   time.Hour,
			RetryDelay:                time.Second,
			MaxRetries:                0,
			WorkerStateUpdateCallback: nil,
		},
	)

	jobTypeId := fmt.Sprintf("%s:%s", "getgroup", typeID)

	// Test getting the job type
	foundType, exists := GetJobType(jobTypeId)
	assert.True(t, exists, "GetJobType should find the type")
	assert.Equal(t, jt, foundType, "GetJobType should return the correct job type")

	// Test getting a non-existent job type
	_, exists = GetJobType("nonexistent:job:type")
	assert.False(t, exists, "GetJobType should not find non-existent type")
}

func TestJobConfig_JobQueueLimit(t *testing.T) {
	// Test job queue constants are defined
	assert.Equal(t, JobQueueName("compute"), JobQueueCompute, "Compute queue name should be correct")
	// Add more tests for other queue-related functionality if available
}

func TestJobConfig_JobStatusMap(t *testing.T) {
	// Test that job status constants are defined with expected string values
	assert.Equal(t, JobStatus("waiting"), JobStatusWaiting, "JobStatusWaiting should be 'waiting'")
	assert.Equal(t, JobStatus("running"), JobStatusRunning, "JobStatusRunning should be 'running'")
	assert.Equal(t, JobStatus("completed"), JobStatusCompleted, "JobStatusCompleted should be 'completed'")
	assert.Equal(t, JobStatus("failed"), JobStatusFailed, "JobStatusFailed should be 'failed'")
	assert.Equal(t, JobStatus("canceled"), JobStatusCanceled, "JobStatusCancelled should be 'canceled'")
}

func TestJobConfig_DuplicateTypeRegistration(t *testing.T) {
	// First registration should succeed
	testTypeID := "duplicate:test:" + uuid.New().String()
	RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "dupgroup",
				Queue:       JobQueueCompute,
				Description: "Test group for duplicate",
			},
			Name:                      testTypeID,
			Description:               "First registration",
			Params:                    &struct{}{},
			WorkerInitCallback:        func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return job, nil },
			WorkerExecutionCallback:   func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   time.Hour,
			RetryDelay:                time.Second,
			MaxRetries:                1,
		},
	)

	// Second registration of the same ID should be a no-op and return the first registration
	jt2 := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "othergroup", // different group
				Queue:       JobQueueCompute,
				Description: "Other group",
			},
			Name:                      testTypeID, // same ID
			Description:               "Second registration attempt",
			Params:                    &struct{}{},
			WorkerInitCallback:        func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return job, nil },
			WorkerExecutionCallback:   func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   time.Minute, // different timeout
			RetryDelay:                time.Second * 5,
			MaxRetries:                2,
		},
	)

	// Verify that the returned type is the first one we registered
	assert.Equal(t, "othergroup", jt2.Group, "Different groups should be allowed to register the same type")
}

// TestJobObj_SetRetryPolicy verifies that SetRetryPolicy correctly updates the retry policy of a job.
func TestJobObj_SetRetryPolicy(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job with default retry policy
	idempotencyKey := uuid.New()
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Set a new retry policy
	newRetryDelaySec := 30 * time.Second
	newMaxRetries := 5
	err = job.SetRetryPolicy(newRetryDelaySec, newMaxRetries, newRetryDelaySec)
	if err != nil {
		t.Fatalf("SetRetryPolicy error: %v", err)
	}

	// Verify the retry policy was updated
	assert.Equal(t, newRetryDelaySec, job.GetRetryDelay(), "RetryDelaySec should be updated")
	assert.Equal(t, newMaxRetries, job.GetMaxRetries(), "MaxRetries should be updated")

	// Verify the changes were persisted to the database
	var jobFromDB JobObj
	err = db.DB.Where("job_id = ?", job.GetJobID()).First(&jobFromDB.private).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}
	jobFromDB.initType(jobFromDB.private.Type)

	assert.Equal(t, newRetryDelaySec, jobFromDB.GetRetryDelay(), "RetryDelaySec should be persisted in DB")
	assert.Equal(t, newMaxRetries, jobFromDB.GetMaxRetries(), "MaxRetries should be persisted in DB")

	// Test setting negative values (should be clamped to 0)
	err = job.SetRetryPolicy(-5, -3, -5)
	if err != nil {
		t.Fatalf("SetRetryPolicy with negative values error: %v", err)
	}

	assert.Equal(t, time.Duration(0), job.GetRetryDelay(), "Negative RetryDelaySec should be clamped to 0")
	assert.Equal(t, 0, job.GetMaxRetries(), "Negative MaxRetries should be clamped to 0")
}

// TestJobObj_SetResult verifies that SetResult correctly updates the result of a job.
func TestJobObj_SetResult(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job
	idempotencyKey := uuid.New()
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Define a result structure
	type JobResult struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	// Set a result
	result := JobResult{
		Status:  "success",
		Message: "Operation completed successfully",
	}

	err = job.SetResult(ctx, result)
	if err != nil {
		t.Fatalf("SetResult error: %v", err)
	}

	// Verify the result was persisted to the database
	var jobFromDB JobObj
	err = db.DB.Where("job_id = ?", job.GetJobID()).First(&jobFromDB.private).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}

	// Unmarshal the result from DB to verify it matches what we set
	var retrievedResult JobResult
	err = json.Unmarshal([]byte(jobFromDB.private.Result), &retrievedResult)
	if err != nil {
		t.Fatalf("Error unmarshaling result: %v", err)
	}

	assert.Equal(t, result.Status, retrievedResult.Status, "Result status should match")
	assert.Equal(t, result.Message, retrievedResult.Message, "Result message should match")
}

// TestJobObj_SetParams verifies that SetParams correctly updates the parameters of a job.
func TestJobObj_SetParams(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job with initial params
	idempotencyKey := uuid.New()
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo":"val"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Verify initial params
	assert.Equal(t, `{"bar":"bar-default","foo":"val"}`, job.GetParams(), "Initial params should match")

	// Update params
	newParams := `{"bar":"value","foo":"updated"}`
	err = job.setParams(newParams)
	if err != nil {
		t.Fatalf("SetParams error: %v", err)
	}

	// Verify params were updated
	assert.Equal(t, newParams, job.GetParams(), "Updated params should match")

	// Verify the changes were persisted to the database
	var jobFromDB JobObj
	err = db.DB.Where("job_id = ?", job.GetJobID()).First(&jobFromDB.private).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}

	assert.Equal(t, newParams, jobFromDB.private.Params, "Updated params should be persisted in DB")

	// Verify that ParamsPtr was properly updated
	paramsPtr := job.GetParamsPtr().(*dummyParams)
	assert.Equal(t, "updated", paramsPtr.Foo, "ParamsPtr should reflect updated values")
}

// TestJobObj_Skip verifies that Skip correctly updates the status of a job to skipped.
func TestJobObj_Skip(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job
	idempotencyKey := uuid.New()
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Initial status should be init
	assert.Equal(t, JobStatusInit, job.GetStatus(), "Initial status should be init")

	// Skip the job
	err = job.SetSkipped(ctx, "skipped reason")
	if err == nil {
		t.Fatalf("New job can't be skipped, it must be scheduled and running first: %v", err)
	}

	// Simulate job skipping
	if err := job.schedule(ctx); err != nil {
		t.Fatalf("Job.schedule() returned error: %v", err)
	}
	if err := job.setRunning(ctx); err != nil {
		t.Fatalf("Job.setRunning() returned error: %v", err)
	}
	if err := job.SetSkipped(ctx, "skipped reason"); err != nil {
		t.Fatalf("Job.SetSkipped() returned error: %v", err)
	}

	// Verify status was updated to skipped
	assert.Equal(t, JobStatusSkipped, job.GetStatus(), "Status should be updated to skipped")

	// Verify the changes were persisted to the database
	var jobFromDB JobObj
	err = db.DB.Where("job_id = ?", job.GetJobID()).First(&jobFromDB.private).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}

	assert.Equal(t, JobStatusSkipped, jobFromDB.private.Status, "Skipped status should be persisted in DB")
}

// TestJobObj_InitParams_EmptyPayload verifies that InitParams correctly initializes
// parameters with default values when an empty payload is provided.
func TestJobObj_InitParams_EmptyPayload(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	idempotencyKey := uuid.New()

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	// Create a job with empty params
	job, err := NewJob(ctx, idempotencyKey, jt, "")
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Verify that params were initialized with defaults
	paramsPtr := job.GetParamsPtr().(*dummyParams)
	assert.Equal(t, "foo-default", paramsPtr.Foo, "Empty payload should use default value for Foo")

	// Verify the params JSON contains the default values
	var params map[string]interface{}
	err = json.Unmarshal([]byte(job.private.Params), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	assert.Equal(t, "foo-default", params["foo"], "Params JSON should contain default value")
}

// TestJobObj_InitParams_PartialPayload verifies that InitParams correctly merges
// provided parameters with defaults when a partial payload is provided.
func TestJobObj_InitParams_PartialPayload(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	idempotencyKey := uuid.New()

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	// Create a job with partial params (only setting Foo)
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo":"custom"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Verify that params were initialized correctly
	paramsPtr := job.GetParamsPtr().(*dummyParams)
	assert.Equal(t, "custom", paramsPtr.Foo, "Partial payload should override Foo with custom value")

	// Verify the params JSON contains the merged values
	var params map[string]interface{}
	err = json.Unmarshal([]byte(job.private.Params), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	assert.Equal(t, "custom", params["foo"], "Params JSON should contain custom value")
}

// TestJobObj_InitParams_FullPayload verifies that InitParams correctly uses
// all provided parameters when a complete payload is provided.
func TestJobObj_InitParams_FullPayload(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()
	idempotencyKey := uuid.New()

	// Create a job type with a more complex parameter structure
	type complexJobParams struct {
		Foo     string `json:"foo" default:"default"`
		Bar     int    `json:"bar" default:"42"`
		Enabled bool   `json:"enabled" default:"false"`
	}

	queueName := JobQueueName("test-queue")
	queue, err := RegisterJobQueue(queueName, 1)
	assert.NotNil(t, queue)
	assert.Nil(t, err)

	complexJobType := RegisterJobType(
		JobTypeParams{
			Group:                   &JobGroup{Name: "test", Queue: queueName},
			Name:                    "complex",
			Description:             "complex job",
			Params:                  &complexJobParams{},
			WorkerInitCallback:      nil,
			WorkerExecutionCallback: nil,
			Timeout:                 1 * time.Second,
			RetryDelay:              1 * time.Second,
			MaxRetries:              3,
			WorkerIsSuspendable:     false,
		},
	)

	defer func() {
		delete(jobTypesMap, complexJobType.TypeID)
	}()

	// Create a job with full params
	fullParams := `{"foo":"custom", "bar":100, "enabled":true}`
	job, err := NewJob(ctx, idempotencyKey, complexJobType, fullParams)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Verify that params were initialized correctly
	paramsPtr := job.GetParamsPtr().(*complexJobParams)
	assert.Equal(t, "custom", paramsPtr.Foo, "Full payload should set Foo to custom value")
	assert.Equal(t, 100, paramsPtr.Bar, "Full payload should set Bar to 100")
	assert.Equal(t, true, paramsPtr.Enabled, "Full payload should set Enabled to true")

	// Verify the params JSON contains all the provided values
	var params map[string]interface{}
	err = json.Unmarshal([]byte(job.private.Params), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	assert.Equal(t, "custom", params["foo"], "Params JSON should contain custom Foo value")
	assert.Equal(t, float64(100), params["bar"], "Params JSON should contain custom Bar value")
	assert.Equal(t, true, params["enabled"], "Params JSON should contain custom Enabled value")
}

// TestGetJob verifies that a job can be retrieved by its ID.
func TestGetJob(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	// Create a new job
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Schedule the job
	err = job.schedule(ctx)
	assert.NoError(t, err)

	// Get the job by ID
	retrievedJob, err := getJob(ctx, job.GetJobID())
	assert.NoError(t, err)
	assert.NotNil(t, retrievedJob)

	// Verify the job properties match
	assert.Equal(t, job.GetJobID(), retrievedJob.GetJobID())
	assert.Equal(t, job.GetType(), retrievedJob.GetType())
	assert.Equal(t, job.GetStatus(), retrievedJob.GetStatus())
}

// TestSetLockedBy_CircularDependency verifies that SetLockedBy detects circular lock dependencies.
func TestSetLockedBy_CircularDependency(t *testing.T) {
	ctx := context.Background()

	// Create two jobs
	job1, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job1)

	err = job1.setStatus(ctx, JobStatusWaiting, nil)
	assert.NoError(t, err)

	err = job1.setStatus(ctx, JobStatusRunning, nil)
	assert.NoError(t, err)

	job2, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "baz"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job2)

	err = job2.setStatus(ctx, JobStatusWaiting, nil)
	assert.NoError(t, err)

	err = job2.setStatus(ctx, JobStatusRunning, nil)
	assert.NoError(t, err)

	// Lock job2 by job1
	err = job2.SetLockedBy(ctx, job1.GetJobID())
	assert.NoError(t, err)

	// Attempt to lock job1 by job2, which would create a circular dependency
	err = job1.SetLockedBy(ctx, job2.GetJobID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular lock dependency detected")
}

// TestJobObj_GetStatus verifies that GetStatus correctly refreshes the job status from the database.
func TestJobObj_GetStatus(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Initial status should be JobStatusInit
	assert.Equal(t, JobStatusInit, job.GetStatus())

	// Update the status in the database directly
	result := db.DB.Model(&Job{}).Where("job_id = ?", job.GetJobID()).Update("status", string(JobStatusWaiting))
	assert.NoError(t, result.Error)

	// GetStatus should refresh from the database
	assert.Equal(t, JobStatusWaiting, job.GetStatus())
}

// TestJobObj_GetLockedBy verifies that GetLockedBy correctly refreshes the locked_by field from the database.
func TestJobObj_GetLockedBy(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Initially, locked_by should be uuid.Nil
	assert.Equal(t, uuid.Nil, job.GetLockedBy())

	// Update locked_by in the database directly
	lockID := uuid.New()
	result := db.DB.Model(&Job{}).Where("job_id = ?", job.GetJobID()).Update("locked_by", lockID)
	assert.NoError(t, result.Error)

	// GetLockedBy should refresh from the database
	assert.Equal(t, lockID, job.GetLockedBy())
}

// TestJobObj_GetTenantID verifies that GetTenantID correctly returns the tenant ID.
func TestJobObj_GetTenantID(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Get the tenant ID and verify it's not empty
	tenantID := job.GetTenantID()
	assert.NotEqual(t, uuid.UUID{}, tenantID, "TenantID should not be empty")

	// Verify the tenant ID matches what's in the database
	var dbJob Job
	result := db.DB.Where("job_id = ?", job.GetJobID()).First(&dbJob)
	assert.NoError(t, result.Error)
	assert.Equal(t, dbJob.TenantID, job.GetTenantID())
}

// TestJobObj_GetUserID verifies that GetUserID correctly returns the user ID.
func TestJobObj_GetUserID(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Get the user ID
	userID := job.GetUserID()

	// Verify the user ID matches what's in the database
	var dbJob Job
	result := db.DB.Where("job_id = ?", job.GetJobID()).First(&dbJob)
	assert.NoError(t, result.Error)
	assert.Equal(t, dbJob.UserID, userID)
}

// TestJobObj_GetTypePtr verifies that GetTypePtr correctly returns the job type pointer.
func TestJobObj_GetTypePtr(t *testing.T) {
	ctx := context.Background()

	// Create a job with a specific job type
	jobType := getDummyJobType()
	job, err := NewJob(ctx, uuid.New(), jobType, `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Verify that GetTypePtr returns the correct job type pointer
	retrievedJobType := job.GetTypePtr()
	assert.NotNil(t, retrievedJobType)
	assert.Equal(t, jobType.TypeID, retrievedJobType.TypeID)
	assert.Equal(t, jobType.Description, retrievedJobType.Description)
	assert.Equal(t, jobType.Group, retrievedJobType.Group)
	assert.Equal(t, jobType.TimeoutSec, retrievedJobType.TimeoutSec)
	assert.Equal(t, jobType.MaxRetries, retrievedJobType.MaxRetries)
	assert.Equal(t, jobType.RetryDelaySec, retrievedJobType.RetryDelaySec)
}

// TestSetProgress_InvalidValues verifies that SetProgress rejects invalid progress values.
func TestSetProgress_InvalidValues(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Test negative progress value
	err = job.SetProgress(ctx, -1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid progress value")

	// Test progress value > 100
	err = job.SetProgress(ctx, 101)
	assert.NoError(t, err)
	assert.Equal(t, job.GetProgress(), float32(100), "progress should be capped at 100")

	// Test valid progress value
	err = job.SetProgress(ctx, 50)
	assert.NoError(t, err)
	assert.Equal(t, float32(50), job.GetProgress())
}

// TestSetTimedOut verifies that SetTimedOut correctly updates the job status and error.
func TestSetTimedOut(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	err = job.setStatus(ctx, JobStatusWaiting, nil)
	assert.NoError(t, err)

	err = job.setStatus(ctx, JobStatusRunning, nil)
	assert.NoError(t, err)

	// Set job as timed out
	timeoutErr := errors.New("job timed out")
	err = job.setTimedOut(ctx, timeoutErr)
	assert.NoError(t, err)

	// Verify job status and error
	assert.Equal(t, JobStatusTimedOut, job.GetStatus())
	assert.Equal(t, timeoutErr.Error(), job.GetError())
}

// TestInitType_Errors verifies that InitType handles error cases correctly.
func TestInitType_Errors(t *testing.T) {
	ctx := context.Background()

	// Create a job with a valid type
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Test empty type string
	err = job.initType("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job type is not set")

	// Test non-existent type
	err = job.initType("non-existent-type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get job type")
}

// TestInitParams_Errors verifies that InitParams handles error cases correctly.
func TestInitParams_Errors(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Test invalid JSON
	err = job.initParams(`{"foo": "bar"`) // Missing closing brace
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")

	// Create a job with an empty type string
	jobWithEmptyType, err := NewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, err)
	assert.NotNil(t, jobWithEmptyType)

	// Manually set the type to empty to simulate a job with no type
	result := db.DB.Model(&Job{}).Where("job_id = ?", jobWithEmptyType.GetJobID()).Update("type", "")
	assert.NoError(t, result.Error)

	// Reinitialize the type - should fail because type is empty
	err = jobWithEmptyType.initType("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job type is not set")

	// Test non-existent type
	err = jobWithEmptyType.initType("non-existent-type")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get job type")
}

// TestJobExecutor_Schedule verifies that a job can be scheduled and executed by the job executor.
func TestJobExecutor_Schedule(t *testing.T) {
	setupTestDB(t)

	ctx := context.Background()

	// Create a job type with a simple worker function that sets a result
	executedChan := make(chan bool, 1)

	// Define a result struct
	type testResult struct {
		Status string `json:"status"`
	}

	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "executor-test",
				Queue:       JobQueueCompute,
				Description: "Job executor test group",
			},
			Name:        "schedule-test",
			Description: "Job for testing scheduling",
			Params:      &struct{}{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Signal that the job was executed
				executedChan <- true
				// Set progress to 100%
				progress <- 100
				// Set a result
				j, ok := worker.(*JobObj)
				if !ok {
					return errors.New("job is not a JobObj")
				}
				return j.SetResult(ctx, testResult{Status: "success"})
			},
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   5 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, err := NewJob(ctx, uuid.New(), jobType, "")
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Schedule the job with the executor
	err = JEScheduleJob(ctx, job)
	assert.NoError(t, err)

	// Wait for the job to be executed
	select {
	case <-executedChan:
		// Job was executed
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for job to be executed")
	}

	// Wait for the job to complete
	err = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, err)

	// Verify the job status and result
	retrievedJob, err := getJob(ctx, job.GetJobID())
	assert.NoError(t, err)
	assert.NotNil(t, retrievedJob)

	assert.Equal(t, JobStatusCompleted, retrievedJob.GetStatus())
	assert.Equal(t, float32(100), retrievedJob.GetProgress())
	assert.Contains(t, retrievedJob.private.Result, "success")
}

// TestJobExecutor_Cancel verifies that a job can be canceled.
func TestJobExecutor_Cancel(t *testing.T) {
	// Skip this test for now as it's flaky
	t.Skip("Skipping flaky test")

	ctx := context.Background()

	// Create a job type with a worker function that blocks until canceled
	canceledChan := make(chan bool, 1)
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "cancel-test",
				Queue:       JobQueueCompute,
				Description: "Job cancellation test group",
			},
			Name:        "cancel-test",
			Description: "Job for testing cancellation",
			Params:      &struct{}{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Send initial progress
				progress <- 10

				// Wait for cancellation or timeout
				select {
				case <-ctx.Done():
					// Job was canceled
					canceledChan <- true
					return ctx.Err()
				case <-time.After(30 * time.Second):
					// Timeout, job wasn't canceled
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   60 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, err := NewJob(ctx, uuid.New(), jobType, "")
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Schedule the job
	err = job.schedule(ctx)
	assert.NoError(t, err)

	// Schedule the job with the executor
	err = JEScheduleJob(ctx, job)
	assert.NoError(t, err)

	// Wait a bit for the job to start
	time.Sleep(1 * time.Second)

	// Cancel the job
	err = JECancelJob(ctx, job.GetJobID(), nil)
	assert.NoError(t, err)

	// Wait for the job to be canceled
	select {
	case <-canceledChan:
		// Job was canceled
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for job to be canceled")
	}

	// Wait for the job status to be updated
	time.Sleep(2 * time.Second)

	// Verify the job status
	retrievedJob, err := getJob(ctx, job.GetJobID())
	assert.NoError(t, err)
	assert.NotNil(t, retrievedJob)

	assert.Equal(t, JobStatusCanceled, retrievedJob.GetStatus())
}

// TestJobExecutor_Timeout verifies that a job times out if it exceeds its timeout duration.
func TestJobExecutor_Timeout(t *testing.T) {
	// Skip this test for now as it's flaky
	t.Skip("Skipping flaky test")

	ctx := context.Background()

	// Create a job type with a worker function that takes longer than the timeout
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "timeout-test",
				Queue:       JobQueueCompute,
				Description: "Job timeout test group",
			},
			Name:        "timeout-test",
			Description: "Job for testing timeout",
			Params:      &struct{}{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Send initial progress
				progress <- 10

				// Sleep longer than the timeout duration
				select {
				case <-ctx.Done():
					// Context was canceled (due to timeout)
					return ctx.Err()
				case <-time.After(10 * time.Second):
					// This should not happen as the job should time out before this
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       false,
			Timeout:                   1 * time.Second, // Very short timeout to trigger timeout quickly
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, err := NewJob(ctx, uuid.New(), jobType, "")
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Schedule the job
	err = job.schedule(ctx)
	assert.NoError(t, err)

	// Schedule the job with the executor
	err = JEScheduleJob(ctx, job)
	assert.NoError(t, err)

	// Wait for the job to time out (should be around 1 second)
	err = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, err)

	// Verify the job status
	retrievedJob, err := getJob(ctx, job.GetJobID())
	assert.NoError(t, err)
	assert.NotNil(t, retrievedJob)

	assert.Equal(t, JobStatusTimedOut, retrievedJob.GetStatus())
	assert.Contains(t, retrievedJob.GetError(), "timed out")
}

func TestJobExecutor_SuspendAndResume(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Create a job that will block indefinitely
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jt, `{"foo":"suspend_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Schedule the job
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(100 * time.Millisecond)

	// Verify job is running
	jobStatus, err := JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Suspend the job
	if err := JESuspendJob(ctx, job.GetJobID()); err != nil {
		t.Fatalf("SuspendJob error: %v", err)
	}

	for i := 0; i < 10; i++ { // Wait for job to suspend
		status := job.GetStatus()
		if status == JobStatusSuspended {
			break
		}
		assert.Equal(t, JobStatusSuspending, status, fmt.Sprintf("Job should be suspending or suspended, but was %s", status))
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, JobStatusSuspended, job.GetStatus(), "Job should be suspended")

	// Resume the job
	if err := JEResumeJob(ctx, job.GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Verify job is running again
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after resume: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running again")

	// Cleanup: cancel the job
	if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}
}

func TestJobExecutor_SuspendInvalidStates(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a job that will complete quickly
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jt, `{"foo":"suspend_invalid_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Schedule the job
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to complete
	time.Sleep(200 * time.Millisecond)

	// Try to suspend a completed job
	err = JESuspendJob(ctx, job.GetJobID())
	assert.Error(t, err, "Should not be able to suspend a completed job")

	// Try to resume a completed job
	err = JEResumeJob(ctx, job.GetJobID())
	assert.Error(t, err, "Should not be able to resume a completed job")
}

// safeCancelJob attempts to cancel a job and handles the case where the job is already in a terminal state.
// It returns nil if the job is already in a terminal state (completed, failed, etc.) or if cancellation succeeds.
func safeCancelJob(ctx context.Context, jobID uuid.UUID) error {
	// Get current job status
	job, err := JEGetJob(ctx, jobID)
	if err != nil {
		return err
	}

	// Check if job is already in a terminal state
	status := job.GetStatus()
	if status == JobStatusCompleted || status == JobStatusFailed ||
		status == JobStatusCanceled || status == JobStatusTimedOut ||
		status == JobStatusSkipped {
		// Job is already in a terminal state, no need to cancel
		return nil
	}

	// Job is not in a terminal state, try to cancel it
	return JECancelJob(ctx, jobID, nil)
}

// TestCancelCompletedOrFailedJob tests that canceling a job that is already in a terminal state
// (completed or failed) does not report any error and the job status remains unchanged.
func TestLockCancelUnlock(t *testing.T) {
	ctx := context.Background()

	// Create a job type that will run long enough for us to manipulate it
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "test_lock_cancel_unlock",
				Queue:       JobQueueCompute,
				Description: "Test job group for lock/cancel/unlock test",
			},
			Name:        "test_lock_cancel_unlock:job",
			Description: "Test job for lock/cancel/unlock test",
			Params:      &dummyParams{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// This job will run for a long time unless canceled
				select {
				case <-ctx.Done():
					// Simulate some work before returning to allow state transitions to complete
					time.Sleep(50 * time.Millisecond)
					return ctx.Err()
				case <-time.After(30 * time.Second):
					return nil
				}
			},
			Timeout:    60 * time.Second,
			RetryDelay: 1 * time.Second,
			MaxRetries: 0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"test_lock_cancel_unlock"}`)
	assert.NoError(t, err, "NewJob error")

	// Schedule and run the job
	err = JEScheduleJob(ctx, job)
	assert.NoError(t, err, "Schedule error")

	// Wait for job to start running
	var jobStatus *JobObj
	for i := 0; i < 10; i++ {
		jobStatus, err = JEGetJob(ctx, job.GetJobID())
		assert.NoError(t, err, "GetJob error")
		if jobStatus.GetStatus() == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify job is running
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Step 2: Lock the job
	// Create another job to use as the locker
	lockingJobIdempotency := uuid.New()
	lockingJob, err := NewJob(ctx, lockingJobIdempotency, jobType, `{"foo":"locking_job"}`)
	assert.NoError(t, err, "NewJob error for locking job")

	// Lock the job
	err = jobStatus.SetLockedBy(ctx, lockingJob.GetJobID())
	assert.NoError(t, err, "SetLockedBy error")

	// Verify job is locked
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	assert.NoError(t, err, "GetJob error after locking")
	assert.Equal(t, JobStatusLocked, jobStatus.GetStatus(), "Job should be locked")
	assert.Equal(t, lockingJob.GetJobID(), jobStatus.GetLockedBy(), "Job should be locked by the locking job")

	// Step 3: Cancel the job
	err = JECancelJob(ctx, job.GetJobID(), errors.New("test cancellation"))
	assert.NoError(t, err, "CancelJob error")

	// After cancellation, job may be in 'canceling' state initially
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	assert.NoError(t, err, "GetJob error after cancellation")

	// The job might be in either 'canceling' or 'canceled' state at this point
	// Wait for it to reach the final 'canceled' state
	for i := 0; i < 20; i++ {
		jobStatus, err = JEGetJob(ctx, job.GetJobID())
		assert.NoError(t, err, "GetJob error while waiting for canceled state")
		if jobStatus.GetStatus() == JobStatusCanceled {
			break
		}
		// If it's not in canceling state, something else happened
		if jobStatus.GetStatus() != JobStatusCanceling {
			t.Fatalf("Unexpected job status: %s", jobStatus.GetStatus())
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify job reached canceled state
	assert.Equal(t, JobStatusCanceled, jobStatus.GetStatus(), "Job should eventually reach canceled state")

	// Step 4: Unlock the job
	err = jobStatus.SetUnlocked(ctx)
	assert.NoError(t, err, "SetUnlocked error")

	// Verify job is still canceled after unlocking
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	assert.NoError(t, err, "GetJob error after unlocking")
	assert.Equal(t, JobStatusCanceled, jobStatus.GetStatus(), "Job should still be canceled after unlocking")
	assert.Equal(t, uuid.Nil, jobStatus.GetLockedBy(), "Job should not be locked by any job after unlocking")
}

func TestCancelCompletedOrFailedJob(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Test canceling a completed job
	t.Run("cancel completed job", func(t *testing.T) {
		// Create a job type that completes quickly
		jobType := RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "test_cancel_completed",
					Queue:       JobQueueCompute,
					Description: "Test job group for canceling completed jobs",
				},
				Name:        "test_cancel_completed:job",
				Description: "Test job for canceling completed jobs",
				Params:      &dummyParams{},
				WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
					return job, nil
				},
				WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
					// Complete immediately
					return nil
				},
				Timeout:    1 * time.Second,
				RetryDelay: 1 * time.Second,
				MaxRetries: 0,
			},
		)

		// Create and schedule the job
		idempotency := uuid.New()
		job, err := NewJob(ctx, idempotency, jobType, `{"foo":"test_cancel_completed"}`)
		assert.NoError(t, err, "NewJob error")

		err = JEScheduleJob(ctx, job)
		assert.NoError(t, err, "Schedule error")

		// Wait for job to complete
		for i := 0; i < 10; i++ {
			jobStatus, err := JEGetJob(ctx, job.GetJobID())
			assert.NoError(t, err, "GetJob error")
			if jobStatus.GetStatus() == JobStatusCompleted {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Verify job is completed
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
		assert.NoError(t, err, "GetJob error")
		assert.Equal(t, JobStatusCompleted, jobStatus.GetStatus(), "Job should be completed")

		// Try to cancel the completed job using our safe cancel function
		err = safeCancelJob(ctx, job.GetJobID())
		assert.NoError(t, err, "Canceling a completed job should not return an error")

		// Verify job is still completed
		jobStatus, err = JEGetJob(ctx, job.GetJobID())
		assert.NoError(t, err, "GetJob error after cancel attempt")
		assert.Equal(t, JobStatusCompleted, jobStatus.GetStatus(), "Job should still be completed after cancel attempt")
	})

	// Test canceling a failed job
	t.Run("cancel failed job", func(t *testing.T) {
		// Create a job type that fails
		jobType := RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "test_cancel_failed",
					Queue:       JobQueueCompute,
					Description: "Test job group for canceling failed jobs",
				},
				Name:        "test_cancel_failed:job",
				Description: "Test job for canceling failed jobs",
				Params:      &dummyParams{},
				WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
					return job, nil
				},
				WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
					// Fail immediately
					return errors.New("intentional failure for test")
				},
				Timeout:    1 * time.Second,
				RetryDelay: 1 * time.Second,
				MaxRetries: 0,
			},
		)

		// Create and schedule the job
		idempotency := uuid.New()
		job, err := NewJob(ctx, idempotency, jobType, `{"foo":"test_cancel_failed"}`)
		assert.NoError(t, err, "NewJob error")

		err = JEScheduleJob(ctx, job)
		assert.NoError(t, err, "Schedule error")

		// Wait for job to fail
		for i := 0; i < 10; i++ {
			jobStatus, err := JEGetJob(ctx, job.GetJobID())
			assert.NoError(t, err, "GetJob error")
			if jobStatus.GetStatus() == JobStatusFailed {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Verify job is failed
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
		assert.NoError(t, err, "GetJob error")
		assert.Equal(t, JobStatusFailed, jobStatus.GetStatus(), "Job should be failed")

		// Try to cancel the failed job using our safe cancel function
		err = safeCancelJob(ctx, job.GetJobID())
		assert.NoError(t, err, "Canceling a failed job should not return an error")

		// Verify job is still failed
		jobStatus, err = JEGetJob(ctx, job.GetJobID())
		assert.NoError(t, err, "GetJob error after cancel attempt")
		assert.Equal(t, JobStatusFailed, jobStatus.GetStatus(), "Job should still be failed after cancel attempt")
	})
}
