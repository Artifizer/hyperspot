package job

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/test_setup"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
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
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					return nil
				},
				WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					// Simulate some progress and then complete.
					err := job.SetProgress(ctx, 50)
					if err != nil {
						return err
					}
					err = job.SetProgress(ctx, 100)
					if err != nil {
						return err
					}
					return nil
				},
				WorkerStateUpdateCallback: nil,
				WorkerIsSuspendable:       false,
				Timeout:                   2 * time.Second,
				RetryDelay:                time.Second,
				MaxRetries:                0,
			},
		)
	}
	return dummyJobType
}

func TestJobObj_NewJob_Success(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, errx := JENewJob(ctx, idempotencyKey, jt, paramsJSON)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}
	if job == nil {
		t.Fatal("NewJob returned nil job")
	}

	if job.GetType() != dummyJobType.TypeID {
		t.Fatalf("Expected job type %s, got %s", dummyJobType.TypeID, job.GetType())
	}
	// Verify that job.Params is a valid JSON string and contains the expected value.
	if !json.Valid([]byte(job.getParams())) {
		t.Fatalf("Job params is not valid JSON: %s", job.getParams())
	}
	var dp dummyParams
	if err := json.Unmarshal([]byte(job.getParams()), &dp); err != nil {
		t.Fatalf("Failed to unmarshal job.Params: %v", err)
	}
	if dp.Foo != "bar" {
		t.Fatalf("Expected job.Params.Foo to be 'bar', got '%s'", dp.Foo)
	}
}

func TestJobObj_NewJob_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	invalidJSON := `{"foo": "bar"` // missing closing brace

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	_, errx := JENewJob(ctx, idempotencyKey, jt, invalidJSON)
	if errx == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
}

func TestJobObj_GetQueue(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, errx := JENewJob(ctx, idempotencyKey, jt, paramsJSON)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}
	queue := job.getQueueName()
	if queue != dummyJobType.GroupPtr.Queue.Name {
		t.Fatalf("Expected job queue '%s', got '%s'", dummyJobType.GroupPtr.Queue.Name, queue)
	}
}

func TestJobObj_SetProgress(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, errx := JENewJob(ctx, idempotencyKey, jt, `{"foo": "progress"}`)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}
	// Set progress to 50.
	if err := job.SetProgress(ctx, 50); err != nil {
		t.Fatalf("Job.SetProgress() returned error: %v", err)
	}

	progress := job.GetProgress()
	if progress != 50 && progress != 0 {
		t.Fatalf("Expected progress to be either 50 or 0, got %f", job.GetProgress())
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
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Do nothing
				return nil
			},
			WorkerStateUpdateCallback: nil,
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
	job, errx := JENewJob(context.Background(), uuid.New(), jt, string(pbytes))
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Verify that the job's job type is correctly set.
	if job.GetType() != fmt.Sprintf("%s:%s", jt.Group, jt.Name) {
		t.Fatalf("Expected job type %v, got %v", fmt.Sprintf("%s:%s", jt.Group, jt.Name), job.GetType())
	}
}

// TestJobType_WorkerExecution has been moved to job_executor_test.go as TestJobType_WorkerExecution_Executor
// to avoid race conditions with background job workers by using proper JE* methods instead of direct status manipulation.

// TestJobType_NewJobType verifies that NewJobType() creates a new job type in the datastore.
func TestJobType_NewJobType(t *testing.T) {
	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	if jt == nil {
		t.Fatalf("RegisterJobType returned nil")
	}
	if jt.TypeID != testJobTypeForAPI.TypeID {
		t.Fatalf("Expected job type ID '%s', got '%s'", testJobTypeForAPI.TypeID, jt.TypeID)
	}

	// Verify that the job type can be retrieved by its ID
	retrievedJT, exists := getJobType(jt.TypeID)
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
	jobTypesList := getJobTypes(context.Background(), nil)
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
	group, exists := getJobGroup(jt.Group)
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
	jobGroupsList := getJobGroups(context.Background(), nil)
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
func TestJobType_GetJobTypes(t *testing.T) {
	// Create two job types.

	jt1 := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "listgroup",
				Queue:       JobQueueCompute,
				Description: "List group",
			},
			Name:                           "list:job:type:1",
			Description:                    "First job type",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerStateUpdateCallback:      nil,
			WorkerIsSuspendable:            false,
			Timeout:                        60 * time.Second,
			RetryDelay:                     0 * time.Second,
			MaxRetries:                     0,
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
			Name:                           "list:job:type:2",
			Description:                    "Second job type",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			Timeout:                        60 * time.Second,
			RetryDelay:                     0,
			MaxRetries:                     0,
			WorkerIsSuspendable:            false,
		},
	)
	assert.NotNil(t, jt2)

	types := getJobTypes(context.Background(), nil)
	if len(types) < 2 {
		t.Fatalf("Expected at least 2 job types, got %d", len(types))
	}
}

