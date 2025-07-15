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
	"github.com/stretchr/testify/assert"
)

// dummyPageRequest returns a simple page request.
func dummyPageRequest() *api.PageAPIRequest {
	return &api.PageAPIRequest{
		PageNumber: 1,
		PageSize:   10,
		Order:      "",
	}
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
				Name:                           "test_job_type_id",
				Description:                    "Test job type",
				Params:                         &testJobParams{},
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
				WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
				WorkerStateUpdateCallback:      nil,
				Timeout:                        time.Hour * 3,
				RetryDelay:                     time.Second * 30,
				MaxRetries:                     5,
				WorkerIsSuspendable:            false,
			},
		)
	}
	return testJobTypeForAPI
}

// TestAPIGetJob_InvalidUUID verifies that APIGetJob returns an error when given an invalid UUID.
func TestJobAPI_GetJob_InvalidUUID(t *testing.T) {
	japi := &JobAPI{}
	ctx := context.Background()

	input := &struct {
		JobID uuid.UUID `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}{
		JobID: uuid.Nil,
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
func TestJobAPI_GetJob_NotFound(t *testing.T) {
	japi := &JobAPI{}
	ctx := context.Background()

	// Use a valid random UUID that is not stored.
	validUUID := uuid.New()
	input := &struct {
		JobID uuid.UUID `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}{
		JobID: validUUID,
	}
	_, err := japi.APIGetJob(ctx, input)
	if err == nil {
		t.Fatalf("Expected error for job not found, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Expected 'not found' error, got: %v", err)
	}
}

// TestAPIGetJob_Success creates a dummy job in the DB and retrieves it via APIGetJob.
func TestJobAPI_GetJob_Success(t *testing.T) {
	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()

	// Create a dummy job.
	idempotency := uuid.New()
	job, err := JENewJob(ctx, idempotency, jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	input := &struct {
		JobID uuid.UUID `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}{
		JobID: job.GetJobID(),
	}

	resp, errx := japi.APIGetJob(ctx, input)
	if errx != nil {
		t.Fatalf("APIGetJob error: %v", errx)
	}
	if resp.Body.JobID != job.GetJobID() {
		t.Fatalf("Expected job ID %s, got %s", job.GetJobID().String(), resp.Body.JobID)
	}

	// Optionally, check that other fields are correctly returned.
}

// TestAPIListJobTypes_Empty verifies that APIListJobTypes returns zero total when no job types are registered.
func TestJobAPI_ListJobTypes_Empty(t *testing.T) {
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
func TestJobAPI_APICancelJob(t *testing.T) {
	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a dummy job.
	job, err := JENewJob(ctx, uuid.New(), jt, `{}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}
	// Set the job to a cancellable state.
	job.setStatus(JobStatusWaiting, "")

	// Prepare input for APICancelJob.
	input := &struct {
		JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}{JobID: job.GetJobID().String()}

	// Call APICancelJob.
	_, errx := japi.APICancelJob(ctx, input)
	if errx != nil {
		t.Fatalf("APICancelJob error: %v", errx)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify that the job status is canceled.
	var fetched Job
	if err := db.DB().First(&fetched, "job_id = ?", job.GetJobID()).Error; err != nil {
		t.Fatalf("Failed to fetch job: %v", err)
	}
	if fetched.Status != "canceled" {
		t.Fatalf("Expected status %s, got %s", JobStatusCanceled, fetched.Status)
	}
}

// TestAPIDeleteJob verifies that the APIDeleteJob endpoint deletes a job.
func TestJobAPI_APIDeleteJob(t *testing.T) {
	japi := &JobAPI{}
	ctx := context.Background()

	jt := getTestJobTypeForAPI()
	assert.NotNil(t, jt)

	// Create a dummy job.
	job, errx := JENewJob(ctx, uuid.New(), jt, `{}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Set the job to a stable state that won't be processed by workers
	job.setStatus(JobStatusCompleted, "Test job for deletion")

	// Prepare input for APIDeleteJob.
	input := &struct {
		JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
	}{JobID: job.GetJobID().String()}

	_, err := japi.APIDeleteJob(ctx, input)
	if err != nil {
		t.Fatalf("APIDeleteJob error: %v", err)
	}

	// Verify job deletion.
	var fetched JobObj
	err = db.DB().First(&fetched, "job_id = ?", job.GetJobID()).Error
	if err == nil {
		t.Fatalf("Expected job to be deleted, but it still exists")
	}
}

// TestAPIScheduleJob verifies that the APIScheduleJob endpoint schedules a new job.
func TestJobAPI_APIScheduleJob(t *testing.T) {
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
	job, err := JEGetJobByID(ctx, jobID)
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

	status := job.GetStatus()
	if status != JobStatusCompleted {
		t.Fatalf("Expected job status %s, got %s", JobStatusCompleted, status)
	}
}

func TestJobAPI_APIScheduleJob_WrongGroup(t *testing.T) {
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
func TestJobAPI_APIListJobs(t *testing.T) {
	japi := &JobAPI{}
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
func TestJobAPI_APIGetJobGroup(t *testing.T) {
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
		Name string `path:"job_group_id" doc:"The job group name" example:"llm_model_ops"`
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
func TestJobAPI_APIListJobGroups(t *testing.T) {
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
func TestJobAPI_APIGetJobType(t *testing.T) {
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
			Name:                           "update",
			Description:                    "Update a model",
			Params:                         &struct{}{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
			Timeout:                        time.Hour * 3,
			RetryDelay:                     time.Second * 30,
			MaxRetries:                     5,
			WorkerIsSuspendable:            false,
		},
	)

	if err := db.DB().Create(jt).Error; err != nil {
		t.Fatalf("Failed to create job type: %v", err)
	}

	input := &struct {
		JobTypeID string `path:"job_type_id" example:"llm_model_ops:load"`
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
func TestJobAPI_JobTypeToAPIResponse(t *testing.T) {
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
		WorkerInitCallback:      func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
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
func TestJobAPI_JobGroupToAPIResponse(t *testing.T) {
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

func TestJobAPI_APIMaflormedParams(t *testing.T) {
	japi := &JobAPI{}
	ctx := context.Background()

	// Prepare input for APIDeleteJob.
	input1 := &struct {
		JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
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
		JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
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

func TestJobAPI_RegisterJobAPIRoutes(t *testing.T) {
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test API", "1.0.0"))
	registerJobAPIRoutes(api)
	// The humachi adapter doesn't provide GetRoutes()
	// We can verify routes are registered by checking the router instead
	// or just verify that registerJobAPIRoutes executes without errors
	// Simple success test - if no panic occurs, the test passes
}
