package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var fastJobType *JobType

func getFastJobType() *JobType {
	if fastJobType == nil {
		fastJobType = RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "fast",
					Queue:       JobQueueCompute,
					Description: "Fast job group",
				},
				Name:        "fast:job",
				Description: "Fast job for executor test",
				Params:      &dummyParams{},
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					return nil
				},
				WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					// simulate work by sleeping a short duration
					time.Sleep(50 * time.Millisecond)
					// Mark job as complete within the worker
					return nil
				},
				WorkerStateUpdateCallback: nil,
				WorkerIsSuspendable:       false,
				Timeout:                   5 * time.Second,
				RetryDelay:                1 * time.Second,
				MaxRetries:                0,
			},
		)
	}
	return fastJobType
}

var blockedJobType *JobType

func getBlockedJobType() *JobType {
	if blockedJobType == nil {
		blockedJobType = RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "blocked",
					Queue:       JobQueueCompute, // parallelism is limited to 1
					Description: "Blocked job group",
				},
				Name:        "blocked:job",
				Description: "Blocked job for executor test",
				Params:      &dummyParams{},
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					return nil
				},
				WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					// Block indefinitely until the context is canceled.
					<-ctx.Done()
					return errorx.NewErrCanceled("canceled by test")
				},
				WorkerStateUpdateCallback: func(job *JobObj) errorx.Error { return nil },
				WorkerIsSuspendable:       true,
				Timeout:                   10 * time.Second,
				RetryDelay:                1 * time.Second,
				MaxRetries:                0,
			},
		)
	}
	return blockedJobType
}

var unblockedJobType *JobType

func getUnblockedJobType() *JobType {
	if unblockedJobType == nil {
		unblockedJobType = RegisterJobType(
			JobTypeParams{
				Group: &JobGroup{
					Name:        "unblocked",
					Queue:       JobQueueMaintenance, // parallelism is limited to 5
					Description: "Unblocked job group",
				},
				Name:        "unblocked:job",
				Description: "Unblocked job for executor test",
				Params:      &dummyParams{},
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					return nil
				},
				WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					// Block indefinitely until the context is canceled.
					<-ctx.Done()
					return errorx.NewErrCanceled("canceled by test")
				},
				WorkerStateUpdateCallback: nil,
				WorkerIsSuspendable:       true,
				Timeout:                   30 * time.Second,
				RetryDelay:                1 * time.Second,
				MaxRetries:                0,
			},
		)
	}
	return unblockedJobType
}

func TestJobExecutor_ScheduleAndWait(t *testing.T) {
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	idempotency := uuid.New()
	job, err := JENewJob(ctx, idempotency, jt, `{"foo":"fast"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for the job to complete
	err = JEWaitJob(ctx, job.GetJobID(), job.GetTimeoutSec())
	if err != nil {
		// If Wait returns "record not found", allow a short delay and then retrieve the job from the DB.
		if err.Error() == "failed to get job status: record not found" {
			time.Sleep(50 * time.Millisecond)
			completedJob, err2 := JEGetJob(ctx, job)

			status := completedJob.GetStatus()

			if err2 != nil || status != JobStatusCompleted {
				t.Fatalf("Job not found or not completed in DB after wait: %v", err2)
			}
		} else {
			t.Fatalf("Wait error: %v", err)
		}
	} else {
		status := job.GetStatus()
		if status != JobStatusCompleted {
			t.Fatalf("Expected job status %s, got %s", JobStatusCompleted, status)
		}
	}
}

func TestJobExecutor_CancelWaiting(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Add a small delay to ensure database is ready
	time.Sleep(50 * time.Millisecond)

	// Schedule a blocked job to occupy the compute worker.
	idempotency1 := uuid.New()
	job1, err := JENewJob(ctx, idempotency1, jt, `{"foo":"block1"}`)
	if err != nil {
		t.Fatalf("NewJob job1 error: %v", err)
	}
	if err := JEScheduleJob(ctx, job1); err != nil {
		t.Fatalf("Schedule job1 error: %v", err)
	}

	// Add a small delay to ensure job1 is fully processed
	time.Sleep(100 * time.Millisecond)

	// Schedule a second job which will be waiting because the queue capacity is 1.
	idempotency2 := uuid.New()
	job2, err := JENewJob(ctx, idempotency2, jt, `{"foo":"block2"}`)
	if err != nil {
		t.Fatalf("NewJob job2 error: %v", err)
	}
	if err := JEScheduleJob(ctx, job2); err != nil {
		t.Fatalf("Schedule job2 error: %v", err)
	}

	// Cancel the waiting job.
	if err := JECancelJob(ctx, job2.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("Cancel job2 error: %v", err)
	}

	status := job2.GetStatus()
	if status != JobStatusCanceled {
		t.Fatalf("Expected job2 status %s, got %s", JobStatusCanceled, status)
	}

	// Cleanup: Cancel job1 so it does not block indefinitely.
	if err := JECancelJob(ctx, job1.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("Cancel job1 error: %v", err)
	}
}

func TestJobExecutor_GetAndList(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	idempotency := uuid.New()
	job, err := JENewJob(ctx, idempotency, jt, `{"foo":"getlist"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Allow some time for the job to be picked up (and hence be in the running state).
	time.Sleep(100 * time.Millisecond)

	gotJob, err := JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if gotJob.GetJobID() != job.GetJobID() {
		t.Fatalf("Get returned wrong job: expected %s, got %s", job.GetJobID(), gotJob.GetJobID())
	}

	// Cleanup: cancel the job.
	if err := JECancelJob(ctx, job.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("Cancel error: %v", err)
	}
}

func TestJobExecutor_Delete(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Schedule a blocked job to occupy the worker.
	idempotency1 := uuid.New()
	job1, errx := JENewJob(ctx, idempotency1, jt, `{"foo":"delete1"}`)
	if errx != nil {
		t.Fatalf("NewJob job1 error: %v", errx)
	}
	if errx := JEScheduleJob(ctx, job1); errx != nil {
		t.Fatalf("Schedule job1 error: %v", errx)
	}

	// Allow time for job1 to be picked up and marked as running.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, job1.GetStatus(), JobStatusRunning)

	// Schedule a second job which will be waiting because the worker is busy.
	idempotency2 := uuid.New()
	job2, errx := JENewJob(ctx, idempotency2, jt, `{"foo":"delete2"}`)
	if errx != nil {
		t.Fatalf("NewJob job2 error: %v", errx)
	}
	if errx := JEScheduleJob(ctx, job2); errx != nil {
		t.Fatalf("Schedule job2 error: %v", errx)
	}
	// Allow time for job2 to be queued.
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, job2.GetStatus(), JobStatusWaiting)

	// Delete the waiting job (job2).
	if errx := jeDeleteJob(ctx, job2.GetJobID(), "deleted by API"); errx != nil {
		t.Fatalf("Delete 'waiting job' error: %v", errx)
	}

	// Verify that job2 is deleted from the database.
	var jobFromDB JobObj
	err := db.DB().Where("job_id = ?", job2.GetJobID()).First(&jobFromDB).Error
	if err == nil {
		t.Fatalf("Expected job2 to be deleted from DB, but found record")
	}

	// Attempt to delete a running job (job1), which should return an error.
	err = jeDeleteJob(ctx, job1.GetJobID(), "deleted by API")
	if err == nil {
		t.Fatalf("Expected error when deleting a running job, but got nil")
	}

	// Cleanup: cancel job1.
	if err := JECancelJob(ctx, job1.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("Cancel job1 error: %v", err)
	}
}

func TestJobExecutor_Shutdown(t *testing.T) {
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	idempotency := uuid.New()
	job, err := JENewJob(ctx, idempotency, jt, `{"foo":"shutdown"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Even after shutdown, Schedule may not return an error, but the job will never be processed.
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error after shutdown: %v", err)
	}

	// Create a context with a short timeout for waiting.
	waitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	err = JEWaitJob(waitCtx, job.GetJobID(), job.GetTimeoutSec())
	if err == nil {
		t.Fatalf("Expected timeout or error when waiting for job completion after shutdown, got nil")
	}
}

func TestJobExecutor_GetJobByIdempotencyKey(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Create a unique idempotency key
	idempotencyKey := uuid.New()

	// Create a job with the idempotency key
	job, err := JENewJob(ctx, idempotencyKey, jt, `{"foo":"data"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Schedule the job
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait a bit for the job to start running
	time.Sleep(50 * time.Millisecond)

	// Test retrieving the job by idempotency key
	retrievedJob, err := jeGetJobByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		t.Fatalf("JEGetJobByIdempotencyKey error: %v", err)
	}

	// Verify the retrieved job matches the original
	assert.Equal(t, job.GetJobID(), retrievedJob.GetJobID(), "JobID should match")
	assert.Equal(t, job.getIdempotencyKey(), retrievedJob.getIdempotencyKey(), "IdempotencyKey should match")
	assert.Equal(t, job.GetType(), retrievedJob.GetType(), "JobType should match")
	assert.Equal(t, job.getParams(), retrievedJob.getParams(), "Payload should match")

	// Test with non-existent idempotency key
	nonExistentKey := uuid.New()
	_, err = jeGetJobByIdempotencyKey(ctx, nonExistentKey)
	assert.Error(t, err, "Expected error when retrieving job with non-existent idempotency key")

	// Test with empty idempotency key
	_, err = jeGetJobByIdempotencyKey(ctx, uuid.Nil)
	assert.Error(t, err, "Expected error when retrieving job with empty idempotency key")

	// Cleanup: cancel the job (now it should be running and cancellable)
	if err := JECancelJob(ctx, job.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("Cancel job error: %v", err)
	}
}

func TestJobExecutor_Timeout_Override(t *testing.T) {
	ctx := context.Background()

	// Create a job type with a 1-second timeout
	timeoutJobGroup := &JobGroup{
		Name:        "timeout_test",
		Queue:       JobQueueCompute,
		Description: "Timeout test job group",
	}

	timeoutJobType := RegisterJobType(
		JobTypeParams{
			Group:       timeoutJobGroup,
			Name:        "timeout:job",
			Description: "Job with short timeout for testing timeout override",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			// Worker callback - will run longer than the timeout
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Block until context is done or timeout occurs
				select {
				case <-ctx.Done():
					// This should happen after 1 second (JobType timeout)
					return errorx.NewErrCanceled("")
				case <-time.After(3 * time.Second):
					// This should never be reached because the context should timeout first
					return nil
				}
			},
			Timeout:                   1 * time.Second, // JobType timeout of 1 second
			RetryDelay:                0,               // No retry delay
			MaxRetries:                0,               // No retries
			WorkerStateUpdateCallback: nil,             // No state update callback
		},
	)

	assert.NotNil(t, timeoutJobType)

	// Create a job with the timeout job type
	idempotencyKey := uuid.New()
	job, err := JENewJob(ctx, idempotencyKey, timeoutJobType, `{"foo":"timeout_test"}`)
	if err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Override the job's timeout to 5 seconds (longer than the JobType timeout)
	err = job.SetRetryPolicy(ctx, 0, 0, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to set retry policy: %v", err)
	}

	// Verify the job's timeout is set to 5 seconds
	assert.Equal(t, time.Duration(5)*time.Second, job.GetTimeoutSec(), "Job timeout should be 5 seconds")

	// Schedule the job
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Failed to schedule job: %v", err)
	}

	// Wait for the job to timeout
	// We need to wait at least 1 second (JobType timeout) but less than 5 seconds (job parameter timeout)
	time.Sleep(2 * time.Second)

	// Get the job from the database to check its status
	updatedJob, err := JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	status := updatedJob.GetStatus()

	// Check that the job status is "timeout"
	assert.Equal(t, JobStatusTimedOut, status, "Job status should be 'timeout'")
}

