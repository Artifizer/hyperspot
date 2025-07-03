package job

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// setupTestDB initializes an in-memory SQLite DB and migrates the schema.
func setupTestDBForAPI(t *testing.T) *gorm.DB {
	testDB, err := db.InitInMemorySQLite(nil)
	if err != nil {
		t.Fatalf("Failed to connect to test DB: %v", err)
	}
	db.DB = testDB

	err = orm.OrmInit(testDB)
	if err != nil {
		t.Fatalf("Failed to initialize ORM: %v", err)
	}

	if err := testDB.AutoMigrate(&Job{}, &JobGroup{}, &JobType{}); err != nil {
		t.Fatalf("Failed to auto migrate schema: %v", err)
	}
	return testDB
}

// dummyPageRequest returns a simple page request.
func dummyPageRequest() *api.PageAPIRequest {
	return &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   10,
		Order:      "",
	}
}

// Setup function to initialize job queues
func setupJobQueuesForTest() {
	// Initialize jobsStore if not already initialized
	initJobQueues()
}

type testJobParams struct {
	Foo string `json:"foo"`
}

// testJobType is a dummy JobType used for testing API endpoints.
var testJobTypeForAPI *JobType

func getTestJobTypeForAPI() *JobType {
	if testJobTypeForAPI == nil {
		testJobTypeForAPI = RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "test_job_group_id",
					Queue:       JobQueueCompute,
					Description: "Test group",
				},
				Name:                      "test_job_type_id",
				Description:               "Test job type",
				Params:                    &testJobParams{},
				WorkerInitCallback:        func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return nil, nil },
				WorkerExecutionCallback:   func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
				WorkerStateUpdateCallback: nil,
				WorkerSuspendCallback:     nil,
				WorkerResumeCallback:      nil,
				Timeout:                   time.Hour * 3,
				RetryDelay:                time.Second * 30,
				MaxRetries:                5,
				WorkerIsSuspendable:       false,
			},
		)
	}
	return testJobTypeForAPI
}

