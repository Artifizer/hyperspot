package job

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/errorx"
)

type JobAPI struct{}

type JobAPIResponseItem Job
type JobTypeAPIResponseItem JobType
type JobGroupAPIResponseItem JobGroup

type JobAPIResponse struct {
	Body JobAPIResponseItem `json:"body"`
}

type JobTypeAPIResponse struct {
	Body JobTypeAPIResponseItem `json:"body"`
}

type JobGroupAPIResponse struct {
	Body JobGroupAPIResponseItem `json:"body"`
}

type ListJobsAPIRequest struct {
	api.PageAPIRequest
	Status string `query:"status" doc:"Filter by status, comma separated list of statuses is supported"`
}

func (j *JobAPI) JobToAPIResponse(job *JobObj) *JobAPIResponse {
	return &JobAPIResponse{
		Body: JobAPIResponseItem(job.priv),
	}
}

func (j *JobAPI) JobTypeToAPIResponse(jobType *JobType) *JobTypeAPIResponse {
	return &JobTypeAPIResponse{
		Body: JobTypeAPIResponseItem(*jobType),
	}
}

func (j *JobAPI) JobGroupToAPIResponse(jobGroup *JobGroup) *JobGroupAPIResponse {
	return &JobGroupAPIResponse{
		Body: JobGroupAPIResponseItem(*jobGroup),
	}
}

type ListJobsAPIResponse struct {
	Body struct {
		api.PageAPIResponse
		Jobs []JobAPIResponseItem `json:"jobs"`
	} `json:"body"`
}

type ListJobTypesAPIResponse struct {
	Body struct {
		api.PageAPIResponse
		JobTypes []JobTypeAPIResponseItem `json:"job_types"`
	} `json:"body"`
}

type ListJobGroupsAPIRequest struct {
	api.PageAPIRequest
}

type ListJobGroupsAPIResponse struct {
	Body struct {
		api.PageAPIResponse
		JobGroups []JobGroupAPIResponseItem `json:"job_groups"`
	} `json:"body"`
}

func (j *JobAPI) APIListJobTypes(ctx context.Context, request *api.PageAPIRequest) (*ListJobTypesAPIResponse, error) {
	resp := &ListJobTypesAPIResponse{}

	err := api.PageAPIInitResponse(request, &resp.Body.PageAPIResponse)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	jobTypes := getJobTypes(ctx, request)
	for _, jobType := range jobTypes {
		resp.Body.JobTypes = append(resp.Body.JobTypes, JobTypeAPIResponseItem(*jobType))
	}

	resp.Body.PageAPIResponse.Total = len(jobTypes)

	return resp, nil
}

func (j *JobAPI) APIGetJobType(ctx context.Context, input *struct {
	JobTypeID string `path:"job_type_id" example:"llm_model_ops:load"`
}) (*JobTypeAPIResponse, error) {
	jobType, ok := getJobType(input.JobTypeID)
	if !ok {
		return nil, huma.Error404NotFound("Job type not found")
	}

	return j.JobTypeToAPIResponse(jobType), nil
}

func (j *JobAPI) APIListJobGroups(ctx context.Context, input *ListJobGroupsAPIRequest) (*ListJobGroupsAPIResponse, error) {
	resp := &ListJobGroupsAPIResponse{}
	err := api.PageAPIInitResponse(&input.PageAPIRequest, &resp.Body.PageAPIResponse)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	jobGroups := getJobGroups(ctx, &input.PageAPIRequest)
	for _, jobGroup := range jobGroups {
		resp.Body.JobGroups = append(resp.Body.JobGroups, JobGroupAPIResponseItem(*jobGroup))
	}
	resp.Body.PageAPIResponse.Total = len(jobGroups)

	return resp, nil
}

func (j *JobAPI) APIGetJobGroup(ctx context.Context, input *struct {
	Name string `path:"job_group_id" doc:"The job group name" example:"llm_model_ops"`
}) (*JobGroupAPIResponse, error) {
	jobGroup, ok := getJobGroup(input.Name)
	if !ok {
		return nil, huma.Error404NotFound("Job group not found")
	}

	return j.JobGroupToAPIResponse(jobGroup), nil
}