func TestJobExecutor_StateUpdateCallback(t *testing.T) {
	ctx := context.Background()

	// Create a channel to track callback invocations
	callbackCalled := make(chan struct{}, 1)

	// Create a job type with a StateUpdateCallback
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "state_update_test",
				Queue:       JobQueueMaintenance,
				Description: "Test job group for state update callback",
			},
			Name:        "state_update_test:job",
			Description: "Test job for state update callback",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Send a progress update to trigger the StateUpdateCallback
				err := job.SetProgress(ctx, 0.5)
				if err != nil {
					return err
				}
				time.Sleep(100 * time.Millisecond)
				return nil
			},
			WorkerStateUpdateCallback: func(job *JobObj) errorx.Error {
				// Signal that the callback was called
				select {
				case callbackCalled <- struct{}{}:
				default:
					// Channel already has a value, which is fine
				}
				return nil
			},
			Timeout:    2 * time.Second,
			RetryDelay: 1 * time.Second,
			MaxRetries: 0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := JENewJob(ctx, idempotency, jobType, `{"foo":"state_update_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for the job to complete and the callback to be called
	select {
	case <-callbackCalled:
		// Success - callback was called
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for StateUpdateCallback to be called")
	}

	// Wait for job to complete
	err = JEWaitJob(ctx, job.GetJobID(), 3*time.Second)
	if err != nil {
		t.Fatalf("Wait error: %v", err)
	}

	// Verify job completed successfully
	jobStatus, err := JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	status := jobStatus.GetStatus()
	assert.Equal(t, JobStatusCompleted, status, "Job should be completed")
}

func TestJobExecutor_SuspendAndResume2(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Create a job that will block indefinitely
	idempotency := uuid.New()
	job, errx := JENewJob(ctx, idempotency, jt, `{"foo":"suspend_test"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Schedule the job
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

	// Verify job is running
	jobStatus, err := JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	status := jobStatus.GetStatus()
	assert.Equal(t, JobStatusRunning, status, "Job should be running")

	// Suspend the job
	if err := jeSuspendJob(ctx, job.GetJobID()); err != nil {
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

	// Verify job is suspended
	status = job.GetStatus()
	assert.Equal(t, JobStatusSuspended, status, "Job should be suspended")

	// Resume the job
	if err := jeResumeJob(ctx, job.GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

	// Verify job is running again
	jobStatus, err = JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("GetJob error after resume: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running again")

	// Cleanup: cancel the job
	if err := JECancelJob(ctx, job.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}
}

func TestJobExecutor_SuspendResumeWithMultipleJobs_Queue1(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	queue, err := jeGetJobQueue(jt.GroupPtr.QueueName)
	if err != nil {
		t.Fatalf("GetJobQueue error: %v", err)
	}

	queueCapacity := queue.config.Capacity
	if queueCapacity != 1 {
		t.Fatalf("Queue capacity is %d, expected 1", queueCapacity)
		t.Skip()
	}

	// Create two jobs that will block indefinitely
	jobs := make([]*JobObj, queueCapacity+1)
	for i := 0; i < 2; i++ {
		idempotency := uuid.New()
		job, errx := JENewJob(ctx, idempotency, jt, fmt.Sprintf(`{"foo":"suspend_test_%d"}`, i))
		if errx != nil {
			t.Fatalf("NewJob error: %v", errx)
		}
		jobs[i] = job

		// Schedule the job
		if err := JEScheduleJob(ctx, job); err != nil {
			t.Fatalf("Schedule error: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	job1, err := JEGetJob(ctx, jobs[0])
	if err != nil {
		t.Fatalf("GetJob error after suspend: %v", err)
	}
	assert.Equal(t, JobStatusRunning, job1.GetStatus(), "First job should be running")

	job2, err := JEGetJob(ctx, jobs[1])
	if err != nil {
		t.Fatalf("GetJob error after suspend: %v", err)
	}
	assert.Equal(t, JobStatusWaiting, job2.GetStatus(), "Second job should be waiting")

	// Suspend first job
	if err := jeSuspendJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("SuspendJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify the first job now is waiting for the queue slot while the second is running
	job1, err = JEGetJob(ctx, jobs[0])
	if err != nil {
		t.Fatalf("GetJob error after resume: %v", err)
	}
	assert.Equal(t, JobStatusSuspended, job1.GetStatus(), "First job should be suspended")

	job2, err = JEGetJob(ctx, jobs[1])
	if err != nil {
		t.Fatalf("GetJob error after resume: %v", err)
	}
	assert.Equal(t, JobStatusRunning, job2.GetStatus(), "Second job should be running")

	if err := JECancelJob(ctx, job1.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}

	if err := JECancelJob(ctx, job2.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, JobStatusCanceled, job1.GetStatus(), "First job should be canceled")
	assert.Equal(t, JobStatusCanceled, job2.GetStatus(), "Second job should be canceled")
}

func TestJobExecutor_SuspendResumeWithMultipleJobs_Queue5(t *testing.T) {
	ctx := context.Background()

	jt := getUnblockedJobType()
	assert.NotNil(t, jt)

	// Create 5 jobs that will block indefinitely
	jobs := make([]*JobObj, 5)
	for i := 0; i < 5; i++ {
		idempotency := uuid.New()
		job, errx := JENewJob(ctx, idempotency, jt, fmt.Sprintf(`{"foo":"suspend_test_%d"}`, i))
		if errx != nil {
			t.Fatalf("NewJob error: %v", errx)
		}
		jobs[i] = job

		// Schedule the job
		if err := JEScheduleJob(ctx, job); err != nil {
			t.Fatalf("Schedule error: %v", err)
		}

		// Wait for jobs to start running
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all the jobs are running
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job)
		if err != nil {
			t.Fatalf("GetJob error: %v", err)
		}

		assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(),
			fmt.Sprintf("Job %d should be running", i))
	}

	// Suspend the first job
	if err := jeSuspendJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("SuspendJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify first job is suspended and all others are running
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job)
		if err != nil {
			t.Fatalf("GetJob error after suspend: %v", err)
		}

		if i == 0 {
			assert.Equal(t, JobStatusSuspended, jobStatus.GetStatus(), "First job should be suspended")
		} else {
			assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job %d should be running", i)
		}
	}

	// Resume the first job
	if err := jeResumeJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify that all jobs are running now
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job)
		if err != nil {
			t.Fatalf("GetJob error after resume: %v", err)
		}

		assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), fmt.Sprintf("Job %d should be running", i))
	}

	// Cleanup: cancel all jobs
	for _, job := range jobs {
		if err := JECancelJob(ctx, job.GetJobID(), "canceled by test"); err != nil {
			t.Fatalf("CancelJob error: %v", err)
		}
	}
}

func TestJobExecutor_SuspendThenCancelJob(t *testing.T) {
	ctx := context.Background()

	// Create a job type that will run for a while
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "suspend_cancel_test",
				Queue:       JobQueueCompute,
				Description: "Test job group for suspend then cancel",
			},
			Name:        "suspend_cancel_test:job",
			Description: "Test job for suspend then cancel",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Block until context is canceled
				<-ctx.Done()
				return errorx.NewErrCanceled("")
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, errx := JENewJob(ctx, idempotency, jobType, `{"foo":"suspend_cancel_test"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	if errx = JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("Schedule error: %v", errx)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

	// Verify job is running
	jobStatus, errx := JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error: %v", errx)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Suspend the job
	if errx = jeSuspendJob(ctx, job.GetJobID()); errx != nil {
		t.Fatalf("SuspendJob error: %v", errx)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify job is suspended
	jobStatus, errx = JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error after suspend: %v", errx)
	}
	assert.Equal(t, JobStatusSuspended, jobStatus.GetStatus(), "Job should be suspended")

	// Cancel the suspended job
	if errx := JECancelJob(ctx, job.GetJobID(), "canceled by test"); errx != nil {
		t.Fatalf("CancelJob error: %v", errx)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify job is canceled
	jobStatus, errx = JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error after cancel: %v", errx)
	}
	assert.Equal(t, JobStatusCanceled, jobStatus.GetStatus(), "Job should be canceled")
}

func TestJobExecutor_SuspendThenDeleteJob(t *testing.T) {
	ctx := context.Background()

	// Create a job type that will run for a while
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "suspend_delete_test",
				Queue:       JobQueueCompute,
				Description: "Test job group for suspend then delete",
			},
			Name:        "suspend_delete_test:job",
			Description: "Test job for suspend then delete",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Block until context is canceled
				<-ctx.Done()
				return errorx.NewErrCanceled("")
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, errx := JENewJob(ctx, idempotency, jobType, `{"foo":"suspend_delete_test"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

	// Verify job is running
	jobStatus, errx := JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error: %v", errx)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Suspend the job
	if errx := jeSuspendJob(ctx, job.GetJobID()); errx != nil {
		t.Fatalf("SuspendJob error: %v", errx)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify job is suspended
	jobStatus, errx = JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error after suspend: %v", errx)
	}
	assert.Equal(t, JobStatusSuspended, jobStatus.GetStatus(), "Job should be suspended")

	// Delete the suspended job
	if errx := jeDeleteJob(ctx, job.GetJobID(), "deleted by test"); errx != nil {
		t.Fatalf("DeleteJob error: %v", errx)
	}

	// Verify job is deleted from the database
	var jobFromDB JobObj
	err := db.DB().Where("job_id = ?", job.GetJobID()).First(&jobFromDB.priv).Error
	if err == nil {
		t.Fatalf("Expected job to be deleted from DB, but found record")
	}
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "Expected gorm.ErrRecordNotFound, got %v", err)
}

