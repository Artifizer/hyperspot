package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/job"
)

func downloadDataset(ctx context.Context, j *job.JobObj, params *DatasetJobParams) error {
	dataset := params.DatasetPtr

	for {
		if dataset.TryLock() {
			defer dataset.Unlock()
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Update dataset status
	err := dataset.StatusStartDownload()
	if err != nil {
		return fmt.Errorf("failed to start download: %w", err)
	}

	// Create a channel to signal download completion
	done := make(chan error, 1)

	// Create progress callback
	progressCallback := func(progress float32) {
		if err := dataset.UpdateDownloadProgress(progress); err != nil {
			logging.Warn("Failed to update download progress: %v", err)
		}
		if err := j.SetProgress(ctx, progress); err != nil {
			logging.Warn("Failed to set job progress: %v", err)
		}

		// Check if download is complete
		if progress >= 100 {
			done <- nil
		}
	}

	// Start the download in a goroutine
	go func() {
		// Start the download
		j.LogInfo("Starting download: %s -> %s", dataset.OriginalURL, dataset.LocalPath)
		err := utils.DownloadWithProgress(
			dataset.OriginalURL,
			dataset.LocalPath,
			progressCallback,
			false,
		)
		done <- err
	}()

	// Wait for download to complete or context cancelation
	select {
	case err := <-done:
		if err != nil {
			errMsg := fmt.Sprintf("download failed: %s", err.Error())
			if setErr := dataset.StatusFailDownload(errMsg); setErr != nil {
				logging.Error("Failed to set dataset status as failed: %v", setErr)
			}
			return fmt.Errorf("%s", errMsg)
		}

		// Update dataset status
		if setErr := dataset.StatusCompleteDownload(); setErr != nil {
			logging.Error("Failed to set dataset status as complete: %v", setErr)
		}

		// Get file info
		if fi, err := os.Stat(dataset.LocalPath); err == nil {
			if setErr := dataset.UpdateDownloadSize(fi.Size()); setErr != nil {
				logging.Warn("Failed to update download size: %v", setErr)
			}
		}

	case <-ctx.Done():
		if setErr := dataset.StatusCancelDownload(); setErr != nil {
			logging.Error("Failed to set dataset status as canceled: %v", setErr)
		}
		return ctx.Err()
	}

	if err := dataset.updateInDB(); err != nil {
		logging.Error("Failed to update dataset in DB: %v", err)
	}

	return nil
}

func deleteDataset(ctx context.Context, job *job.JobObj) errorx.Error {
	paramsPtr := job.GetParamsPtr()
	if paramsPtr == nil {
		return errorx.NewErrInternalServerError("job parameters are nil")
	}

	params, ok := paramsPtr.(*DatasetJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("invalid job parameters type; expected *DatasetJobParams")
	}

	dataset := params.DatasetPtr

	for {
		if dataset.TryLock() {
			defer dataset.Unlock()
			// if err := j.setRunning(); err != nil {
			//	return fmt.Errorf("failed to run job: %w", err)
			//}
			if err := dataset.Delete(); err != nil {
				return errorx.NewErrInternalServerError("failed to delete dataset: %s", err.Error())
			}
			break
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}

// LLMModelJob represents a LLM model management job
type DatasetJobParams struct {
	Dataset       string   `json:"dataset"`
	ForceDownload bool     `json:"force_download"`
	DatasetPtr    *Dataset `json:"-" gorm:"-"`
}

// LLMModelJobWorker performs work for LLM model jobs.
func DatasetDownloadJobWorkerExecutor(ctx context.Context, job *job.JobObj) errorx.Error {
	paramsPtr := job.GetParamsPtr()
	if paramsPtr == nil {
		return errorx.NewErrInternalServerError("job parameters are nil")
	}

	params, ok := paramsPtr.(*DatasetJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("invalid job parameters type; expected *DatasetJobParams")
	}

	if params.ForceDownload {
		job.LogInfo("Force downloading dataset: %s", params.Dataset)
	} else {
		if params.DatasetPtr.IsDownloaded() {
			msg := fmt.Sprintf("Dataset %s already downloaded, use force_download=true to force download", params.Dataset)
			job.LogInfo(msg)
			if setErr := job.SetSkipped(ctx, msg); setErr != nil {
				logging.Error("Failed to set job as skipped: %v", setErr)
			}
			return nil
		}
	}

	err := downloadDataset(ctx, job, params)

	if err != nil {
		return errorx.NewErrInternalServerError("failed to execute dataset job: %s", err.Error())
	}

	return nil
}

func DatasetDeleteJobWorker(ctx context.Context, job *job.JobObj) errorx.Error {
	err := deleteDataset(ctx, job)
	if err != nil {
		return errorx.NewErrInternalServerError("failed to delete dataset: %s", err.Error())
	}

	return nil
}

func DatasetUpdateJobWorker(ctx context.Context, job *job.JobObj) errorx.Error {
	err := deleteDataset(ctx, job)
	if err != nil {
		return errorx.NewErrInternalServerError("failed to delete dataset: %s", err.Error())
	}

	err = DatasetDownloadJobWorkerExecutor(ctx, job)
	if err != nil {
		return errorx.NewErrInternalServerError("failed to download dataset: %s", err.Error())
	}

	return nil
}

func DatasetDownloadJobWorkerInit(ctx context.Context, j *job.JobObj) errorx.Error {
	paramsPtr := j.GetParamsPtr()
	if paramsPtr == nil {
		return errorx.NewErrInternalServerError("invalid job parameters")
	}
	jobParams, ok := paramsPtr.(*DatasetJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("invalid job parameters type; expected *DatasetJobParams")
	}

	if jobParams.Dataset == "" {
		return errorx.NewErrBadRequest("Dataset is not set")
	}

	jobParams.DatasetPtr = GetDataset(jobParams.Dataset)
	if jobParams.DatasetPtr == nil {
		return errorx.NewErrBadRequest("Dataset not found")
	}

	// The job worker object is just a pointer to the job itself
	return nil
}

var JOB_GROUP_DATASET_DOWNLOAD = &job.JobGroup{
	Name:        "dataset_download",
	Description: "Download a dataset from remote site",
	Queue:       job.JobQueueDownload,
}

var JOB_GROUP_DATASET_OPS = &job.JobGroup{
	Name:        "dataset_ops",
	Description: "Operations on datasets",
	Queue:       job.JobQueueMaintenance,
}

var downloadJobType *job.JobType
var updateJobType *job.JobType
var deleteJobType *job.JobType

func initJobs() {
	// Register the LLM Model job worker.
	downloadJobType = job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_DATASET_DOWNLOAD,
			Name:                           "download",
			Description:                    "Download a dataset from remote site",
			Params:                         &DatasetJobParams{},
			WorkerParamsValidationCallback: DatasetDownloadJobWorkerInit,
			WorkerExecutionCallback:        DatasetDownloadJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Hour * 10,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 30,
			WorkerIsSuspendable:            false,
		},
	)

	updateJobType = job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_DATASET_DOWNLOAD,
			Name:                           "update",
			Description:                    "Update a dataset from remote site",
			Params:                         &DatasetJobParams{},
			WorkerParamsValidationCallback: DatasetDownloadJobWorkerInit,
			WorkerExecutionCallback:        DatasetDownloadJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Hour * 10,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 30,
			WorkerIsSuspendable:            false,
		},
	)

	deleteJobType = job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_DATASET_OPS,
			Name:                           "delete",
			Description:                    "Delete a dataset from local storage",
			Params:                         &DatasetJobParams{},
			WorkerParamsValidationCallback: DatasetDownloadJobWorkerInit,
			WorkerExecutionCallback:        DatasetDeleteJobWorker,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Hour * 10,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 30,
			WorkerIsSuspendable:            false,
		},
	)
}

func ScheduleDownloadJob(ctx context.Context, datasetName string) (j *job.JobObj, err error) {
	// Get the dataset
	dataset := GetDataset(datasetName)
	if dataset == nil {
		return nil, fmt.Errorf("dataset '%s' not found", datasetName)
	}

	// Create job parameters
	params := &DatasetJobParams{
		Dataset:    datasetName,
		DatasetPtr: dataset,
	}

	// Marshal params to JSON
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job params: %w", err)
	}

	// Create and schedule the job
	j, err = job.JENewJob(
		ctx,
		uuid.New(),
		downloadJobType,
		string(paramsBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new dataset download job: %w", err)
	}

	err = job.JEScheduleJob(ctx, j)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule dataset download job: %w", err)
	}

	return j, nil
}