func (j *JobAPI) APIListJobs(ctx context.Context, input *ListJobsAPIRequest) (*ListJobsAPIResponse, error) {
	// Build response
	resp := &ListJobsAPIResponse{}

	if input.PageAPIRequest.Order == "" {
		input.PageAPIRequest.Order = "-scheduled_at"
	}

	err := api.PageAPIInitResponse(&input.PageAPIRequest, &resp.Body.PageAPIResponse)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	jobs, err := listJobs(ctx, &input.PageAPIRequest, input.Status)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}

	resp.Body.Jobs = make([]JobAPIResponseItem, 0)

	for _, job := range jobs {
		resp.Body.Jobs = append(resp.Body.Jobs, JobAPIResponseItem(job.priv))
	}

	resp.Body.Total = len(jobs)

	return resp, nil
}

func (j *JobAPI) APIScheduleJob(ctx context.Context, input *struct {
	Body struct {
		Type           string      `json:"type"`
		IdempotencyKey string      `json:"idempotency_key"`
		Params         interface{} `json:"params"`
	}
}) (*JobAPIResponse, error) {
	jobType, ok := getJobType(input.Body.Type)
	if !ok {
		jobTypes := getJobTypes(ctx, nil)
		jobTypeNames := make([]string, len(jobTypes))
		for i, jt := range jobTypes {
			jobTypeNames[i] = jt.TypeID
		}
		return nil, huma.Error400BadRequest(fmt.Sprintf(
			"Unsupported job type: '%s', supported types are: %s",
			input.Body.Type,
			strings.Join(jobTypeNames, ", "),
		))
	}

	idempotencyKey, err := uuid.Parse(input.Body.IdempotencyKey)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid idempotency key")
	}

	job, err := JEGetJobByID(ctx, idempotencyKey)
	if err == nil {
		if job.priv.IdempotencyKey != idempotencyKey {
			return nil, huma.Error409Conflict("Job with given ID key already exists")
		}
		return j.JobToAPIResponse(job), nil
	}

	paramsBytes, err := json.Marshal(input.Body.Params)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid params format")
	}

	job, errx := JENewJob(ctx, idempotencyKey, jobType, string(paramsBytes))
	if errx != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("Invalid data format: %s", errx))
	}

	if errx := JEScheduleJob(ctx, job); errx != nil {
		return nil, errx
	}

	return j.JobToAPIResponse(job), nil
}

func (j *JobAPI) APIDeleteJob(ctx context.Context, input *struct {
	JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}) (*struct{}, error) {
	uuid, err := uuid.Parse(input.JobID)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid job ID")
	}

	errx := jeDeleteJob(ctx, uuid, "deleted by API")
	if errx != nil {
		switch errx.(type) {
		case *errorx.ErrNotFound:
			return &struct{}{}, nil
		default:
			return nil, errx.HumaError()
		}
	}

	return &struct{}{}, nil
}

func (j *JobAPI) APICancelJob(ctx context.Context, input *struct {
	JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}) (*JobAPIResponse, error) {
	uuid, err := uuid.Parse(input.JobID)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid job ID")
	}

	job, errx := JEGetJobByID(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	if job.priv.Status == JobStatusFailed || job.priv.Status == JobStatusCompleted {
		job, errx = JEGetJobByID(ctx, uuid)
		if errx != nil {
			return nil, errx.HumaError()
		}
		return j.JobToAPIResponse(job), nil
	}

	if job.priv.Status != JobStatusWaiting && job.priv.Status != JobStatusRunning {
		return j.JobToAPIResponse(job), nil
	}

	err = JECancelJob(ctx, uuid, "canceled by API")
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to cancel job: %s", err))
	}

	// Re-read job after cancel
	job, errx = JEGetJobByID(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	return j.JobToAPIResponse(job), nil
}