func TestJobExecutor_ResumeRunningJob(t *testing.T) {
	ctx := context.Background()

	// Create a job type that will run for a while
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "resume_running_test",
				Queue:       JobQueueCompute,
				Description: "Test job group for resuming running job",
			},
			Name:        "resume_running_test:job",
			Description: "Test job for resuming running job",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Block until context is canceled
				<-ctx.Done()
				return errorx.NewErrCanceled("")
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, errx := JENewJob(ctx, idempotency, jobType, `{"foo":"resume_running_test"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

	// Verify job is running
	jobStatus, err := JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Try to resume a running job - should succeed
	err = jeResumeJob(ctx, job.GetJobID())
	assert.NoError(t, err, "Resuming a running job should succeed")

	// Verify job is still running
	jobStatus, err = JEGetJob(ctx, job)
	if err != nil {
		t.Fatalf("GetJob error after resume attempt: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should still be running")

	// Cleanup: cancel the job
	if err := JECancelJob(ctx, job.GetJobID(), "canceled by test"); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}
}

// TestSetLockedBy_CircularDependency_Executor verifies that SetLockedBy detects circular lock dependencies
// using proper job executor methods instead of direct status manipulation.
func TestJobExecutor_SetLockedBy_CircularDependency(t *testing.T) {
	ctx := context.Background()

	JobQueueCircular := &JobQueueConfig{
		Name:     JobQueueName("circular-lock-test"),
		Capacity: 2,
	}
	if _, err := JERegisterJobQueue(JobQueueCircular); err != nil {
		t.Fatalf("Failed to register job queue: %v", err)
	}

	// Create a job type that will run long enough for us to test locking
	longRunningJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "circular-lock-test",
				Queue:       JobQueueCircular,
				Description: "Circular lock test job group",
			},
			Name:        "circular:lock:job",
			Description: "Job for testing circular lock detection",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Run for a long time or until canceled
				select {
				case <-ctx.Done():
					return errorx.NewErrCanceled("job was canceled")
				case <-time.After(30 * time.Second):
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   60 * time.Second,
			RetryDelay:                time.Second,
			MaxRetries:                0,
		},
	)

	// Create two jobs
	idempotencyKey1 := uuid.New()
	job1, errx := JENewJob(ctx, idempotencyKey1, longRunningJobType, `{"foo": "job1"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job1)

	idempotencyKey2 := uuid.New()
	job2, errx := JENewJob(ctx, idempotencyKey2, longRunningJobType, `{"foo": "job2"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job2)

	// Schedule both jobs using proper JE methods
	errx = JEScheduleJob(ctx, job1)
	assert.NoError(t, errx)

	errx = JEScheduleJob(ctx, job2)
	assert.NoError(t, errx)

	// Wait for both jobs to start running
	var job1Status, job2Status *JobObj
	for i := 0; i < 50; i++ {
		job1Status, errx = JEGetJob(ctx, job1)
		assert.NoError(t, errx)
		job2Status, errx = JEGetJob(ctx, job2)
		assert.NoError(t, errx)

		if job1Status.GetStatus() == JobStatusRunning && job2Status.GetStatus() == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify both jobs are running
	assert.Equal(t, JobStatusRunning, job1Status.GetStatus(), "Job1 should be running")
	assert.Equal(t, JobStatusRunning, job2Status.GetStatus(), "Job2 should be running")

	// Lock job2 by job1
	errx = job2Status.SetLockedBy(ctx, job1.GetJobID())
	assert.NoError(t, errx)

	// Get updated job2 status
	job2Status, errx = JEGetJob(ctx, job2)
	assert.NoError(t, errx)

	// Verify job2 is locked
	assert.Equal(t, JobStatusLocked, job2Status.GetStatus(), "Job2 should be locked")
	assert.Equal(t, job1.GetJobID(), job2Status.getLockedBy(), "Job2 should be locked by job1")

	// Attempt to lock job1 by job2, which would create a circular dependency
	errx = job1Status.SetLockedBy(ctx, job2.GetJobID())
	assert.Error(t, errx)
	assert.Contains(t, errx.Error(), "circular lock dependency detected")

	// Clean up: cancel both jobs
	JECancelJob(ctx, job1.GetJobID(), "test cleanup")
	JECancelJob(ctx, job2.GetJobID(), "test cleanup")
}

// TestSetTimedOut_Executor verifies that SetTimedOut correctly updates the job status and error
// using proper job executor methods instead of direct status manipulation.
func TestJobExecutor_SetTimedOut(t *testing.T) {
	ctx := context.Background()

	// Create a job type that will timeout quickly
	timeoutJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "timeout-test-executor",
				Queue:       JobQueueCompute,
				Description: "Timeout test job group for executor",
			},
			Name:        "timeout:executor:job",
			Description: "Job for testing timeout with executor",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Send some progress
				err := job.SetProgress(ctx, 25)
				if err != nil {
					return err
				}

				// Run longer than the timeout to trigger timeout
				select {
				case <-ctx.Done():
					// Context was canceled (due to timeout)
					return errorx.NewErrCanceled("job timed out")
				case <-time.After(10 * time.Second):
					// This should not happen as the job should time out before this
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   1 * time.Second, // Very short timeout to trigger timeout quickly
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), timeoutJobType, `{"foo": "timeout"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Schedule the job using proper JE method
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait for the job to timeout (should be around 1 second)
	errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, errx)

	// Get the timed out job status
	timedOutJob, errx := JEGetJob(ctx, job)
	assert.NoError(t, errx)

	// Verify job status and error
	assert.Equal(t, JobStatusTimedOut, timedOutJob.GetStatus(), "Job should be timed out")
}

// TestJobSkip_Executor verifies the job skip() API functionality using proper job executor methods
// instead of direct status manipulation to avoid race conditions with background job workers.
func TestJobExecutor_Skip(t *testing.T) {
	ctx := context.Background()

	// Create a job type that can be skipped
	skipJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "skip-test-executor",
				Queue:       JobQueueCompute,
				Description: "Skip test job group for executor",
			},
			Name:        "skip:executor:job",
			Description: "Job for testing skip functionality with executor",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Check if job was skipped
				if job.GetStatus() == JobStatusSkipped {
					return errorx.NewErrCanceled("job was skipped")
				}

				// Send some progress
				err := job.SetProgress(ctx, 25)
				if err != nil {
					return err
				}

				// Run for a while to allow skipping
				select {
				case <-ctx.Done():
					return errorx.NewErrCanceled("job was canceled")
				case <-time.After(5 * time.Second):
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   10 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), skipJobType, `{"foo": "skip"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Schedule the job using proper JE method
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait a bit for the job to start
	time.Sleep(100 * time.Millisecond)

	// Skip the job using proper JE method
	skipReason := "skipped by test"
	errx = job.SetSkipped(ctx, skipReason)
	assert.NoError(t, errx)

	// Wait for the job to complete (should be skipped)
	errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, errx)

	// Get the skipped job status
	skippedJob, errx := JEGetJob(ctx, job)
	assert.NoError(t, errx)

	// Verify job status
	assert.Equal(t, JobStatusSkipped, skippedJob.GetStatus(), "Job should be skipped")
	assert.Contains(t, skippedJob.getError(), skipReason, "Job error should contain skip reason")
}

// TestGetJob_Executor verifies that a job can be retrieved by its ID using proper job executor methods
// instead of direct status manipulation to avoid race conditions with background job workers.
func TestJobExecutor_GetJob(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a new job
	job, errx := JENewJob(ctx, idempotencyKey, jt, `{"foo": "bar"}`)
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Schedule the job using proper JE method
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait for the job to complete
	errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, errx)

	// Get the job by ID using proper JE method
	retrievedJob, errx := JEGetJob(ctx, job)
	assert.NoError(t, errx)
	assert.NotNil(t, retrievedJob)

	// Verify the job properties match
	assert.Equal(t, job.GetJobID(), retrievedJob.GetJobID())
	assert.Equal(t, job.GetType(), retrievedJob.GetType())
	assert.Equal(t, JobStatusCompleted, retrievedJob.GetStatus(), "Job should be completed")
}

// TestJobExecutor_Schedule verifies that a job can be scheduled and executed by the job executor.
func TestJobExecutor_Schedule(t *testing.T) {
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
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Signal that the job was executed
				executedChan <- true
				// Set progress to 100%
				err := job.SetProgress(ctx, 100)
				if err != nil {
					return err
				}
				// Set a result
				return job.SetResult(ctx, testResult{Status: "success"})
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   5 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), jobType, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job)
	assert.NotNil(t, job.priv)

	// Schedule the job with the executor
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait for the job to be executed
	select {
	case <-executedChan:
		// Job was executed
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for job to be executed")
	}

	// Wait for the job to complete
	errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, errx)

	// Verify the job status and result
	retrievedJob, errx := getJob(job.GetJobID())
	assert.NoError(t, errx)
	assert.NotNil(t, retrievedJob)

	assert.Equal(t, JobStatusCompleted, retrievedJob.GetStatus())
	assert.Equal(t, float32(100), retrievedJob.GetProgress())
	assert.Contains(t, retrievedJob.priv.Result, "success")
}

func TestJobExecutor_Cancel(t *testing.T) {
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
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Send initial progress
				err := job.SetProgress(ctx, 10)
				if err != nil {
					return err
				}

				// Wait for cancellation or timeout
				select {
				case <-ctx.Done():
					// Job was canceled
					canceledChan <- true
					return errorx.NewErrCanceled("job was canceled")
				case <-time.After(30 * time.Second):
					// Timeout, job wasn't canceled
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   60 * time.Second,
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), jobType, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Schedule the job with the executor
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait a bit for the job to start
	time.Sleep(1 * time.Second)

	// Cancel the job
	errx = JECancelJob(ctx, job.GetJobID(), "")
	assert.NoError(t, errx)

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
	retrievedJob, errx := getJob(job.GetJobID())
	assert.NoError(t, errx)
	assert.NotNil(t, retrievedJob)

	assert.Equal(t, JobStatusCanceled, retrievedJob.GetStatus())
}

// TestJobExecutor_Timeout verifies that a job times out if it exceeds its timeout duration.
func TestJobExecutor_Timeout(t *testing.T) {
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
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Send initial progress
				err := job.SetProgress(ctx, 10)
				if err != nil {
					return err
				}

				// Sleep longer than the timeout duration
				select {
				case <-ctx.Done():
					// Context was canceled (due to timeout)
					return errorx.NewErrCanceled("job timed out")
				case <-time.After(10 * time.Second):
					// This should not happen as the job should time out before this
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   1 * time.Second, // Very short timeout to trigger timeout quickly
			RetryDelay:                1 * time.Second,
			MaxRetries:                0,
		},
	)

	// Create a job
	job, errx := JENewJob(ctx, uuid.New(), jobType, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job)

	// Schedule the job with the executor
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait for the job to time out (should be around 1 second)
	errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
	assert.NoError(t, errx)

	// Verify the job status
	retrievedJob, errx := JEGetJob(ctx, job)
	assert.NoError(t, errx)
	assert.NotNil(t, retrievedJob)
	assert.Equal(t, JobStatusTimedOut, retrievedJob.GetStatus())
}

func TestJobExecutor_SuspendAndResume(t *testing.T) {
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Create a job that will block indefinitely
	idempotency := uuid.New()
	job, errx := JENewJob(ctx, idempotency, jt, `{"foo":"suspend_test"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Schedule the job
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("Schedule error: %v", errx)
	}

	// Wait for job to start running
	time.Sleep(100 * time.Millisecond)

	// Verify job is running
	jobStatus, errx := JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error: %v", errx)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Suspend the job
	if errx := jeSuspendJob(ctx, job.GetJobID()); errx != nil {
		t.Fatalf("SuspendJob error: %v", errx)
	}

	for i := 0; i < 10; i++ { // Wait for job to suspend
		status := job.GetStatus()
		if status == JobStatusSuspended {
			break
		}
		assert.Equal(t, JobStatusSuspending, status, fmt.Sprintf("Job should be suspending or suspended, but was %s", status))
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, JobStatusSuspended, job.GetStatus(), "Job %s should be suspended, but was %s", job.GetJobID(), job.GetStatus())

	// Resume the job
	if errx = jeResumeJob(ctx, job.GetJobID()); errx != nil {
		t.Fatalf("ResumeJob %s error: %v", job.GetJobID(), errx)
	}

	time.Sleep(10 * time.Millisecond)

	// Verify job is running again
	jobStatus, errx = JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("GetJob error after resume: %v", errx)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running again")

	// Cleanup: cancel the job
	if errx := JECancelJob(ctx, job.GetJobID(), "canceled by test"); errx != nil {
		t.Fatalf("CancelJob error: %v", errx)
	}
}