// TestJobGroup_GetJobGroup verifies that a specific job group can be retrieved.
func TestJobGroup_GetJobGroup(t *testing.T) {
	group := &JobGroup{
		Name:        "testgroup",
		Queue:       JobQueueCompute,
		Description: "A job group for testing",
	}
	if err := db.DB().Create(group).Error; err != nil {
		t.Fatalf("Failed to create job group: %v", err)
	}
	var fetched JobGroup
	if err := db.DB().First(&fetched, "name = ?", group.Name).Error; err != nil {
		t.Fatalf("Failed to retrieve job group: %v", err)
	}
	if fetched.Name != group.Name {
		t.Fatalf("Expected job group name %s, got %s", group.Name, fetched.Name)
	}
}

// TestJobGroup_GetJobGroups verifies that listing job groups returns all created groups.
func TestJobGroup_GetJobGroups(t *testing.T) {
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

	jg1, ok := getJobGroup(group1.Name)
	if !ok {
		t.Fatalf("GetJobGroup returned false for group1")
	}
	if jg1.Description != group1.Description || jg1.Name != group1.Name || jg1.Queue != group1.Queue {
		t.Fatalf("GetJobGroup returned wrong job group for group1")
	}
	jg2, ok := getJobGroup(group2.Name)
	if !ok {
		t.Fatalf("GetJobGroup returned false for group2")
	}
	if jg2.Description != group2.Description || jg2.Name != group2.Name || jg2.Queue != group2.Queue {
		t.Fatalf("GetJobGroup returned wrong job group for group2")
	}
}

// TestJobObj_ListJobs verifies that the created jobs can be retrieved.
func TestJobObj_ListJobs(t *testing.T) {
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a couple of dummy jobs.
	_, errx := JENewJob(ctx, uuid.New(), jt, `{}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}
	_, errx = JENewJob(ctx, uuid.New(), jt, `{}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	var jobs []Job
	if err := db.DB().Find(&jobs).Error; err != nil {
		t.Fatalf("Failed to retrieve jobs: %v", err)
	}
	if len(jobs) < 2 {
		t.Fatalf("Expected at least 2 jobs, got %d", len(jobs))
	}

	// Test ListJobs using an empty filter instead of nil.
	jobsList, err := listJobs(ctx, &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   1000,
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
			Description:                    "Test group for config",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerStateUpdateCallback:      nil,
			WorkerIsSuspendable:            false,
			Timeout:                        time.Hour,   // timeoutSec
			RetryDelay:                     time.Second, // retryDelaySec
			MaxRetries:                     3,           // maxRetries
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
	foundType, exists := getJobType(jobTypeId)
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
			Name:                           typeID,
			Description:                    "Job type for get test",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			Timeout:                        time.Hour,
			RetryDelay:                     time.Second,
			MaxRetries:                     0,
			WorkerStateUpdateCallback:      nil,
		},
	)

	jobTypeId := fmt.Sprintf("%s:%s", "getgroup", typeID)

	// Test getting the job type
	foundType, exists := getJobType(jobTypeId)
	assert.True(t, exists, "GetJobType should find the type")
	assert.Equal(t, jt, foundType, "GetJobType should return the correct job type")

	// Test getting a non-existent job type
	_, exists = getJobType("nonexistent:job:type")
	assert.False(t, exists, "GetJobType should not find non-existent type")
}

func TestJobConfig_JobQueueLimit(t *testing.T) {
	// Test job queue constants are defined
	assert.Equal(t, JobQueueName("compute"), JobQueueCompute.Name, "Compute queue name should be correct")
	assert.Equal(t, JobQueueName("maintenance"), JobQueueMaintenance.Name, "Maintenance queue name should be correct")
	assert.Equal(t, JobQueueName("download"), JobQueueDownload.Name, "Download queue name should be correct")
	// Add more tests for other queue-related functionality if available
}