func (j *JobAPI) APISuspendJob(ctx context.Context, input *struct {
	JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}) (*JobAPIResponse, error) {
	uuid, err := uuid.Parse(input.JobID)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid job ID")
	}

	job, errx := JEGetJobByID(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	// Check if the job can be suspended (only initializing, waiting, suspended and running jobs can be suspended)
	if job.priv.Status != JobStatusInit && job.priv.Status != JobStatusWaiting &&
		job.priv.Status != JobStatusRunning && job.priv.Status != JobStatusSuspended {
		return nil, huma.Error409Conflict(fmt.Sprintf("Job with status '%s' cannot be suspended", job.priv.Status))
	}

	// Suspend the job
	errx = jeSuspendJob(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	// Re-read job after suspend
	job, errx = JEGetJobByID(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	return j.JobToAPIResponse(job), nil
}

func (j *JobAPI) APIResumeJob(ctx context.Context, input *struct {
	JobID string `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}) (*JobAPIResponse, error) {
	uuid, err := uuid.Parse(input.JobID)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid job ID")
	}

	job, errx := JEGetJobByID(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	// Check if the job can be resumed (only suspended jobs can be resumed)
	if job.priv.Status != JobStatusSuspended {
		return nil, huma.Error409Conflict(fmt.Sprintf("Job with status '%s' cannot be resumed", job.priv.Status))
	}

	// Resume the job
	errx = jeResumeJob(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	// Re-read job after resume
	job, errx = JEGetJobByID(ctx, uuid)
	if errx != nil {
		return nil, errx.HumaError()
	}

	return j.JobToAPIResponse(job), nil
}

func (j *JobAPI) APIGetJob(ctx context.Context, input *struct {
	JobID uuid.UUID `path:"job_id" doc:"The job UUID" example:"550e8400-e29b-41d4-a716-446655440000"`
}) (*JobAPIResponse, error) {
	if input.JobID == uuid.Nil {
		return nil, huma.Error400BadRequest("Invalid job ID")
	}

	job, errx := JEGetJobByID(ctx, input.JobID)
	if errx != nil {
		return nil, errx.HumaError()
	}
	return j.JobToAPIResponse(job), nil
}

func registerJobAPIRoutes(humaApi huma.API) {
	j := &JobAPI{}

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-jobs",
		Method:      http.MethodGet,
		Path:        "/jobs",
		Summary:     "List all jobs",
		Tags:        []string{"Jobs"},
	}, j.APIListJobs)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:   "schedule-job",
		Method:        http.MethodPost,
		Path:          "/jobs",
		Summary:       "Schedule a new job",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"Jobs"},
	}, j.APIScheduleJob)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:   "suspend-job",
		Method:        http.MethodPost,
		Path:          "/jobs/{job_id}/suspend",
		Summary:       "Suspend a job",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"Jobs"},
	}, j.APISuspendJob)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:   "resume-job",
		Method:        http.MethodPost,
		Path:          "/jobs/{job_id}/resume",
		Summary:       "Resume a job",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"Jobs"},
	}, j.APIResumeJob)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:   "cancel-job",
		Method:        http.MethodPost,
		Path:          "/jobs/{job_id}/cancel",
		Summary:       "Cancel a job",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"Jobs"},
	}, j.APICancelJob)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-job",
		Method:      http.MethodGet,
		Path:        "/jobs/{job_id}",
		Summary:     "Get a job",
		Tags:        []string{"Jobs"},
	}, j.APIGetJob)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID:   "delete-job",
		Method:        http.MethodDelete,
		Path:          "/jobs/{job_id}",
		Summary:       "Delete a job",
		DefaultStatus: http.StatusAccepted,
		Tags:          []string{"Jobs"},
	}, j.APIDeleteJob)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-job-types",
		Method:      http.MethodGet,
		Path:        "/job_types",
		Summary:     "List all job types",
		Tags:        []string{"Jobs"},
	}, j.APIListJobTypes)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-job-type",
		Method:      http.MethodGet,
		Path:        "/job_types/{job_type_id}",
		Summary:     "Get a job type",
		Tags:        []string{"Jobs"},
	}, j.APIGetJobType)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-job-groups",
		Method:      http.MethodGet,
		Path:        "/job_groups",
		Summary:     "List all job groups",
		Tags:        []string{"Jobs"},
	}, j.APIListJobGroups)

	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-job-group",
		Method:      http.MethodGet,
		Path:        "/job_groups/{job_group_id}",
		Summary:     "Get a job group",
		Tags:        []string{"Jobs"},
	}, j.APIGetJobGroup)
}

// NewJobAPI creates and returns a new instance of JobAPI.
func NewJobAPI() *JobAPI {
	return &JobAPI{}
}