func TestJobExecutor_SuspendInvalidStates(t *testing.T) {
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a job that will complete quickly
	idempotency := uuid.New()
	job, errx := JENewJob(ctx, idempotency, jt, `{"foo":"suspend_invalid_test"}`)
	if errx != nil {
		t.Fatalf("NewJob error: %v", errx)
	}

	// Schedule the job
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("Schedule error: %v", errx)
	}

	// Wait for job to complete
	time.Sleep(200 * time.Millisecond)

	// Try to suspend a completed job
	errx = jeSuspendJob(ctx, job.GetJobID())
	assert.Error(t, errx, "Should not be able to suspend a completed job")

	// Try to resume a completed job
	errx = jeResumeJob(ctx, job.GetJobID())
	assert.Error(t, errx, "Should not be able to resume a completed job")
}

func TestJobExecutor_LockCancelUnlock(t *testing.T) {
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
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// This job will run for a long time unless canceled
				select {
				case <-ctx.Done():
					// Simulate some work before returning to allow state transitions to complete
					time.Sleep(50 * time.Millisecond)
					return errorx.NewErrCanceled("job timed out")
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
	job, errx := JENewJob(ctx, idempotency, jobType, `{"foo":"test_lock_cancel_unlock"}`)
	assert.NoError(t, errx, "NewJob error")

	// Schedule and run the job
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx, "Schedule error")

	// Wait for job to start running
	var jobStatus *JobObj
	for i := 0; i < 10; i++ {
		jobStatus, errx = JEGetJob(ctx, job)
		assert.NoError(t, errx, "GetJob error")
		status := jobStatus.GetStatus()
		if status == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify job is running
	status := jobStatus.GetStatus()
	assert.Equal(t, JobStatusRunning, status, "Job should be running")

	// Step 2: Lock the job
	// Create another job to use as the locker
	lockingJobIdempotency := uuid.New()
	lockingJob, errx := JENewJob(ctx, lockingJobIdempotency, jobType, `{"foo":"locking_job"}`)
	require.NoError(t, errx, "NewJob error for locking job")

	// Lock the job
	errx = jobStatus.SetLockedBy(ctx, lockingJob.GetJobID())
	assert.NoError(t, errx, "SetLockedBy error")

	// Verify job is locked
	jobStatus, errx = JEGetJob(ctx, job)
	assert.NoError(t, errx, "GetJob error after locking")
	assert.Equal(t, JobStatusLocked, jobStatus.GetStatus(), "Job should be locked")
	assert.Equal(t, lockingJob.GetJobID(), jobStatus.getLockedBy(), "Job should be locked by the locking job")

	// Step 3: Cancel the job
	errx = JECancelJob(ctx, job.GetJobID(), "test cancellation")
	assert.NoError(t, errx, fmt.Sprintf("CancelJob error: %s", errx))

	// After cancellation, job may be in 'canceling' state initially
	jobStatus, errx = JEGetJob(ctx, job)
	assert.NoError(t, errx, fmt.Sprintf("GetJob error after cancellation: %s", errx))

	// The job might be in either 'canceling' or 'canceled' state at this point
	// Wait for it to reach the final 'canceled' state
	for i := 0; i < 20; i++ {
		jobStatus, errx = JEGetJob(ctx, job)
		assert.NoError(t, errx, "GetJob error while waiting for canceled state")
		status := jobStatus.GetStatus()
		if status == JobStatusCanceled {
			break
		}
		// If it's not in canceling state, something else happened
		if status != JobStatusCanceling {
			t.Fatalf("Unexpected job status: %s", status)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify job reached canceled state
	assert.Equal(t, JobStatusCanceled, jobStatus.GetStatus(), "Job should eventually reach canceled state")

	// Step 4: Unlock the job
	errx = jobStatus.SetUnlocked(ctx)
	assert.Error(t, errx, "SetUnlocked for canceled job should fail")

	// Verify job is still canceled after unlocking
	jobStatus, errx = JEGetJob(ctx, job)
	assert.NoError(t, errx, "GetJob error after unlocking")
	assert.Equal(t, JobStatusCanceled, jobStatus.GetStatus(), "Job should still be canceled after unlocking")
	assert.Equal(t, uuid.Nil, jobStatus.getLockedBy(), "Job should not be locked by any job after unlocking")
}

func TestJobExecutor_CancelCompletedOrFailedJob(t *testing.T) {
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
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					return nil
				},
				WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
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
		job, errx := JENewJob(ctx, idempotency, jobType, `{"foo":"test_cancel_completed"}`)
		if errx != nil {
			t.Fatalf("NewJob error: %v", errx)
		}

		errx = JEScheduleJob(ctx, job)
		if errx != nil {
			t.Fatalf("Schedule error: %v", errx)
		}

		// Wait for job to complete
		for i := 0; i < 10; i++ {
			jobStatus, errx := JEGetJob(ctx, job)
			if errx != nil {
				t.Fatalf("GetJob error: %v", errx)
			}
			if jobStatus.GetStatus() == JobStatusCompleted {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Verify job is completed
		jobStatus, errx := JEGetJob(ctx, job)
		assert.NoError(t, errx, "GetJob error")
		assert.Equal(t, JobStatusCompleted, jobStatus.GetStatus(), "Job should be completed")

		// Try to cancel the completed job using our safe cancel function
		err := safeCancelJob(ctx, job.GetJobID())
		assert.NoError(t, err, "Canceling a completed job should not return an error")

		// Verify job is still completed
		jobStatus, err = JEGetJob(ctx, job)
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
				WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					return nil
				},
				WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
					// Fail immediately
					return errorx.NewErrCanceled("intentional failure for test")
				},
				Timeout:    1 * time.Second,
				RetryDelay: 1 * time.Second,
				MaxRetries: 0,
			},
		)

		// Create and schedule the job
		idempotency := uuid.New()
		job, errx := JENewJob(ctx, idempotency, jobType, `{"foo":"test_cancel_failed"}`)
		assert.NoError(t, errx, "NewJob error")

		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx, "Schedule error")

		// Wait for job to fail
		for i := 0; i < 10; i++ {
			jobStatus, err := JEGetJob(ctx, job)
			assert.NoError(t, err, "GetJob error")
			if jobStatus.GetStatus() == JobStatusFailed {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Verify job is failed
		jobStatus, err := JEGetJob(ctx, job)
		assert.NoError(t, err, "GetJob error")
		assert.Equal(t, JobStatusFailed, jobStatus.GetStatus(), "Job should be failed")

		// Try to cancel the failed job using our safe cancel function
		errx = JECancelJob(ctx, job.GetJobID(), "test cancellation")
		assert.NoError(t, errx, "Canceling a failed job should not return an error")

		// Verify job is still failed
		status := job.GetStatus()
		assert.Equal(t, JobStatusFailed, status, "Job should still be failed after cancel attempt")
	})
}