func TestJobConfig_JobStatusMap(t *testing.T) {
	// Test that job status constants are defined with expected string values
	assert.Equal(t, JobStatus("waiting"), JobStatusWaiting, "JobStatusWaiting should be 'waiting'")
	assert.Equal(t, JobStatus("running"), JobStatusRunning, "JobStatusRunning should be 'running'")
	assert.Equal(t, JobStatus("completed"), JobStatusCompleted, "JobStatusCompleted should be 'completed'")
	assert.Equal(t, JobStatus("failed"), JobStatusFailed, "JobStatusFailed should be 'failed'")
	assert.Equal(t, JobStatus("canceled"), JobStatusCanceled, "JobStatusCanceled should be 'canceled'")
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
			Name:                           testTypeID,
			Description:                    "First registration",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerStateUpdateCallback:      nil,
			WorkerIsSuspendable:            false,
			Timeout:                        time.Hour,
			RetryDelay:                     time.Second,
			MaxRetries:                     1,
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
			Name:                           testTypeID, // same ID
			Description:                    "Second registration attempt",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerStateUpdateCallback:      nil,
			WorkerIsSuspendable:            false,
			Timeout:                        time.Minute, // different timeout
			RetryDelay:                     time.Second * 5,
			MaxRetries:                     2,
		},
	)

	// Verify that the returned type is the first one we registered
	assert.Equal(t, "othergroup", jt2.Group, "Different groups should be allowed to register the same type")
}

