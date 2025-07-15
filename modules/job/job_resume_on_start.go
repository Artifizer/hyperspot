package job

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/logging"
)

// ResumeJobsOnServerStart finds all jobs in initializing, waiting, running, and locked states
// and resumes them. This function is intended to be called during server startup to catch up
// any jobs that might have been left in an inconsistent state due to a previous server shutdown.
// Jobs are processed in batches of 50 to avoid loading too many jobs into memory at once.
func ResumeJobsOnServerStart(ctx context.Context) error {
	// Define the states we want to resume
	statesToResume := []string{
		string(JobStatusInit),
		string(JobStatusWaiting),
		string(JobStatusRunning),
		string(JobStatusLocked),
		string(JobStatusCanceling),
		string(JobStatusSuspending),
	}

	statusFilter := strings.Join(statesToResume, ",")

	// Process jobs in batches of 50
	batchSize := 50
	totalJobs := 0
	totalResumed := 0
	totalCanceled := 0
	totalFailed := 0
	page := 0

	reason := "suspended on service restart"

	for {
		page++

		// Create pagination request for current batch
		pageRequest := &api.PageAPIRequest{
			PageNumber: page,
			PageSize:   batchSize,
		}

		// Find jobs in the specified states for the current page
		jobs, err := listJobs(ctx, pageRequest, statusFilter)
		if err != nil {
			return fmt.Errorf("failed to list in-progress jobs (page %d): %w", page, err)
		}

		// If no jobs found in this batch, we're done
		if len(jobs) == 0 {
			break
		}

		logging.Debug("Processing jobs resume after service start, batch: %d, found %d in-progress jobs to resume", page, len(jobs))

		for _, job := range jobs {
			status := job.GetStatus()
			if job.statusIsFinal(status) {
				continue
			}

			needToCancel := false
			totalJobs++

			if status == JobStatusDeleted {
				job.LogError("Job %s is deleted, skipping", job.priv.JobID)
				continue
			} else if status == JobStatusFailed {
				job.LogError("Job %s is failed, skipping", job.priv.JobID)
				continue
			} else if status == JobStatusTimedOut {
				job.LogError("Job %s is timed out, skipping", job.priv.JobID)
				needToCancel = true
			} else if status == JobStatusCanceling {
				needToCancel = true
			} else if job.GetTypePtr().WorkerIsSuspendable {
				// First suspend the job in memory (which doesn't require database access)
				// Then update the entry state in the database afterward
				err := job.setSuspending(reason)
				if err != nil {
					job.LogError("Failed to suspend job: %s", err.Error())
					needToCancel = true
				} else {
					err = job.setSuspended(reason)
					if err != nil {
						job.LogError("Failed to suspend job: %s", err.Error())
						needToCancel = true
					}
					job.priv.Error = reason

					job.mu.Lock()
					if err := job.dbSaveFields(&job.priv.Error); err != nil {
						job.LogError("Failed to save job error field: %v", err)
					}
					job.mu.Unlock()

					errx := jeResumeJob(ctx, job.GetJobID())
					if errx == nil {
						totalResumed++
						job.LogInfo("resumed on service start")
					} else {
						job.LogError("failed to resume the job after service restart: %s", errx.Error())
					}
				}
			} else {
				needToCancel = true
			}

			if needToCancel {
				// First cancel the job in memory (which doesn't require database access)
				// Then update the entry state in the database afterward
				err := job.setCanceling(reason)
				if err != nil {
					logging.Error("Failed to cancel job %s: %v", job.GetJobID().String(), err)
					totalFailed++
					continue
				} else {
					err = job.setCanceled(reason)
					if err != nil {
						logging.Error("Failed to cancel job %s: %v", job.GetJobID().String(), err)
						totalFailed++
						continue
					}
					totalCanceled++
				}
			}
		}

		// If we got fewer jobs than the batch size, we've processed all jobs
		if len(jobs) < batchSize {
			break
		}

		// Move to the next page automatically, because jobs are canceled
		time.Sleep(50 * time.Millisecond)
	}

	if totalJobs > 0 {
		logging.Info("Completed in-progress jobs resume after server restart: %d jobs resumed, %d jobs canceled, %d jobs failed",
			totalResumed, totalCanceled, totalFailed)
	}
	return nil
}