func TestJobExecutor_ScheduleJob(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, errx := JENewJob(ctx, idempotencyKey, jt, paramsJSON)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}

	// Test scheduling using proper JE method
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("JEScheduleJob returned error: %v", errx)
	}

	// Wait for the job to start running
	var jobStatus *JobObj
	for i := 0; i < 50; i++ { // Wait up to 5 seconds
		jobStatus, errx = JEGetJob(ctx, job)
		if errx != nil {
			t.Fatalf("JEGetJob returned error: %v", errx)
		}
		status := jobStatus.GetStatus()
		if status == JobStatusRunning || status == JobStatusCompleted {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify the job was scheduled and started running
	assert.NotEqual(t, int64(0), jobStatus.getScheduledAt(), "Expected ScheduledAt to be set")
	assert.NotEqual(t, int64(0), jobStatus.getETA(), "Expected ETA to be set")

	// If job is still running, wait for it to complete
	if jobStatus.GetStatus() == JobStatusRunning {
		assert.NotEqual(t, int64(0), jobStatus.getStartedAt(), "Expected StartedAt to be set when running")

		// Wait for completion
		errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
		if errx != nil {
			t.Fatalf("JEWaitJob returned error: %v", errx)
		}
	}
}

func TestJobExecutor_CompleteJob(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	jt := getDummyJobType()
	assert.NotNil(t, jt)

	job, errx := JENewJob(ctx, idempotencyKey, jt, paramsJSON)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}

	// Schedule the job using proper JE method
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("JEScheduleJob returned error: %v", errx)
	}

	// Wait for the job to complete naturally (the dummy job type completes automatically)
	errx = JEWaitJob(ctx, job.GetJobID(), 10*time.Second)
	if errx != nil {
		t.Fatalf("JEWaitJob returned error: %v", errx)
	}

	// Get the completed job status
	completedJob, errx := JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("JEGetJob returned error: %v", errx)
	}

	status := completedJob.GetStatus()
	if status != JobStatusCompleted {
		t.Fatalf("Expected job status 'completed', got '%s'", status)
	}
	if completedJob.GetProgress() != 100 {
		t.Fatalf("Expected job progress 100, got %.1f", completedJob.GetProgress())
	}
}

func TestJobExecutor_FailJob(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	// Create a job type that always fails
	dummyErr := errors.New("intentional test failure")
	failingJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "failing-test",
				Queue:       JobQueueCompute,
				Description: "Failing job group for tests",
			},
			Name:        "failing:job",
			Description: "Job that always fails for testing",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Always fail with the dummy error
				return errorx.NewErrInternalServerError(dummyErr.Error())
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   5 * time.Second,
			RetryDelay:                time.Second,
			MaxRetries:                0, // No retries, fail immediately
		},
	)

	job, errx := JENewJob(ctx, idempotencyKey, failingJobType, paramsJSON)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}

	// Schedule the job using proper JE method
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("JEScheduleJob returned error: %v", errx)
	}

	// Wait for the job to fail naturally
	errx = JEWaitJob(ctx, job.GetJobID(), 10*time.Second)
	if errx != nil {
		t.Fatalf("JEWaitJob returned error: %v", errx)
	}

	// Get the failed job status
	failedJob, errx := JEGetJob(ctx, job)
	if errx != nil {
		t.Fatalf("JEGetJob returned error: %v", errx)
	}

	status := failedJob.GetStatus()
	if status != JobStatusFailed {
		t.Fatalf("Expected job status 'failed', got '%s'", status)
	}
	if !strings.Contains(failedJob.getError(), dummyErr.Error()) {
		t.Fatalf("Expected job error to contain '%s', got '%s'", dummyErr.Error(), failedJob.getError())
	}
}

func TestJobExecutor_RetryJob(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()
	paramsJSON := `{"foo": "bar"}`

	// Track attempts using a shared variable
	var attemptCount int32

	// Create a job type that fails on first attempt but succeeds on retry
	dummyErr := errors.New("dummy retry failure")
	retryJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "retry-test",
				Queue:       JobQueueCompute,
				Description: "Retry job group for tests",
			},
			Name:        "retry:job",
			Description: "Job that fails first time but succeeds on retry",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Use atomic increment to track attempts safely
				currentAttempt := atomic.AddInt32(&attemptCount, 1)
				job.LogTrace("currentAttempt: %d", currentAttempt)

				if currentAttempt == 1 {
					// Fail on first attempt
					return errorx.NewErrInternalServerError(dummyErr.Error())
				}
				// Succeed on subsequent attempts
				err := job.SetProgress(ctx, 100)
				if err != nil {
					return err
				}
				return nil
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   5 * time.Second,
			RetryDelay:                1 * time.Second, // Short delay for test
			MaxRetries:                2,               // Allow retries
		},
	)

	job, errx := JENewJob(ctx, idempotencyKey, retryJobType, paramsJSON)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}

	initialRetries := job.getRetries()

	// Schedule the job using proper JE method
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("JEScheduleJob returned error: %v", errx)
	}

	for i := 0; i < 10; i++ {
		time.Sleep(50 * time.Millisecond)
		// Wait for the job to complete (should succeed after retry)
		errx = JEWaitJob(ctx, job.GetJobID(), 500*time.Millisecond)
		if errx != nil {
			switch errx.(type) {
			case *errorx.ErrLocked:
				t.Logf("JEWaitJob returned error: %v", errx)
				continue
			default:
				t.Fatalf("JEWaitJob returned error: %v", errx)
			}
		}

		if job.GetStatus() != JobStatusCompleted {
			t.Logf("Job status: %s, continue waiting ...", job.GetStatus())
			continue
		}

		// Verify retries were incremented
		if job.getRetries() <= initialRetries {
			t.Fatalf("Expected job.Retries to be incremented from %d, got %d", initialRetries, job.getRetries())
		}

		// Verify the job was attempted more than once
		if atomic.LoadInt32(&attemptCount) < 2 {
			t.Fatalf("Expected at least 2 attempts, got %d", atomic.LoadInt32(&attemptCount))
		}

		return
	}

	if job.GetStatus() != JobStatusCompleted {
		t.Fatalf("Expected job status 'completed', got '%s'", job.GetStatus())
	}
}

func TestJobExecutor_LockAndUnlock(t *testing.T) {
	ctx := context.Background()

	queueConfig := &JobQueueConfig{
		Name:     "lock-test-job-queue-for-lock-and-unlock",
		Capacity: 2,
	}
	if _, err := JERegisterJobQueue(queueConfig); err != nil {
		t.Fatalf("JERegisterJobQueue returned error: %v", err)
	}

	// Create a job type that runs long enough for us to test locking
	longRunningJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "lock-test",
				Queue:       queueConfig,
				Description: "Lock test job group",
			},
			Name:        "lock:job",
			Description: "Job for testing locking functionality",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Run for a long time or until canceled
				select {
				case <-ctx.Done():
					return errorx.NewErrCanceled("job was canceled")
				case <-time.After(30 * time.Second):
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   60 * time.Second,
			RetryDelay:                time.Second,
			MaxRetries:                0,
		},
	)

	// Create two jobs
	idempotencyKey1 := uuid.New()
	job1, errx := JENewJob(ctx, idempotencyKey1, longRunningJobType, `{"foo": "job1"}`)
	if errx != nil {
		t.Fatalf("NewJob job1 error: %v", errx)
	}

	idempotencyKey2 := uuid.New()
	job2, errx := JENewJob(ctx, idempotencyKey2, longRunningJobType, `{"foo": "job2"}`)
	if errx != nil {
		t.Fatalf("NewJob job2 error: %v", errx)
	}

	// Schedule both jobs
	if errx := JEScheduleJob(ctx, job1); errx != nil {
		t.Fatalf("JEScheduleJob job1 error: %v", errx)
	}
	if errx := JEScheduleJob(ctx, job2); errx != nil {
		t.Fatalf("JEScheduleJob job2 error: %v", errx)
	}

	// Wait for both jobs to start running
	var job1Status, job2Status *JobObj
	for i := 0; i < 50; i++ {
		job1Status, errx = JEGetJob(ctx, job1)
		if errx != nil {
			t.Fatalf("JEGetJob job1 error: %v", errx)
		}
		job2Status, errx = JEGetJob(ctx, job2)
		if errx != nil {
			t.Fatalf("JEGetJob job2 error: %v", errx)
		}

		if job1Status.GetStatus() == JobStatusRunning && job2Status.GetStatus() == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify both jobs are running
	assert.Equal(t, JobStatusRunning, job1Status.GetStatus(), "Job1 should be running")
	assert.Equal(t, JobStatusRunning, job2Status.GetStatus(), "Job2 should be running")

	// Lock job2 with job1's ID
	if errx := job2Status.SetLockedBy(ctx, job1.GetJobID()); errx != nil {
		t.Fatalf("Job2.SetLockedBy() returned error: %v", errx)
	}

	// Get updated job2 status
	job2Status, errx = JEGetJob(ctx, job2)
	if errx != nil {
		t.Fatalf("JEGetJob job2 after lock error: %v", errx)
	}

	status := job2Status.GetStatus()
	if status != JobStatusLocked {
		t.Fatalf("Expected job2 status 'locked', got '%s'", status)
	}
	if job2Status.getLockedBy() != job1.GetJobID() {
		t.Fatal("Expected job2 LockedBy to be job1's ID")
	}

	// Now unlock job2
	if errx := job2Status.SetUnlocked(ctx); errx != nil {
		t.Fatalf("Job2.SetUnlocked() returned error: %v", errx)
	}

	// Get updated job2 status
	job2Status, errx = JEGetJob(ctx, job2)
	if errx != nil {
		t.Fatalf("JEGetJob job2 after unlock error: %v", errx)
	}

	status = job2Status.GetStatus()
	if status != JobStatusRunning {
		t.Fatalf("Expected job2 status 'running' after unlock, got '%s'", status)
	}
	if job2Status.getLockedBy() != uuid.Nil {
		t.Fatalf("Expected job2 LockedBy to be uuid.Nil after unlock, got '%s'", job2Status.getLockedBy().String())
	}

	t.Logf("job1 status: %s", job1Status.GetStatus())
	t.Logf("job1 LockedBy: %s", job1Status.getLockedBy().String())

	t.Logf("job2 status: %s", job2Status.GetStatus())
	t.Logf("job2 LockedBy: %s", job2Status.getLockedBy().String())

	// Clean up: cancel both jobs
	JECancelJob(ctx, job1.GetJobID(), "test cleanup")
	JECancelJob(ctx, job2.GetJobID(), "test cleanup")
}