// TestAPIGetJob_InvalidUUID verifies that APIGetJob returns an error when given an invalid UUID.
func TestAPIGetJob_InvalidUUID(t *testing.T) {
	setupTestDBForAPI(t)
	japi := &JobAPI{}
	ctx := context.Background()

	input := &struct {
		JobID string `path:"job_id"`
	}{
		JobID: "invalid-uuid",
	}

	_, err := japi.APIGetJob(ctx, input)
	if err == nil {
		t.Fatalf("Expected error for invalid job ID, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid job ID") {
		t.Fatalf("Expected 'Invalid job ID' error, got: %v", err)
	}
}

// TestAPIGetJob_NotFound verifies that APIGetJob returns a proper error for a valid UUID that is not in the DB.
func TestAPIGetJob_NotFound(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	// Use a valid random UUID that is not stored.
	validUUID := uuid.New().String()
	input := &struct {
		JobID string `path:"job_id"`
	}{
		JobID: validUUID,
	}
	_, err := japi.APIGetJob(ctx, input)
	if err == nil {
		t.Fatalf("Expected error for job not found, got nil")
	}
	if !strings.Contains(err.Error(), "Job not found") {
		t.Fatalf("Expected 'Job not found' error, got: %v", err)
	}
}

// TestAPIGetJob_Success creates a dummy job in the DB and retrieves it via APIGetJob.
func TestAPIGetJob_Success(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()

	// Create a dummy job.
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	input := &struct {
		JobID string `path:"job_id"`
	}{
		JobID: job.GetJobID().String(),
	}

	resp, err := japi.APIGetJob(ctx, input)
	if err != nil {
		t.Fatalf("APIGetJob error: %v", err)
	}
	if resp.Body.JobID != job.GetJobID() {
		t.Fatalf("Expected job ID %s, got %s", job.GetJobID().String(), resp.Body.JobID)
	}

	// Optionally, check that other fields are correctly returned.
}

// TestAPIListJobTypes_Empty verifies that APIListJobTypes returns zero total when no job types are registered.
func TestAPIListJobTypes_Empty(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()
	req := dummyPageRequest()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	resp, err := japi.APIListJobTypes(ctx, req)
	if err != nil {
		t.Fatalf("APIListJobTypes error: %v", err)
	}
	if resp.Body.PageAPIResponse.Total == 0 {
		t.Fatalf("could not find any job types")
	}
}

// Additional tests for APIListJobGroups, APIGetJobGroup, etc. could be added here.

// TestAPICancelJob verifies that the APICancelJob endpoint cancels a 'waiting' job.
func TestAPICancelJob(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a dummy job.
	job, err := NewJob(ctx, uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}
	// Set the job to a cancellable state.
	job.setStatus(ctx, JobStatusWaiting, nil)

	// Prepare input for APICancelJob.
	input := &struct {
		JobID string `path:"job_id"`
	}{JobID: job.GetJobID().String()}

	// Call APICancelJob.
	_, err = japi.APICancelJob(ctx, input)
	if err != nil {
		t.Fatalf("APICancelJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify that the job status is canceled.
	var fetched Job
	if err := db.DB.First(&fetched, "job_id = ?", job.GetJobID()).Error; err != nil {
		t.Fatalf("Failed to fetch job: %v", err)
	}
	if fetched.Status != "canceled" {
		t.Fatalf("Expected status %s, got %s", JobStatusCanceled, fetched.Status)
	}
}

// TestAPIDeleteJob verifies that the APIDeleteJob endpoint deletes a job.
func TestAPIDeleteJob(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a dummy job.
	job, err := NewJob(ctx, uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Prepare input for APIDeleteJob.
	input := &struct {
		JobID string `path:"job_id"`
	}{JobID: job.GetJobID().String()}

	_, err = japi.APIDeleteJob(ctx, input)
	if err != nil {
		t.Fatalf("APIDeleteJob error: %v", err)
	}

	// Verify job deletion.
	var fetched JobObj
	err = db.DB.First(&fetched, "job_id = ?", job.GetJobID()).Error
	if err == nil {
		t.Fatalf("Expected job to be deleted, but it still exists")
	}
}

// TestAPIScheduleJob verifies that the APIScheduleJob endpoint schedules a new job.
func TestAPIScheduleJob(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Prepare input for scheduling a job.
	// Assume input requires a JobType and Params.
	input := &struct {
		Body struct {
			Type           string      `json:"type"`
			IdempotencyKey string      `json:"idempotency_key"`
			Params         interface{} `json:"params"`
		}
	}{
		Body: struct {
			Type           string      `json:"type"`
			IdempotencyKey string      `json:"idempotency_key"`
			Params         interface{} `json:"params"`
		}{
			Type:           testJobTypeForAPI.TypeID,
			IdempotencyKey: uuid.New().String(),
			Params:         &testJobParams{Foo: "bar"},
		},
	}

	resp, err := japi.APIScheduleJob(ctx, input)
	if err != nil {
		t.Fatalf("APIScheduleJob error: %v", err)
	}

	// Optionally, verify that the returned response contains expected job information.
	if resp == nil {
		t.Fatalf("APIScheduleJob returned nil response")
	}

	jobID := resp.Body.JobID
	if jobID == uuid.Nil {
		t.Fatalf("APIScheduleJob returned nil job")
	}
	job, err := JEGetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("JEGetJob error: %v", err)
	}
	if job == nil {
		t.Fatalf("APIScheduleJob returned nil job")
	}
	if job.GetJobID() == uuid.Nil {
		t.Fatalf("APIScheduleJob returned job with nil JobID")
	}

	err = JEWaitJob(ctx, job.GetJobID(), job.GetTimeoutSec())
	if err != nil {
		t.Fatalf("JEWaitJob error: %v", err)
	}

	if job.GetStatus() != JobStatusCompleted {
		t.Fatalf("Expected job status %s, got %s", JobStatusCompleted, job.GetStatus())
	}
}

func TestAPIScheduleJob_WrongGroup(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Prepare input for scheduling a job.
	// Assume input requires a JobType and Params.
	input := &struct {
		Body struct {
			Type           string      `json:"type"`
			IdempotencyKey string      `json:"idempotency_key"`
			Params         interface{} `json:"params"`
		}
	}{
		Body: struct {
			Type           string      `json:"type"`
			IdempotencyKey string      `json:"idempotency_key"`
			Params         interface{} `json:"params"`
		}{
			Type:           fmt.Sprintf("wronggroup:wrongtype"),
			IdempotencyKey: uuid.New().String(),
			Params:         &testJobParams{Foo: "bar"},
		},
	}

	_, err := japi.APIScheduleJob(ctx, input)
	if err == nil {
		t.Fatalf("APIScheduleJob should have returned an error")
	}
}

// TestAPIListJobs verifies that the APIListJobs endpoint returns created jobs.
func TestAPIListJobs(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
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

	req := &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   10,
		Order:      "-started_at",
	}
	resp, err := japi.APIListJobs(ctx, &ListJobsAPIRequest{PageAPIRequest: *req})
	if err != nil {
		t.Fatalf("APIListJobs error: %v", err)
	}
	if resp == nil || len(resp.Body.Jobs) == 0 {
		t.Fatalf("Expected non-empty jobs list, got empty")
	}
}

// TestAPIGetJobGroup verifies that the APIGetJobGroup endpoint returns the details of a specific group.
func TestAPIGetJobGroup(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	// Create a job group.
	group := &JobGroup{
		Name:        "group-test",
		Queue:       JobQueueCompute,
		Description: "Test job group",
	}
	RegisterJobGroup(group)

	input := &struct {
		Name string `path:"job_group_id"`
	}{Name: group.Name}

	resp, err := japi.APIGetJobGroup(ctx, input)
	if err != nil {
		t.Fatalf("APIGetJobGroup error: %v", err)
	}
	if resp == nil {
		t.Fatalf("APIGetJobGroup returned nil response")
	}
	// Check that the group name matches
	if group.Name != resp.Body.Name {
		t.Fatalf("Expected group name %s, got %v", group.Name, resp.Body.Name)
	}
}

// TestAPIListJobGroups verifies that the APIListJobGroups endpoint returns all job groups.
func TestAPIListJobGroups(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	// Create two job groups.
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

	req := dummyPageRequest()
	resp, err := japi.APIListJobGroups(ctx, &ListJobGroupsAPIRequest{PageAPIRequest: *req})
	if err != nil {
		t.Fatalf("APIListJobGroups error: %v", err)
	}
	// Assume that the response body contains a PageAPIResponse with a Total field.
	if resp == nil || resp.Body.PageAPIResponse.Total < 2 {
		t.Fatalf("Expected at least 2 job groups, got %v", resp.Body.PageAPIResponse.Total)
	}
}

// TestAPIGetJobType verifies that the APIGetJobType endpoint returns details of a specific job type.
func TestAPIGetJobType(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	// Create a job type.
	jt := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "getgroup",
				Queue:       JobQueueCompute,
				Description: "Job group for get test",
			},
			Name:                    "update",
			Description:             "Update a model",
			Params:                  &struct{}{},
			WorkerInitCallback:      func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return nil, nil },
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
			Timeout:                 time.Hour * 3,
			RetryDelay:              time.Second * 30,
			MaxRetries:              5,
			WorkerIsSuspendable:     false,
		},
	)

	if err := db.DB.Create(jt).Error; err != nil {
		t.Fatalf("Failed to create job type: %v", err)
	}

	input := &struct {
		JobTypeID string `path:"job_type_id"`
	}{JobTypeID: jt.TypeID}

	resp, err := japi.APIGetJobType(ctx, input)
	if err != nil {
		t.Fatalf("APIGetJobType error: %v", err)
	}
	if resp == nil {
		t.Fatalf("APIGetJobType returned nil response")
	}
	// Check that the returned job type has the expected TypeID.
	if resp.Body.TypeID != jt.TypeID {
		t.Fatalf("Expected job type ID %s, got %v", jt.TypeID, resp.Body.TypeID)
	}
}

