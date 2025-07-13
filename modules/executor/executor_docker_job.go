package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/modules/job"
	jobs "github.com/hypernetix/hyperspot/modules/job"
)

// DockerImageDownloadJobParams represents parameters for Docker image download jobs
type DockerImageDownloadJobParams struct {
	TenantID        uuid.UUID       `json:"tenant_id"`
	CodeExecutor    string          `json:"code_executor"`
	CodeExecutorPtr *DockerExecutor `json:"-" gorm:"-"`
}

// DockerImageDownloadJobWorker performs work for Docker image download jobs
func DockerImageDownloadJobWorker(ctx context.Context, job *job.JobObj, progress chan<- float32) errorx.Error {
	paramsPtr := job.GetParamsPtr()
	if paramsPtr == nil {
		return errorx.NewErrInternalServerError("job parameters are nil")
	}

	params, ok := paramsPtr.(*DockerImageDownloadJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("invalid job parameters type; expected *DockerImageJobParams")
	}

	// Initialize the Docker executor
	executor := params.CodeExecutorPtr

	// Pull the Docker image with progress tracking
	reader, err := executor.cli.ImagePull(ctx, params.CodeExecutorPtr.Image, types.ImagePullOptions{})
	if err != nil {
		return errorx.NewErrInternalServerError("failed to pull Docker image: %s", err.Error())
	}
	defer reader.Close()

	type Event struct {
		Status         string `json:"status"`
		Error          string `json:"error"`
		Progress       string `json:"progress"`
		ProgressDetail struct {
			Current int `json:"current"`
			Total   int `json:"total"`
		} `json:"progressDetail"`
	}

	// Decode the JSON stream to track progress
	decoder := json.NewDecoder(reader)
	var lastProgress float32
	for {
		var event *Event

		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return errorx.NewErrInternalServerError("error decoding pull progress: %s", err.Error())
		}

		// Calculate progress percentage if both values are available
		if event.ProgressDetail.Current > 0 && event.ProgressDetail.Total > 0 {
			currentProgress := float32(event.ProgressDetail.Current) / float32(event.ProgressDetail.Total) * 95
			// Only send progress updates when there's meaningful change
			if currentProgress-lastProgress >= 1.0 {
				progress <- currentProgress
				lastProgress = currentProgress
			}
		}
	}

	return nil
}

// DockerImageDownloadParamsValidation validates the Docker image download job parameters
func DockerImageDownloadParamsValidation(ctx context.Context, job *jobs.JobObj) errorx.Error {
	paramsPtr := job.GetParamsPtr()
	if paramsPtr == nil {
		return errorx.NewErrInternalServerError("invalid job parameters")
	}

	params, ok := paramsPtr.(*DockerImageDownloadJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("invalid job parameters type; expected *DockerImageJobParams")
	}

	if params.CodeExecutor == "" {
		return errorx.NewErrBadRequest("docker executor is not set")
	}

	params.CodeExecutorPtr = newDockerExecutor(params.CodeExecutor)

	// Create a new Docker executor for the job
	if params.CodeExecutorPtr == nil {
		return errorx.NewErrInternalServerError(fmt.Sprintf("failed to create Docker executor for image '%s'", params.CodeExecutor))
	}

	return nil
}

// ScheduleDockerImageDownloadJob schedules a job to download a Docker image
func ScheduleDockerImageDownloadJob(ctx context.Context, executorName string) (*jobs.JobObj, error) {
	// Create job parameters
	params := &DockerImageDownloadJobParams{
		CodeExecutor: executorName,
	}

	// Marshal params to JSON
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, errorx.NewErrInternalServerError("failed to marshal job params: %s", err.Error())
	}

	// Create and schedule the job
	job, err := jobs.JENewJob(
		ctx,
		uuid.New(),
		dockerImageDownloadJob,
		string(paramsBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Docker image download job: %s", err.Error())
	}

	err = jobs.JEScheduleJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule Docker image download job: %s", err.Error())
	}

	return job, nil
}

var dockerImageDownloadJob *jobs.JobType

// RegisterDockerImageJob registers the Docker image download job type
func RegisterDockerImageJob() {
	dockerImageDownloadJob = jobs.RegisterJobType(
		jobs.JobTypeParams{
			Group: &jobs.JobGroup{
				Name:        "docker_image",
				Description: "Download and manage Docker images",
				Queue:       jobs.JobQueueDownload,
			},
			Name:                           "download",
			Description:                    "Download a Docker image",
			Params:                         &DockerImageDownloadJobParams{},
			WorkerParamsValidationCallback: DockerImageDownloadParamsValidation,
			WorkerExecutionCallback:        DockerImageDownloadJobWorker,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        3600,
			MaxRetries:                     30,
			RetryDelay:                     3,
		},
	)
}