func TestJobExecutor_CancelingAndCancel(t *testing.T) {
	ctx := context.Background()
	idempotencyKey := uuid.New()

	// Create a job type that runs long enough to be canceled
	cancelableJobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "cancel-test",
				Queue:       JobQueueCompute,
				Description: "Cancel test job group",
			},
			Name:        "cancel:job",
			Description: "Job for testing cancellation functionality",
			Params:      &dummyParams{},
			WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return nil
			},
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Send some progress
				err := job.SetProgress(ctx, 25)
				if err != nil {
					return err
				}

				// Run for a long time or until canceled
				select {
				case <-ctx.Done():
					return errorx.NewErrCanceled("job was canceled")
				case <-time.After(30 * time.Second):
					return nil
				}
			},
			WorkerStateUpdateCallback: nil,
			WorkerIsSuspendable:       false,
			Timeout:                   60 * time.Second,
			RetryDelay:                time.Second,
			MaxRetries:                0,
		},
	)

	job, errx := JENewJob(ctx, idempotencyKey, cancelableJobType, `{"foo": "cancel"}`)
	if errx != nil {
		t.Fatalf("NewJob returned error: %v", errx)
	}

	// Schedule the job using proper JE method
	if errx := JEScheduleJob(ctx, job); errx != nil {
		t.Fatalf("JEScheduleJob returned error: %v", errx)
	}

	// Wait for the job to start running
	var jobStatus *JobObj
	for i := 0; i < 50; i++ {
		jobStatus, errx = JEGetJob(ctx, job)
		if errx != nil {
			t.Fatalf("JEGetJob returned error: %v", errx)
		}
		if jobStatus.GetStatus() == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify job is running
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Cancel the job using proper JE method
	cancelReason := "test cancellation"
	if errx := JECancelJob(ctx, job.GetJobID(), cancelReason); errx != nil {
		t.Fatalf("JECancelJob returned error: %v", errx)
	}

	// Wait for the job to reach canceled state (may go through canceling first)
	for i := 0; i < 50; i++ {
		jobStatus, errx = JEGetJob(ctx, job)
		if errx != nil {
			t.Fatalf("JEGetJob returned error: %v", errx)
		}
		status := jobStatus.GetStatus()

		// Job should be either canceling or canceled
		if status == JobStatusCanceled {
			break
		} else if status != JobStatusCanceling && status != JobStatusRunning {
			t.Fatalf("Unexpected job status during cancellation: '%s'", status)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify final job status
	status := jobStatus.GetStatus()
	if status != JobStatusCanceled {
		t.Fatalf("Expected job status 'canceled', got '%s'", status)
	}

	// Verify the error message contains the cancellation reason or indicates cancellation
	errorMsg := jobStatus.getError()
	if !strings.Contains(errorMsg, "cancel") && errorMsg != "" {
		t.Fatalf("Expected job error to contain 'cancel', got '%s'", errorMsg)
	}
}

// TestJobExecutor_SuspendWaitingJob tests suspending a job in the Waiting state (Waiting → Suspended)
func TestJobExecutor_SuspendWaitingJob(t *testing.T) {
	ctx := context.Background()

	queueConfig := &JobQueueConfig{
		Name:     "suspend-waiting-test-job-queue",
		Capacity: 1,
	}
	if _, err := JERegisterJobQueue(queueConfig); err != nil {
		t.Fatalf("JERegisterJobQueue returned error: %v", err)
	}

	jt := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "suspend-waiting-test",
			Queue:       queueConfig,
			Description: "Suspend waiting job group",
		},
		Name:                           "suspend-waiting-job",
		Description:                    "Job for suspending in waiting state",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
			select {
			case <-ctx.Done():
				return errorx.NewErrCanceled("job canceled")
			case <-time.After(5 * time.Second):
				return nil
			}
		},
		WorkerIsSuspendable: true,
		Timeout:             10 * time.Second,
	})

	job1, errx := JENewJob(ctx, uuid.New(), jt, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job1)
	assert.Equal(t, JobStatusInit, job1.GetStatus())

	job2, errx := JENewJob(ctx, uuid.New(), jt, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job2)
	assert.Equal(t, JobStatusInit, job1.GetStatus())

	errx = JEScheduleJob(ctx, job2)
	assert.NoError(t, errx)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, JobStatusRunning, job2.GetStatus())

	job3, errx := JENewJob(ctx, uuid.New(), jt, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job3)
	assert.Equal(t, JobStatusInit, job1.GetStatus())

	errx = JEScheduleJob(ctx, job3)
	assert.NoError(t, errx)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, JobStatusWaiting, job3.GetStatus())

	// Schedule but do not let it run yet (simulate by not starting workers immediately)
	errx = jeSuspendJob(ctx, job1.GetJobID())
	assert.NoError(t, errx, "Suspending initialized job should succeed")

	errx = jeSuspendJob(ctx, job3.GetJobID())
	assert.NoError(t, errx, "Suspending waiting job should succeed")

	errx = jeSuspendJob(ctx, job2.GetJobID())
	assert.NoError(t, errx, "Suspending running job should succeed")

	// Wait for job to become suspended
	for i := 0; i < 10; i++ {
		j, _ := JEGetJob(ctx, job2)
		if j.GetStatus() == JobStatusSuspended {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	j, _ := JEGetJob(ctx, job2)
	assert.Equal(t, JobStatusSuspended, j.GetStatus(), "Job should be suspended")

	j, _ = JEGetJob(ctx, job1)
	assert.Equal(t, JobStatusSuspended, j.GetStatus(), "Job should be suspended")

	j, _ = JEGetJob(ctx, job3)
	assert.Equal(t, JobStatusSuspended, j.GetStatus(), "Job should be suspended")
}

// TestJobExecutor_SuspendingToSuspended tests transition from Suspending to Suspended
func TestJobExecutor_SuspendingToSuspended(t *testing.T) {
	ctx := context.Background()
	jt := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "suspending-to-suspended",
			Queue:       JobQueueCompute,
			Description: "Suspending to suspended group",
		},
		Name:                           "suspending-to-suspended-job",
		Description:                    "Job for suspending to suspended",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
			// Wait for context cancel (simulate suspending)
			select {
			case <-ctx.Done():
				return errorx.NewErrCanceled("job suspended")
			case <-time.After(2 * time.Second):
				return nil
			}
		},
		WorkerIsSuspendable: true,
		Timeout:             10 * time.Second,
	})
	job, errx := JENewJob(ctx, uuid.New(), jt, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job)
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait for job to start running
	for i := 0; i < 10; i++ {
		j, _ := JEGetJob(ctx, job)
		if j.GetStatus() == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Suspend the job (should go to Suspending, then Suspended)
	errx = jeSuspendJob(ctx, job.GetJobID())
	assert.NoError(t, errx)
	errx = jeSuspendJob(ctx, job.GetJobID())
	assert.NoError(t, errx)
	// Wait for job to become suspended
	for i := 0; i < 20; i++ {
		j, _ := JEGetJob(ctx, job)
		if j.GetStatus() == JobStatusSuspended {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	j, _ := JEGetJob(ctx, job)
	assert.Equal(t, JobStatusSuspended, j.GetStatus(), "Job should be suspended after suspending")
}

// TestJobExecutor_ResumeSuspendedJob tests Suspended → Resuming → Waiting
func TestJobExecutor_ResumeSuspendedJob(t *testing.T) {
	ctx := context.Background()
	jt := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "resume-suspended",
			Queue:       JobQueueCompute,
			Description: "Resume suspended group",
		},
		Name:                           "resume-suspended-job",
		Description:                    "Job for resume suspended",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
			// Wait for context cancel (simulate suspending)
			select {
			case <-ctx.Done():
				return errorx.NewErrCanceled("job suspended")
			case <-time.After(2 * time.Second):
				return nil
			}
		},
		WorkerIsSuspendable: true,
		Timeout:             10 * time.Second,
	})
	job, errx := JENewJob(ctx, uuid.New(), jt, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job)
	errx = JEScheduleJob(ctx, job)
	assert.NoError(t, errx)

	// Wait for job to start running
	for i := 0; i < 10; i++ {
		j, _ := JEGetJob(ctx, job)
		if j.GetStatus() == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Suspend the job
	err := jeSuspendJob(ctx, job.GetJobID())
	assert.NoError(t, err)
	// Wait for job to become suspended
	for i := 0; i < 20; i++ {
		j, _ := JEGetJob(ctx, job)
		if j.GetStatus() == JobStatusSuspended {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Resume the job
	errx = jeResumeJob(ctx, job.GetJobID())
	assert.NoError(t, errx)
	errx = jeResumeJob(ctx, job.GetJobID())
	assert.NoError(t, errx)

	// Wait for job to go to waiting
	for i := 0; i < 20; i++ {
		j, _ := JEGetJob(ctx, job)
		status := j.GetStatus()
		if status == JobStatusWaiting || status == JobStatusRunning {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	status := job.GetStatus()
	assert.True(t, status == JobStatusWaiting || status == JobStatusRunning, "Job should be waiting or running after resume from suspended")
}

// TestJobExecutor_RetryingToRunning tests Retrying → Running transition
func TestJobExecutor_RetryingToRunning(t *testing.T) {
	ctx := context.Background()
	var attempt int
	jt := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "retrying-to-running",
			Queue:       JobQueueCompute,
			Description: "Retrying to running group",
		},
		Name:                           "retrying-to-running-job",
		Description:                    "Job for retrying to running",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
			attempt++
			if attempt == 1 {
				return errorx.NewErrInternalServerError("fail first")
			}
			err := job.SetProgress(ctx, 100)
			if err != nil {
				return err
			}
			return nil
		},
		WorkerIsSuspendable: false,
		Timeout:             5 * time.Second,
		RetryDelay:          1 * time.Second,
		MaxRetries:          1,
	})
	job, errx := JENewJob(ctx, uuid.New(), jt, "")
	assert.NoError(t, errx)
	assert.NotNil(t, job)
	JEScheduleJob(ctx, job)
	// Wait for job to retry and then complete
	for i := 0; i < 30; i++ {
		j, _ := JEGetJob(ctx, job)
		if j.GetStatus() == JobStatusCompleted {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	j, _ := JEGetJob(ctx, job)
	assert.Equal(t, JobStatusCompleted, j.GetStatus(), "Job should complete after retrying")
	assert.GreaterOrEqual(t, job.getRetries(), 1, "Job should have retried at least once")
}

// TestJobExecutor_DeleteFinalStates tests deleting jobs in all final states
func TestJobExecutor_DeleteFinalStates(t *testing.T) {
	ctx := context.Background()

	// Test deleting jobs in all final states
	t.Run("Completed", func(t *testing.T) {
		// Create a job that completes normally
		jt := RegisterJobType(JobTypeParams{
			Group: &JobGroup{
				Name:        "delete-test-completed",
				Queue:       JobQueueCompute,
				Description: "Test job group for delete test",
			},
			Name:                "delete-test-completed-job",
			Description:         "Job that completes successfully",
			WorkerIsSuspendable: false,
			Timeout:             10 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Just return nil for successful completion
				return nil
			},
		})

		job, errx := JENewJob(ctx, uuid.New(), jt, "")
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx)

		// Wait for job to complete
		errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
		assert.NoError(t, errx)

		// Verify job completed
		assert.Equal(t, JobStatusCompleted, job.GetStatus())

		// Now delete
		err := jeDeleteJob(ctx, job.GetJobID(), "delete test")
		assert.NoError(t, err, "Deleting completed job should succeed")

		// Should not be found after delete
		_, errx = JEGetJobByID(ctx, job.GetJobID())
		assert.Error(t, errx, "Job should not be found after delete")
	})

	t.Run("Failed", func(t *testing.T) {
		// Create a job that fails
		jt := RegisterJobType(JobTypeParams{
			Group: &JobGroup{
				Name:        "delete-test-failed",
				Queue:       JobQueueCompute,
				Description: "Test job group for delete test",
			},
			Name:                "delete-test-failed-job",
			Description:         "Job that fails",
			WorkerIsSuspendable: false,
			Timeout:             10 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				return errorx.NewErrInternalServerError("intentional failure for test")
			},
		})

		job, errx := JENewJob(ctx, uuid.New(), jt, "")
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx)

		// Wait for job to fail
		errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
		assert.NoError(t, errx) // Should error because job failed

		// Verify job failed
		assert.Equal(t, JobStatusFailed, job.GetStatus())

		// Now delete
		err := jeDeleteJob(ctx, job.GetJobID(), "delete test")
		assert.NoError(t, err, "Deleting failed job should succeed")

		// Should not be found after delete
		_, errx = JEGetJobByID(ctx, job.GetJobID())
		assert.Error(t, errx, "Job should not be found after delete")
	})

	t.Run("TimedOut", func(t *testing.T) {
		// Create a job that times out
		jt := RegisterJobType(JobTypeParams{
			Group: &JobGroup{
				Name:        "delete-test-timeout",
				Queue:       JobQueueCompute,
				Description: "Test job group for delete test",
			},
			Name:                "delete-test-timeout-job",
			Description:         "Job that times out",
			WorkerIsSuspendable: false,
			Timeout:             1 * time.Second, // Very short timeout
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Sleep longer than the timeout
				select {
				case <-ctx.Done():
					return errorx.NewErrCanceled("job timed out")
				case <-time.After(2 * time.Second):
					return nil
				}
			},
		})

		job, errx := JENewJob(ctx, uuid.New(), jt, "")
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx)

		// Wait for job to time out
		errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
		assert.NoError(t, errx)

		// Verify job timed out
		assert.Equal(t, JobStatusTimedOut, job.GetStatus())

		// Now delete
		err := jeDeleteJob(ctx, job.GetJobID(), "delete test")
		assert.NoError(t, err, "Deleting timed out job should succeed")

		// Should not be found after delete
		_, errx = JEGetJobByID(ctx, job.GetJobID())
		assert.Error(t, errx, "Job should not be found after delete")
	})

	t.Run("Skipped", func(t *testing.T) {
		// Create a job that gets skipped
		jt := RegisterJobType(JobTypeParams{
			Group: &JobGroup{
				Name:        "delete-test-skipped",
				Queue:       JobQueueCompute,
				Description: "Test job group for delete test",
			},
			Name:                "delete-test-skipped-job",
			Description:         "Job that gets skipped",
			WorkerIsSuspendable: false,
			Timeout:             10 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Skip the job
				err := JESetSkipped(ctx, job.GetJobID(), "skipped for test")
				if err != nil {
					return errorx.NewErrInternalServerError("failed to skip job: %v", err)
				}
				return nil
			},
		})

		job, errx := JENewJob(ctx, uuid.New(), jt, "")
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx)

		// Wait for job to be skipped
		errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
		assert.NoError(t, errx) // Should not error for skipped jobs

		// Verify job was skipped
		assert.Equal(t, JobStatusSkipped, job.GetStatus(), "Job '%s' should be skipped", job.GetJobID())

		// Now delete
		err := jeDeleteJob(ctx, job.GetJobID(), "delete test")
		assert.NoError(t, err, "Deleting skipped job should succeed")

		// Should not be found after delete
		_, errx = JEGetJobByID(ctx, job.GetJobID())
		assert.Error(t, errx, "Job should not be found after delete")
	})

	t.Run("Canceled", func(t *testing.T) {
		// Create a job that gets canceled
		jt := RegisterJobType(JobTypeParams{
			Group: &JobGroup{
				Name:        "delete-test-canceled",
				Queue:       JobQueueCompute,
				Description: "Test job group for delete test",
			},
			Name:                "delete-test-canceled-job",
			Description:         "Job that gets canceled",
			WorkerIsSuspendable: true,
			Timeout:             10 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
			WorkerExecutionCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Long-running job that can be canceled
				for i := 0; i < 10; i++ {
					select {
					case <-ctx.Done():
						return errorx.NewErrCanceled("job was canceled")
					case <-time.After(500 * time.Millisecond):
						// Continue working
					}
				}
				return nil
			},
		})

		job, errx := JENewJob(ctx, uuid.New(), jt, "")
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx)

		// Wait for job to start running
		for i := 0; i < 10; i++ {
			if job.GetStatus() == JobStatusRunning {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		assert.Equal(t, JobStatusRunning, job.GetStatus(), "Job should be running before cancellation")

		// Cancel the job
		errx = JECancelJob(ctx, job.GetJobID(), "canceled for test")
		assert.NoError(t, errx)

		// Wait for job to be canceled
		errx = JEWaitJob(ctx, job.GetJobID(), 5*time.Second)
		assert.NoError(t, errx)

		// Verify job was canceled
		assert.Equal(t, JobStatusCanceled, job.GetStatus())

		// Now delete
		err := jeDeleteJob(ctx, job.GetJobID(), "delete test")
		assert.NoError(t, err, "Deleting canceled job should succeed")

		// Should not be found after delete
		_, errx = JEGetJobByID(ctx, job.GetJobID())
		assert.Error(t, errx, "Job '%s' should not be found after delete", job.GetJobID())
	})
}