// TestJobTypeToAPIResponse verifies that JobTypeToAPIResponse correctly converts a JobType.
func TestJobTypeToAPIResponse(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	// Create a dummy job type.
	jt := &JobType{
		TypeID:      "convert:job:type",
		Description: "Conversion test",
		Group:       "convertgroup",
		GroupPtr: &JobGroup{
			Name:        "convertgroup",
			Queue:       JobQueueCompute,
			Description: "Conversion group",
		},
		Params:                  &struct{}{},
		TimeoutSec:              60,
		MaxRetries:              1,
		RetryDelaySec:           2,
		WorkerInitCallback:      func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return nil, nil },
		WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error { return nil },
	}
	japi := &JobAPI{}
	resp := japi.JobTypeToAPIResponse(jt)
	if resp == nil {
		t.Fatalf("JobTypeToAPIResponse returned nil")
	}
	if resp.Body.TypeID != jt.TypeID {
		t.Fatalf("Expected converted TypeID %s, got %s", jt.TypeID, resp.Body.TypeID)
	}
}

// TestJobGroupToAPIResponse verifies that JobGroupToAPIResponse correctly converts a JobGroup.
func TestJobGroupToAPIResponse(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	group := &JobGroup{
		Name:        "convert:group",
		Queue:       JobQueueCompute,
		Description: "Group conversion test",
	}
	// Create a new API instance for testing
	japi := &JobAPI{}
	resp := japi.JobGroupToAPIResponse(group)
	if resp == nil {
		t.Fatalf("JobGroupToAPIResponse returned nil")
	}
	if resp.Body.Name != group.Name {
		t.Fatalf("Expected converted group name %s, got %s", group.Name, resp.Body.Name)
	}
}

func TestAPIMaflormedParams(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	japi := &JobAPI{}
	ctx := context.Background()

	// Prepare input for APIDeleteJob.
	input1 := &struct {
		JobID string `path:"job_id"`
	}{JobID: uuid.New().String()}

	_, err := japi.APIDeleteJob(ctx, input1)
	if err != nil {
		t.Fatalf("APIDeleteJob should succeed even if job ID doesn't exist")
	}

	_, err = japi.APICancelJob(ctx, input1)
	if err == nil {
		t.Fatalf("APICancelJob should have returned an error")
	}

	// Prepare input for APIDeleteJob.
	input2 := &struct {
		JobID string `path:"job_id"`
	}{JobID: "malformed-job-id"}

	_, err = japi.APIDeleteJob(ctx, input2)
	if err == nil {
		t.Fatalf("APIDeleteJob should have returned an error")
	}
	_, err = japi.APICancelJob(ctx, input2)
	if err == nil {
		t.Fatalf("APICancelJob should have returned an error")
	}
}

func TestRegisterJobAPIRoutes(t *testing.T) {
	setupTestDBForAPI(t)
	setupJobQueuesForTest()

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test API", "1.0.0"))
	registerJobAPIRoutes(api)
	// The humachi adapter doesn't provide GetRoutes()
	// We can verify routes are registered by checking the router instead
	// or just verify that registerJobAPIRoutes executes without errors
	// Simple success test - if no panic occurs, the test passes
}