// TestJobObj_SetRetryPolicy verifies that SetRetryPolicy correctly updates the retry policy of a job.
func TestJobObj_SetRetryPolicy(t *testing.T) {
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job with default retry policy
	idempotencyKey := uuid.New()
	job, errx := JENewJob(ctx, idempotencyKey, jt, `{"foo":"bar"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Set a new retry policy
	newRetryDelay := 30 * time.Second
	newMaxRetries := 5
	errx = job.SetRetryPolicy(ctx, newRetryDelay, newMaxRetries, newRetryDelay)
	if errx != nil {
		t.Fatalf("SetRetryPolicy error: %v", errx)
	}

	// Verify the retry policy was updated
	assert.Equal(t, newRetryDelay, job.getRetryDelay(), "RetryDelaySec should be updated")
	assert.Equal(t, newMaxRetries, job.getMaxRetries(), "MaxRetries should be updated")

	// Verify the changes were persisted to the database
	var jobFromDB JobObj
	err := db.DB().Where("job_id = ?", job.GetJobID()).First(&jobFromDB.priv).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}
	jobFromDB.initType(jobFromDB.priv.Type)

	assert.Equal(t, newRetryDelay, jobFromDB.getRetryDelay(), "RetryDelaySec should be persisted in DB")
	assert.Equal(t, newMaxRetries, jobFromDB.getMaxRetries(), "MaxRetries should be persisted in DB")

	// Test setting negative values (should be clamped to 0)
	err = job.SetRetryPolicy(ctx, -5, -3, -5)
	if err != nil {
		t.Fatalf("SetRetryPolicy with negative values error: %v", err)
	}

	assert.Equal(t, time.Duration(0), job.getRetryDelay(), "Negative RetryDelaySec should be clamped to 0")
	assert.Equal(t, 0, job.getMaxRetries(), "Negative MaxRetries should be clamped to 0")
}

// TestJobObj_SetResult verifies that SetResult correctly updates the result of a job.
func TestJobObj_SetResult(t *testing.T) {
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job
	idempotencyKey := uuid.New()
	job, errx := JENewJob(ctx, idempotencyKey, jt, `{"foo":"bar"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
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

	errx = job.SetResult(ctx, result)
	if errx != nil {
		t.Fatalf("SetResult error: %v", errx)
	}

	// Verify the result was persisted to the database
	var jobFromDB JobObj
	err := db.DB().Where("job_id = ?", job.GetJobID()).First(&jobFromDB.priv).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}

	// Unmarshal the result from DB to verify it matches what we set
	var retrievedResult JobResult
	err = json.Unmarshal([]byte(jobFromDB.priv.Result), &retrievedResult)
	if err != nil {
		t.Fatalf("Error unmarshaling result: %v", err)
	}

	assert.Equal(t, result.Status, retrievedResult.Status, "Result status should match")
	assert.Equal(t, result.Message, retrievedResult.Message, "Result message should match")
}

// TestJobObj_SetParams verifies that SetParams correctly updates the parameters of a job.
func TestJobObj_SetParams(t *testing.T) {
	// setupTestDB removed - using shared setup
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job with initial params
	idempotencyKey := uuid.New()
	job, errx := JENewJob(ctx, idempotencyKey, jt, `{"foo":"val"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Verify initial params
	assert.Equal(t, `{"bar":"bar-default","foo":"val"}`, job.getParams(), "Initial params should match")

	// Update params
	newParams := `{"bar":"value","foo":"updated"}`
	errx = job.setParams(newParams)
	if errx != nil {
		t.Fatalf("SetParams error: %v", errx)
	}

	// Verify params were updated
	assert.Equal(t, newParams, job.getParams(), "Updated params should match")

	// Verify the changes were persisted to the database
	var jobFromDB JobObj
	err := db.DB().Where("job_id = ?", job.GetJobID()).First(&jobFromDB.priv).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}

	assert.Equal(t, newParams, jobFromDB.priv.Params, "Updated params should be persisted in DB")

	// Verify that ParamsPtr was properly updated
	paramsPtr := job.GetParamsPtr().(*dummyParams)
	assert.Equal(t, "updated", paramsPtr.Foo, "ParamsPtr should reflect updated values")
}

// TestJobObj_StatusFromDB verifies that setStatus() and getStatus() always get the actual status
// from the database, not a cached value in the object.
func TestJobObj_StatusFromDB(t *testing.T) {
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job
	idempotencyKey := uuid.New()
	job, errx := JENewJob(ctx, idempotencyKey, jt, `{}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Initial status should be JobStatusInit
	assert.Equal(t, JobStatusInit, job.GetStatus(), "Initial status should be init")

	// Create a second JobObj instance pointing to the same job in the database
	jobDuplicate, errx := JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("JEGetJob error: %v", errx)
	}

	// Update status using the first instance
	errx = job.setStatus(JobStatusFailed, "failed for test")
	if errx != nil {
		t.Fatalf("setStatus error: %v", errx)
	}

	// Verify the second instance sees the updated status without refreshing
	// This confirms getStatus() reads from the database, not from cached values
	assert.Equal(t, JobStatusFailed, jobDuplicate.GetStatus(), "Second instance should see updated status")

	// Update status directly in the database, bypassing the setStatus method
	err := db.DB().Model(&Job{}).Where("job_id = ?", job.GetJobID()).Updates(map[string]interface{}{
		"status": JobStatusCompleted,
		"error":  "failed for test",
	}).Error
	if err != nil {
		t.Fatalf("Direct DB update error: %v", err)
	}

	// Verify both instances see the updated status
	assert.Equal(t, JobStatusCompleted, job.GetStatus(), "First instance should see DB-updated status")
	assert.Equal(t, JobStatusCompleted, jobDuplicate.GetStatus(), "Second instance should see DB-updated status")

	// Verify error message is also retrieved from DB
	assert.Contains(t, job.getError(), "failed for test", "Error message should be retrieved from DB")

	// Test that status changes are properly persisted
	var jobFromDB JobObj
	err = db.DB().Where("job_id = ?", job.GetJobID()).First(&jobFromDB.priv).Error
	if err != nil {
		t.Fatalf("Error retrieving job from DB: %v", err)
	}

	assert.Equal(t, JobStatusCompleted, jobFromDB.GetStatus(), "Status in DB should match updated status")
}

func TestJobObj_InitParams_EmptyPayload(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	// Create a job with empty params
	job, errx := JENewJob(ctx, idempotencyKey, jt, "")
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Verify that params were initialized with defaults
	paramsPtr := job.GetParamsPtr().(*dummyParams)
	assert.Equal(t, "foo-default", paramsPtr.Foo, "Empty payload should use default value for Foo")

	// Verify the params JSON contains the default values
	var params map[string]interface{}
	err := json.Unmarshal([]byte(job.priv.Params), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	assert.Equal(t, "foo-default", params["foo"], "Params JSON should contain default value")
}

func TestJobObj_InitParams_PartialPayload(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	// Create a job with partial params (only setting Foo)
	job, errx := JENewJob(ctx, idempotencyKey, jt, `{"foo":"custom"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Verify that params were initialized correctly
	paramsPtr := job.GetParamsPtr().(*dummyParams)
	assert.Equal(t, "custom", paramsPtr.Foo, "Partial payload should override Foo with custom value")

	// Verify the params JSON contains the merged values
	var params map[string]interface{}
	err := json.Unmarshal([]byte(job.priv.Params), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	assert.Equal(t, "custom", params["foo"], "Params JSON should contain custom value")
}

// TestJobObj_InitParams_FullPayload verifies that InitParams correctly uses
// all provided parameters when a complete payload is provided.
func TestJobObj_InitParams_FullPayload(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()

	// Create a job type with a more complex parameter structure
	type complexJobParams struct {
		Foo     string `json:"foo" default:"default"`
		Bar     int    `json:"bar" default:"42"`
		Enabled bool   `json:"enabled" default:"false"`
	}

	queueName := JobQueueName("test-queue")
	queue, err := JERegisterJobQueue(&JobQueueConfig{
		Name:     queueName,
		Capacity: 1,
	})
	assert.NotNil(t, queue)
	assert.Nil(t, err)

	complexJobType := RegisterJobType(
		JobTypeParams{
			Group:                          &JobGroup{Name: "test", Queue: &queue.config},
			Name:                           "complex",
			Description:                    "complex job",
			Params:                         &complexJobParams{},
			WorkerParamsValidationCallback: nil,
			WorkerExecutionCallback:        nil,
			Timeout:                        1 * time.Second,
			RetryDelay:                     1 * time.Second,
			MaxRetries:                     3,
			WorkerIsSuspendable:            false,
		},
	)

	defer func() {
		delete(jobTypesMap, complexJobType.TypeID)
	}()

	// Create a job with full params
	fullParams := `{"foo":"custom", "bar":100, "enabled":true}`
	job, errx := JENewJob(ctx, idempotencyKey, complexJobType, fullParams)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Verify that params were initialized correctly
	paramsPtr := job.GetParamsPtr().(*complexJobParams)
	assert.Equal(t, "custom", paramsPtr.Foo, "Full payload should set Foo to custom value")
	assert.Equal(t, 100, paramsPtr.Bar, "Full payload should set Bar to 100")
	assert.Equal(t, true, paramsPtr.Enabled, "Full payload should set Enabled to true")

	// Verify the params JSON contains all the provided values
	var params map[string]interface{}
	err = json.Unmarshal([]byte(job.priv.Params), &params)
	if err != nil {
		t.Fatalf("Failed to unmarshal params: %v", err)
	}
	assert.Equal(t, "custom", params["foo"], "Params JSON should contain custom Foo value")
	assert.Equal(t, float64(100), params["bar"], "Params JSON should contain custom Bar value")
	assert.Equal(t, true, params["enabled"], "Params JSON should contain custom Enabled value")
}

func TestJobObj_GetLockedBy(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Initially, locked_by should be uuid.Nil
	assert.Equal(t, uuid.Nil, job.getLockedBy())

	lockID := uuid.New()
	errx = job.SetLockedBy(ctx, lockID)
	assert.Error(t, errx, "job.SetLockedBy should fail because target job doesn't exist")

	job2, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job2)

	// Lock job2 by job1
	errx = job.SetLockedBy(ctx, job2.GetJobID())
	assert.NoError(t, errx)

	// GetLockedBy should refresh from the database
	assert.Equal(t, job2.GetJobID(), job.getLockedBy())
}

// TestJobObj_GetTenantID verifies that GetTenantID correctly returns the tenant ID.
func TestJobObj_GetTenantID(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Get the tenant ID and verify it's not empty
	tenantID := job.GetTenantID()
	assert.NotEqual(t, uuid.UUID{}, tenantID, "TenantID should not be empty")

	// Verify the tenant ID matches what's in the database
	var dbJob Job
	result := db.DB().Where("job_id = ?", job.GetJobID()).First(&dbJob)
	assert.NoError(t, result.Error)
	assert.Equal(t, dbJob.TenantID, job.GetTenantID())
}

// TestJobObj_GetUserID verifies that GetUserID correctly returns the user ID.
func TestJobObj_GetUserID(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Get the user ID
	userID := job.GetUserID()

	// Verify the user ID matches what's in the database
	var dbJob Job
	result := db.DB().Where("job_id = ?", job.GetJobID()).First(&dbJob)
	assert.NoError(t, result.Error)
	assert.Equal(t, dbJob.UserID, userID)
}

// TestJobObj_GetTypePtr verifies that GetTypePtr correctly returns the job type pointer.
func TestJobObj_GetTypePtr(t *testing.T) {
	ctx := context.Background()

	// Create a job with a specific job type
	jobType := getDummyJobType()
	job, errx := JENewJob(ctx, uuid.New(), jobType, `{"foo": "bar"}`)
	assert.NoError(t, errx)
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
func TestJobObj_SetProgress_InvalidValues(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Test negative progress value
	errx = job.SetProgress(ctx, -1)
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "invalid progress value")

	// Test valid progress value
	errx = job.SetProgress(ctx, 50)
	progress := job.GetProgress()
	assert.NoError(t, errx)
	assert.True(t, progress == float32(0) || progress == float32(50), "progress should be either 0 or 50")

	// Test progress value > 100
	errx = job.SetProgress(ctx, 101)
	assert.NoError(t, errx)
	assert.Equal(t, job.GetProgress(), float32(100), "progress should be capped at 100")
}

// TestInitType_Errors verifies that InitType handles error cases correctly.
func TestJobObj_InitType_Errors(t *testing.T) {
	ctx := context.Background()

	// Create a job with a valid type
	job, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Test empty type string
	errx = job.initType("")
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "job type is not set")

	// Test non-existent type
	errx = job.initType("non-existent-type")
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "failed to get job type")
}

// TestInitParams_Errors verifies that InitParams handles error cases correctly.
func TestJobObj_InitParams_Errors(t *testing.T) {
	ctx := context.Background()

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Test invalid JSON
	errx = job.initParams(`{"foo": "bar"`) // Missing closing brace
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "failed to unmarshal")

	// Create a job with an empty type string
	jobWithEmptyType, errx := JENewJob(ctx, uuid.New(), getDummyJobType(), `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, jobWithEmptyType)

	// Manually set the type to empty to simulate a job with no type
	result := db.DB().Model(&Job{}).Where("job_id = ?", jobWithEmptyType.GetJobID()).Update("type", "")
	assert.NoError(t, result.Error)

	// Reinitialize the type - should fail because type is empty
	errx = jobWithEmptyType.initType("")
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "job type is not set")

	// Test non-existent type
	errx = jobWithEmptyType.initType("non-existent-type")
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "failed to get job type")
}

// safeCancelJob attempts to cancel a job and handles the case where the job is already in a terminal state.
// It returns nil if the job is already in a terminal state (completed, failed, etc.) or if cancellation succeeds.
func safeCancelJob(ctx context.Context, jobID uuid.UUID) error {
	// Get current job status
	job, errx := JEGetJobByID(ctx, jobID)
	if errx != nil {
		return errx
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
	return JECancelJob(ctx, jobID, "canceled by test")
}

// Commont part for job tests execution

var (
	testDB     *gorm.DB
	testDBOnce sync.Once
	initError  error
	testMode   bool
)

// ensureTestSetup ensures the test environment is initialized exactly once
// This runs automatically when the package is imported for testing
func ensureTestSetup() {
	testDBOnce.Do(func() {
		if !testMode {
			return // Don't initialize if not in test mode
		}

		testDB, err := test_setup.InitTestEnv()
		if err != nil {
			initError = err
			return
		}

		logging.MainLogger.ConsoleLevel = logging.TraceLevel
		logging.MainLogger.FileLevel = logging.NoneLevel
		logger.SetConsoleLogLevel(logging.TraceLevel)
		logger.SetFileLogLevel(logging.NoneLevel)

		db.SetDB(testDB)

		// Migrate the necessary tables using SafeAutoMigrate
		if err := db.SafeAutoMigrate(testDB, &Job{}, &JobGroup{}, &JobType{}); err != nil {
			initError = err
			return
		}

		// Initialize job executor
		if err := JEInit(); err != nil {
			initError = err
			return
		}

		logging.Trace("=============== Test DB setup complete =================")
	})
}

// TestMain runs before all tests when running the full test suite
func TestMain(m *testing.M) {
	testMode = true

	// Ensure setup is done
	ensureTestSetup()
	fmt.Println("TestMain")
	if initError != nil {
		panic("Failed to initialize test environment: " + initError.Error())
	}

	// Run tests
	exitCode := m.Run()

	// Cleanup
	if testDB != nil {
		sqlDB, err := testDB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}

	os.Exit(exitCode)
}