// TestJobExecutor_SaveWorkerSnapshot tests the SaveWorkerSnapshot functionality
func TestJobExecutor_SaveWorkerSnapshot(t *testing.T) {
	ctx := context.Background()

	// Create a job type for testing snapshots
	snapshotJobType := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "snapshot-test",
			Queue:       JobQueueCompute,
			Description: "Snapshot test job group",
		},
		Name:                           "snapshot:test:job",
		Description:                    "Job for testing worker snapshots",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerIsSuspendable:            true,
	})

	// Test 1: Save a simple string snapshot
	t.Run("SaveStringSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		snapshot := "test snapshot data"
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.Equal(t, snapshot, loadedSnapshot)
	})

	// Test 2: Save a complex struct snapshot
	t.Run("SaveStructSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		type TestSnapshot struct {
			Counter   int               `json:"counter"`
			Data      map[string]string `json:"data"`
			Timestamp int64             `json:"timestamp"`
		}

		snapshot := TestSnapshot{
			Counter: 42,
			Data: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			Timestamp: time.Now().Unix(),
		}

		errx = job.SaveWorkerSnapshot(snapshot)
		assert.NoError(t, errx)

		// Verify the snapshot was saved correctly
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)

		// Convert back to struct for comparison
		var loadedStruct TestSnapshot
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		err := json.Unmarshal(loadedJSON, &loadedStruct)
		assert.NoError(t, err)

		assert.Equal(t, snapshot.Counter, loadedStruct.Counter)
		assert.Equal(t, snapshot.Data, loadedStruct.Data)
		assert.Equal(t, snapshot.Timestamp, loadedStruct.Timestamp)
	})

	// Test 3: Save a slice snapshot
	t.Run("SaveSliceSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		snapshot := []int{1, 2, 3, 4, 5}
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		// Compare JSON representations to avoid type mismatch
		originalJSON, _ := json.Marshal(snapshot)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})

	// Test 4: Save a map snapshot
	t.Run("SaveMapSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		snapshot := map[string]interface{}{
			"string": "value",
			"int":    123,
			"float":  45.67,
			"bool":   true,
			"null":   nil,
		}
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		// Compare JSON representations to avoid type mismatch
		originalJSON, _ := json.Marshal(snapshot)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})

	// Test 5: Save nil snapshot
	t.Run("SaveNilSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		errx = job.SaveWorkerSnapshot(nil)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.Nil(t, loadedSnapshot)
	})

	// Test 6: Save empty string snapshot
	t.Run("SaveEmptyStringSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		snapshot := ""
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.Equal(t, snapshot, loadedSnapshot)
	})

	// Test 7: Save snapshot with special characters
	t.Run("SaveSpecialCharactersSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		snapshot := "test with \"quotes\", \n newlines, \t tabs, and unicode: 🚀"
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.Equal(t, snapshot, loadedSnapshot)
	})

	// Test 1: Save very large snapshot
	t.Run("SaveLargeSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Create a large snapshot (but not too large to avoid test output issues)
		largeData := make([]string, 1000)
		for i := range largeData {
			largeData[i] = fmt.Sprintf("data_%d_%s", i, strings.Repeat("x", 50))
		}

		errx = job.SaveWorkerSnapshot(largeData)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		// Compare JSON representations to avoid type mismatch
		originalJSON, _ := json.Marshal(largeData)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})
}

// TestJobExecutor_LoadWorkerSnapshot tests the LoadWorkerSnapshot functionality
func TestJobExecutor_LoadWorkerSnapshot(t *testing.T) {
	ctx := context.Background()

	// Create a job type for testing snapshots
	snapshotJobType := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "snapshot-load-test",
			Queue:       JobQueueCompute,
			Description: "Snapshot load test job group",
		},
		Name:                           "snapshot:load:test:job",
		Description:                    "Job for testing worker snapshot loading",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerIsSuspendable:            true,
	})

	// Test 1: Load snapshot from job with no saved snapshot
	t.Run("LoadEmptySnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Load snapshot from job that has never had a snapshot saved
		snapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.Nil(t, snapshot) // Should return nil for empty snapshot
	})

	// Test 2: Load snapshot after saving
	t.Run("LoadAfterSave", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		originalSnapshot := map[string]interface{}{
			"test": "data",
			"num":  42,
		}

		// Save snapshot
		errx = job.SaveWorkerSnapshot(originalSnapshot)
		assert.NoError(t, errx)

		// Load snapshot
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		// Compare JSON representations to avoid type mismatch
		originalJSON, _ := json.Marshal(originalSnapshot)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})

	// Test 3: Load snapshot multiple times
	t.Run("LoadMultipleTimes", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		originalSnapshot := "test data"

		// Save snapshot
		errx = job.SaveWorkerSnapshot(originalSnapshot)
		assert.NoError(t, errx)

		// Load snapshot multiple times
		for i := 0; i < 5; i++ {
			loadedSnapshot, errx := job.LoadWorkerSnapshot()
			assert.NoError(t, errx)
			assert.Equal(t, originalSnapshot, loadedSnapshot)
		}
	})

	// Test 4: Load snapshot with complex nested structure
	t.Run("LoadComplexSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		originalSnapshot := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": []interface{}{
						"item1",
						map[string]interface{}{
							"nested": "value",
						},
						123,
					},
				},
			},
			"array": []interface{}{
				"string",
				456,
				true,
				nil,
			},
		}

		// Save snapshot
		errx = job.SaveWorkerSnapshot(originalSnapshot)
		assert.NoError(t, errx)

		// Load snapshot
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		// Compare JSON representations to avoid type mismatch
		originalJSON, _ := json.Marshal(originalSnapshot)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})

	// Test 5: Load snapshot after job status changes
	t.Run("LoadAfterStatusChange", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		originalSnapshot := "persistent data"

		// Save snapshot
		errx = job.SaveWorkerSnapshot(originalSnapshot)
		assert.NoError(t, errx)

		// Change job status
		errx = JEScheduleJob(ctx, job)
		assert.NoError(t, errx)

		// Load snapshot after status change
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.Equal(t, originalSnapshot, loadedSnapshot)
	})
}

// TestJobExecutor_WorkerSnapshotConcurrency tests concurrent access to worker snapshots
func TestJobExecutor_WorkerSnapshotConcurrency(t *testing.T) {
	ctx := context.Background()

	// Create a job type for testing concurrent snapshots
	snapshotJobType := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "snapshot-concurrency-test",
			Queue:       JobQueueCompute,
			Description: "Snapshot concurrency test job group",
		},
		Name:                           "snapshot:concurrency:test:job",
		Description:                    "Job for testing concurrent worker snapshots",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerIsSuspendable:            true,
	})

	// Test concurrent save operations
	t.Run("ConcurrentSave", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		var wg sync.WaitGroup
		numGoroutines := 10
		errors := make(chan errorx.Error, numGoroutines)

		// Start multiple goroutines saving snapshots concurrently
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				snapshot := map[string]interface{}{
					"goroutine_id": id,
					"timestamp":    time.Now().UnixNano(),
				}
				errx := job.SaveWorkerSnapshot(snapshot)
				if errx != nil {
					errors <- errx
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for any errors
		for err := range errors {
			assert.NoError(t, err)
		}

		// Verify that a snapshot was saved (one of the concurrent saves should succeed)
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		assert.NotNil(t, loadedSnapshot)
	})

	// Test concurrent save and load operations
	t.Run("ConcurrentSaveAndLoad", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		var wg sync.WaitGroup
		numGoroutines := 5
		errors := make(chan errorx.Error, numGoroutines*2)

		// Start goroutines that save snapshots
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				snapshot := map[string]interface{}{
					"save_id":   id,
					"timestamp": time.Now().UnixNano(),
				}
				errx := job.SaveWorkerSnapshot(snapshot)
				if errx != nil {
					errors <- errx
				}
			}(i)
		}

		// Start goroutines that load snapshots
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				_, errx := job.LoadWorkerSnapshot()
				if errx != nil {
					errors <- errx
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for any errors
		for err := range errors {
			assert.NoError(t, err)
		}
	})
}

// TestJobExecutor_WorkerSnapshotEdgeCases tests edge cases for worker snapshots
func TestJobExecutor_WorkerSnapshotEdgeCases(t *testing.T) {
	ctx := context.Background()

	// Create a job type for testing edge cases
	snapshotJobType := RegisterJobType(JobTypeParams{
		Group: &JobGroup{
			Name:        "snapshot-edge-test",
			Queue:       JobQueueCompute,
			Description: "Snapshot edge case test job group",
		},
		Name:                           "snapshot:edge:test:job",
		Description:                    "Job for testing worker snapshot edge cases",
		Params:                         &struct{}{},
		WorkerParamsValidationCallback: func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerExecutionCallback:        func(ctx context.Context, job *JobObj) errorx.Error { return nil },
		WorkerIsSuspendable:            true,
	})

	// Test 1: Save very large snapshot
	t.Run("SaveLargeSnapshot", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Create a large snapshot (but not too large to avoid test output issues)
		largeData := make([]string, 1000)
		for i := range largeData {
			largeData[i] = fmt.Sprintf("data_%d_%s", i, strings.Repeat("x", 50))
		}

		errx = job.SaveWorkerSnapshot(largeData)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)
		// Compare JSON representations to avoid type mismatch
		originalJSON, _ := json.Marshal(largeData)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})

	// Test 2: Save snapshot with circular references (should fail gracefully)
	t.Run("SaveCircularReference", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Create a map with circular reference
		circularMap := make(map[string]interface{})
		circularMap["self"] = circularMap

		// This should fail due to circular reference
		errx = job.SaveWorkerSnapshot(circularMap)
		assert.Error(t, errx)
		assert.Contains(t, errx.Error(), "failed to marshal")
	})

	// Test 3: Save snapshot with channel (should fail gracefully)
	t.Run("SaveChannel", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Create a struct with a channel
		type TestStruct struct {
			Data    string
			Channel chan int
		}

		snapshot := TestStruct{
			Data:    "test",
			Channel: make(chan int),
		}

		// This should fail due to channel not being JSON serializable
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.Error(t, errx)
		assert.Contains(t, errx.Error(), "failed to marshal")
	})

	// Test 4: Save and load snapshot with function (should fail gracefully)
	t.Run("SaveFunction", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Create a struct with a function
		type TestStruct struct {
			Data     string
			Function func()
		}

		snapshot := TestStruct{
			Data:     "test",
			Function: func() {},
		}

		// This should fail due to function not being JSON serializable
		errx = job.SaveWorkerSnapshot(snapshot)
		assert.Error(t, errx)
		assert.Contains(t, errx.Error(), "failed to marshal")
	})

	// Test 5: Save snapshot with very deep nesting
	t.Run("SaveDeepNesting", func(t *testing.T) {
		job, errx := JENewJob(ctx, uuid.New(), snapshotJobType, `{}`)
		assert.NoError(t, errx)
		assert.NotNil(t, job)

		// Create deeply nested structure
		deepNested := make(map[string]interface{})
		current := deepNested
		for i := 0; i < 100; i++ {
			current["level"] = i
			if i < 99 {
				current["next"] = make(map[string]interface{})
				current = current["next"].(map[string]interface{})
			}
		}

		errx = job.SaveWorkerSnapshot(deepNested)
		assert.NoError(t, errx)

		// Verify the snapshot was saved
		loadedSnapshot, errx := job.LoadWorkerSnapshot()
		assert.NoError(t, errx)

		// JSON unmarshaling converts numbers to float64, so we need to compare the structure differently
		// Convert both to JSON strings for comparison
		originalJSON, _ := json.Marshal(deepNested)
		loadedJSON, _ := json.Marshal(loadedSnapshot)
		assert.Equal(t, string(originalJSON), string(loadedJSON))
	})
}
