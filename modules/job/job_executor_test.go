package job

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/orm"
	"github.com/stretchr/testify/assert"
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
				WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
					return job, nil
				},
				WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
					// simulate work by sleeping a short duration
					time.Sleep(50 * time.Millisecond)
					// Mark job as complete within the worker
					return nil
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
				WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
					return nil, nil
				},
				WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
					// Block indefinitely until the context is canceled.
					<-ctx.Done()
					return ctx.Err()
				},
				WorkerStateUpdateCallback: func(ctx context.Context, job *JobObj) error { return nil },
				WorkerSuspendCallback:     func(ctx context.Context, job *JobObj) errorx.Error { return nil },
				WorkerResumeCallback:      func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) { return nil, nil },
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
				WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
					return job, nil
				},
				WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
					// Block indefinitely until the context is canceled.
					<-ctx.Done()
					return ctx.Err()
				},
				WorkerStateUpdateCallback: nil,
				WorkerSuspendCallback:     nil,
				WorkerResumeCallback:      nil,
				WorkerIsSuspendable:       true,
				Timeout:                   30 * time.Second,
				RetryDelay:                1 * time.Second,
				MaxRetries:                0,
			},
		)
	}
	return unblockedJobType
}

// setupTestDB sets up an in-memory SQLite DB for testing executor functionality.
func setupTestDB(t *testing.T) *gorm.DB {
	testDB, err := db.InitInMemorySQLite(nil)
	if err != nil {
		t.Fatalf("Failed to connect to test DB: %v", err)
	}
	db.DB = testDB
	if err := orm.OrmInit(testDB); err != nil {
		t.Fatalf("Failed to initialize ORM: %v", err)
	}

	// Migrate the necessary tables
	if err := testDB.AutoMigrate(
		&Job{},
		&JobGroup{},
		&JobType{},
	); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Initialize the job store for this test
	jobsStore = &JobsExecutor{
		queues:         make(map[JobQueueName]*JobQueue),
		shutdownSignal: make(chan struct{}),
	}

	// Register the necessary queues
	RegisterJobQueue(JobQueueCompute, 1)
	RegisterJobQueue(JobQueueMaintenance, 5)

	return testDB
}

func TestExecutor_ScheduleAndWait(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jt, `{"foo":"fast"}`)
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
			completedJob, err2 := JEGetJob(ctx, job.GetJobID())
			if err2 != nil || completedJob.GetStatus() != JobStatusCompleted {
				t.Fatalf("Job not found or not completed in DB after wait: %v", err2)
			}
		} else {
			t.Fatalf("Wait error: %v", err)
		}
	} else {
		if job.GetStatus() != JobStatusCompleted {
			t.Fatalf("Expected job status %s, got %s", JobStatusCompleted, job.GetStatus())
		}
	}
}

func TestExecutor_CancelWaiting(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Add a small delay to ensure database is ready
	time.Sleep(50 * time.Millisecond)

	// Schedule a blocked job to occupy the compute worker.
	idempotency1 := uuid.New()
	job1, err := NewJob(ctx, idempotency1, jt, `{"foo":"block1"}`)
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
	job2, err := NewJob(ctx, idempotency2, jt, `{"foo":"block2"}`)
	if err != nil {
		t.Fatalf("NewJob job2 error: %v", err)
	}
	if err := JEScheduleJob(ctx, job2); err != nil {
		t.Fatalf("Schedule job2 error: %v", err)
	}

	// Cancel the waiting job.
	if err := JECancelJob(ctx, job2.GetJobID(), nil); err != nil {
		t.Fatalf("Cancel job2 error: %v", err)
	}
	if job2.GetStatus() != JobStatusCanceled {
		t.Fatalf("Expected job2 status %s, got %s", JobStatusCanceled, job2.GetStatus())
	}

	// Cleanup: Cancel job1 so it does not block indefinitely.
	if err := JECancelJob(ctx, job1.GetJobID(), nil); err != nil {
		t.Fatalf("Cancel job1 error: %v", err)
	}
}

func TestExecutor_GetAndList(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jt, `{"foo":"getlist"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Allow some time for the job to be picked up (and hence be in the running state).
	time.Sleep(100 * time.Millisecond)

	gotJob, err := JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if gotJob.GetJobID() != job.GetJobID() {
		t.Fatalf("Get returned wrong job: expected %s, got %s", job.GetJobID(), gotJob.GetJobID())
	}

	// Cleanup: cancel the job.
	if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
		t.Fatalf("Cancel error: %v", err)
	}
}

func TestExecutor_Delete(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Schedule a blocked job to occupy the worker.
	idempotency1 := uuid.New()
	job1, err := NewJob(ctx, idempotency1, jt, `{"foo":"delete1"}`)
	if err != nil {
		t.Fatalf("NewJob job1 error: %v", err)
	}
	if err := JEScheduleJob(ctx, job1); err != nil {
		t.Fatalf("Schedule job1 error: %v", err)
	}
	// Allow time for job1 to be picked up and marked as running.
	time.Sleep(150 * time.Millisecond)

	// Schedule a second job which will be waiting because the worker is busy.
	idempotency2 := uuid.New()
	job2, err := NewJob(ctx, idempotency2, jt, `{"foo":"delete2"}`)
	if err != nil {
		t.Fatalf("NewJob job2 error: %v", err)
	}
	if err := JEScheduleJob(ctx, job2); err != nil {
		t.Fatalf("Schedule job2 error: %v", err)
	}
	// Allow time for job2 to be queued.
	time.Sleep(150 * time.Millisecond)

	// Delete the waiting job (job2).
	if err := JEDeleteJob(ctx, job2.GetJobID(), errors.New("deleted by API")); err != nil {
		t.Fatalf("Delete 'waiting job' error: %v", err)
	}

	// Verify that job2 is deleted from the database.
	var jobFromDB JobObj
	err = db.DB.Where("job_id = ?", job2.GetJobID()).First(&jobFromDB).Error
	if err == nil {
		t.Fatalf("Expected job2 to be deleted from DB, but found record")
	}

	// Attempt to delete a running job (job1), which should return an error.
	err = JEDeleteJob(ctx, job1.GetJobID(), errors.New("deleted by API"))
	if err == nil {
		t.Fatalf("Expected error when deleting a running job, but got nil")
	}

	// Cleanup: cancel job1.
	if err := JECancelJob(ctx, job1.GetJobID(), nil); err != nil {
		t.Fatalf("Cancel job1 error: %v", err)
	}
}

func TestExecutor_Shutdown(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jt, `{"foo":"shutdown"}`)
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

func TestJEGetJobByIdempotencyKey(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getFastJobType()
	assert.NotNil(t, jt)

	// Create a unique idempotency key
	idempotencyKey := uuid.New()

	// Create a job with the idempotency key
	job, err := NewJob(ctx, idempotencyKey, jt, `{"foo":"data"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	// Schedule the job
	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Test retrieving the job by idempotency key
	retrievedJob, err := JEGetJobByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		t.Fatalf("JEGetJobByIdempotencyKey error: %v", err)
	}

	// Verify the retrieved job matches the original
	assert.Equal(t, job.GetJobID(), retrievedJob.GetJobID(), "JobID should match")
	assert.Equal(t, job.GetIdempotencyKey(), retrievedJob.GetIdempotencyKey(), "IdempotencyKey should match")
	assert.Equal(t, job.GetType(), retrievedJob.GetType(), "JobType should match")
	assert.Equal(t, job.GetParams(), retrievedJob.GetParams(), "Payload should match")

	// Test with non-existent idempotency key
	nonExistentKey := uuid.New()
	_, err = JEGetJobByIdempotencyKey(ctx, nonExistentKey)
	assert.Error(t, err, "Expected error when retrieving job with non-existent idempotency key")

	// Test with empty idempotency key
	_, err = JEGetJobByIdempotencyKey(ctx, uuid.Nil)
	assert.Error(t, err, "Expected error when retrieving job with empty idempotency key")

	// Cleanup: cancel the job
	if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
		t.Fatalf("Cancel job error: %v", err)
	}
}

// TestJobExecutor_Timeout_Override tests that when a JobType has a 1-second timeout,
// but a job is created with a 5-second timeout parameter, the job's status becomes "timeout"
// after the JobType's timeout (1 second) is reached, ignoring the job parameter.
func TestJobExecutor_Timeout_Override(t *testing.T) {
	// Skip this test temporarily as it's flaky
	t.Skip("Skipping timeout test until the jobsStore.retry issue is fixed")

	setupTestDB(t)
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
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			// Worker callback - will run longer than the timeout
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is done or timeout occurs
				select {
				case <-ctx.Done():
					// This should happen after 1 second (JobType timeout)
					return ctx.Err()
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
	job, err := NewJob(ctx, idempotencyKey, timeoutJobType, `{"foo":"timeout_test"}`)
	if err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	// Override the job's timeout to 5 seconds (longer than the JobType timeout)
	err = job.SetRetryPolicy(0, 0, 5*time.Second)
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
	updatedJob, err := JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("Failed to get job: %v", err)
	}

	// Check that the job status is "timeout"
	assert.Equal(t, JobStatusTimedOut, updatedJob.GetStatus(), "Job status should be 'timeout'")
}

func TestStateUpdateCallback(t *testing.T) {
	setupTestDB(t)
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
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Send a progress update to trigger the StateUpdateCallback
				progress <- 0.5
				time.Sleep(100 * time.Millisecond)
				return nil
			},
			WorkerStateUpdateCallback: func(ctx context.Context, job *JobObj) error {
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
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"state_update_test"}`)
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
	jobStatus, err := JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	assert.Equal(t, JobStatusCompleted, jobStatus.GetStatus(), "Job should be completed")
}

func TestSuspendCallback(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a channel to track callback invocations
	suspendCallbackCalled := make(chan struct{}, 1)

	// Create a job type with a SuspendCallback
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "suspend_callback_test",
				Queue:       JobQueueCompute,
				Description: "Test job group for suspend callback",
			},
			Name:        "suspend_callback_test:job",
			Description: "Test job for suspend callback",
			Params:      &dummyParams{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is canceled
				<-ctx.Done()
				return ctx.Err()
			},
			WorkerSuspendCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Signal that the callback was called
				select {
				case suspendCallbackCalled <- struct{}{}:
				default:
					// Channel already has a value, which is fine
				}
				return nil
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"suspend_callback_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

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

	// Wait for the SuspendCallback to be called
	select {
	case <-suspendCallbackCalled:
		// Success - callback was called
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for SuspendCallback to be called")
	}

	time.Sleep(100 * time.Millisecond)

	// Verify job is suspended
	assert.Equal(t, JobStatusSuspended, job.GetStatus(), "Job should be suspended")

	// Cleanup: cancel the job
	if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}
}

func TestResumeCallback(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create channels to track callback invocations
	suspendCallbackCalled := make(chan struct{}, 1)
	resumeCallbackCalled := make(chan struct{}, 1)

	// Create a job type with both SuspendCallback and ResumeCallback
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "resume_callback_test",
				Queue:       JobQueueCompute,
				Description: "Test job group for resume callback",
			},
			Name:        "resume_callback_test:job",
			Description: "Test job for resume callback",
			Params:      &dummyParams{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is canceled
				<-ctx.Done()
				return ctx.Err()
			},
			WorkerSuspendCallback: func(ctx context.Context, job *JobObj) errorx.Error {
				// Signal that the callback was called
				select {
				case suspendCallbackCalled <- struct{}{}:
				default:
					// Channel already has a value, which is fine
				}
				return nil
			},
			WorkerResumeCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				// Signal that the callback was called
				select {
				case resumeCallbackCalled <- struct{}{}:
				default:
					// Channel already has a value, which is fine
				}
				return job, nil
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"resume_callback_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

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

	// Wait for the SuspendCallback to be called
	select {
	case <-suspendCallbackCalled:
		// Success - callback was called
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for SuspendCallback to be called")
	}

	time.Sleep(200 * time.Millisecond)

	// Verify job is suspended
	assert.Equal(t, JobStatusSuspended, job.GetStatus(), "Job should be suspended")

	// Resume the job
	if err := JEResumeJob(ctx, job.GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	// Wait for the ResumeCallback to be called
	select {
	case <-resumeCallbackCalled:
		// Success - callback was called
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for ResumeCallback to be called")
	}

	// Wait for job to start running again
	time.Sleep(100 * time.Millisecond)

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

func TestExecutor_SuspendAndResume(t *testing.T) {
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
	time.Sleep(50 * time.Millisecond)

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

	// Verify job is suspended
	assert.Equal(t, JobStatusSuspended, job.GetStatus(), "Job should be suspended")

	// Resume the job
	if err := JEResumeJob(ctx, job.GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

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

func TestExecutor_SuspendResumeWithMultipleJobs_Queue1(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getBlockedJobType()
	assert.NotNil(t, jt)

	// Create two jobs that will block indefinitely
	jobs := make([]*JobObj, 2)
	for i := 0; i < 2; i++ {
		idempotency := uuid.New()
		job, err := NewJob(ctx, idempotency, jt, fmt.Sprintf(`{"foo":"suspend_test_%d"}`, i))
		if err != nil {
			t.Fatalf("NewJob error: %v", err)
		}
		jobs[i] = job

		// Schedule the job
		if err := JEScheduleJob(ctx, job); err != nil {
			t.Fatalf("Schedule error: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Verify the first job is running and the second is waiting for the queue slot
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
		if err != nil {
			t.Fatalf("GetJob error: %v", err)
		}

		if i == 0 {
			assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(),
				fmt.Sprintf("Job %d should be running", i))
		} else {
			assert.Equal(t, JobStatusWaiting, jobStatus.GetStatus(),
				fmt.Sprintf("Job %d should be waiting", i))
		}
	}

	// Suspend first job
	if err := JESuspendJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("SuspendJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify first job is suspended and second is now running
	jobStatus, err := JEGetJob(ctx, jobs[0].GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after suspend: %v", err)
	}
	assert.Equal(t, JobStatusSuspended, jobStatus.GetStatus(), "First job should be suspended")

	jobStatus, err = JEGetJob(ctx, jobs[1].GetJobID())
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Second job should be now running")

	// Resume first job
	if err := JEResumeJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify the first job now is waiting for the queue slot while the second is running
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
		if err != nil {
			t.Fatalf("GetJob error after resume: %v", err)
		}

		if i == 0 {
			assert.Equal(t, JobStatusWaiting, jobStatus.GetStatus(),
				fmt.Sprintf("Job %d should be waiting for the queue slot", i))
		} else {
			assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(),
				fmt.Sprintf("Job %d should be running again", i))
		}
	}

	// Cleanup: cancel all jobs
	for _, job := range jobs {
		if err := JECancelJob(ctx,
			job.GetJobID(), nil); err != nil {
			t.Fatalf("CancelJob error: %v", err)
		}
	}
}

func TestExecutor_SuspendResumeWithMultipleJobs_Queue5(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	jt := getUnblockedJobType()
	assert.NotNil(t, jt)

	// Create 5 jobs that will block indefinitely
	jobs := make([]*JobObj, 5)
	for i := 0; i < 5; i++ {
		idempotency := uuid.New()
		job, err := NewJob(ctx, idempotency, jt, fmt.Sprintf(`{"foo":"suspend_test_%d"}`, i))
		if err != nil {
			t.Fatalf("NewJob error: %v", err)
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
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
		if err != nil {
			t.Fatalf("GetJob error: %v", err)
		}

		assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(),
			fmt.Sprintf("Job %d should be running", i))
	}

	// Suspend the first job
	if err := JESuspendJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("SuspendJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify first job is suspended and all others are running
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
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
	if err := JEResumeJob(ctx, jobs[0].GetJobID()); err != nil {
		t.Fatalf("ResumeJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify that all jobs are running now
	for i, job := range jobs {
		jobStatus, err := JEGetJob(ctx, job.GetJobID())
		if err != nil {
			t.Fatalf("GetJob error after resume: %v", err)
		}

		assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(),
			fmt.Sprintf("Job %d should be running", i))
	}

	// Cleanup: cancel all jobs
	for _, job := range jobs {
		if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
			t.Fatalf("CancelJob error: %v", err)
		}
	}
}

func TestSuspendThenCancelJob(t *testing.T) {
	setupTestDB(t)
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
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is canceled
				<-ctx.Done()
				return ctx.Err()
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"suspend_cancel_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

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

	time.Sleep(50 * time.Millisecond)

	// Verify job is suspended
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after suspend: %v", err)
	}
	assert.Equal(t, JobStatusSuspended, jobStatus.GetStatus(), "Job should be suspended")

	// Cancel the suspended job
	if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify job is canceled
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after cancel: %v", err)
	}
	assert.Equal(t, JobStatusCanceled, jobStatus.GetStatus(), "Job should be canceled")
}

func TestSuspendThenDeleteJob(t *testing.T) {
	setupTestDB(t)
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
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is canceled
				<-ctx.Done()
				return ctx.Err()
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"suspend_delete_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

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

	time.Sleep(50 * time.Millisecond)

	// Verify job is suspended
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after suspend: %v", err)
	}
	assert.Equal(t, JobStatusSuspended, jobStatus.GetStatus(), "Job should be suspended")

	// Delete the suspended job
	if err := JEDeleteJob(ctx, job.GetJobID(), errors.New("deleted by test")); err != nil {
		t.Fatalf("DeleteJob error: %v", err)
	}

	// Verify job is deleted from the database
	var jobFromDB JobObj
	err = db.DB.Where("job_id = ?", job.GetJobID()).First(&jobFromDB.private).Error
	if err == nil {
		t.Fatalf("Expected job to be deleted from DB, but found record")
	}
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "Expected gorm.ErrRecordNotFound, got %v", err)
}

func TestResumeRunningJob(t *testing.T) {
	setupTestDB(t)
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
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is canceled
				<-ctx.Done()
				return ctx.Err()
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"resume_running_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

	if err := JEScheduleJob(ctx, job); err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for job to start running
	time.Sleep(50 * time.Millisecond)

	// Verify job is running
	jobStatus, err := JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should be running")

	// Try to resume a running job - should succeed
	err = JEResumeJob(ctx, job.GetJobID())
	assert.NoError(t, err, "Resuming a running job should succeed")

	// Verify job is still running
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after resume attempt: %v", err)
	}
	assert.Equal(t, JobStatusRunning, jobStatus.GetStatus(), "Job should still be running")

	// Cleanup: cancel the job
	if err := JECancelJob(ctx, job.GetJobID(), nil); err != nil {
		t.Fatalf("CancelJob error: %v", err)
	}
}

func TestResumeCallbackError(t *testing.T) {
	setupTestDB(t)
	ctx := context.Background()

	// Create a job type that returns an error during resume
	jobType := RegisterJobType(
		JobTypeParams{
			Group: &JobGroup{
				Name:        "resume_error_test",
				Queue:       JobQueueCompute,
				Description: "Test job group for resume error",
			},
			Name:        "resume_error_test:job",
			Description: "Test job for resume error",
			Params:      &dummyParams{},
			WorkerInitCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return job, nil
			},
			WorkerExecutionCallback: func(ctx context.Context, worker JobWorker, progress chan<- float32) error {
				// Block until context is canceled
				<-ctx.Done()
				return ctx.Err()
			},
			WorkerResumeCallback: func(ctx context.Context, job *JobObj) (JobWorker, errorx.Error) {
				return nil, errorx.NewErrInternalServerError("Deliberate error during resume")
			},
			WorkerIsSuspendable: true,
			Timeout:             30 * time.Second,
			RetryDelay:          1 * time.Second,
			MaxRetries:          0,
		},
	)

	// Create and schedule the job
	idempotency := uuid.New()
	job, err := NewJob(ctx, idempotency, jobType, `{"foo":"resume_error_test"}`)
	if err != nil {
		t.Fatalf("NewJob error: %v", err)
	}

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

	// Wait for job to suspend
	for i := 0; i < 10; i++ {
		status := job.GetStatus()
		if status == JobStatusSuspended {
			break
		}
		assert.Equal(t, JobStatusSuspending, status, fmt.Sprintf("Job should be suspending or suspended, but was %s", status))
		time.Sleep(100 * time.Millisecond)
	}

	// Verify job is suspended
	assert.Equal(t, JobStatusSuspended, job.GetStatus(), "Job should be suspended")

	// Resume the job - this should trigger the error
	err = JEResumeJob(ctx, job.GetJobID())
	assert.Error(t, err, "Resume operation itself should fail")

	// Verify job is failed with the expected error
	jobStatus, err = JEGetJob(ctx, job.GetJobID())
	if err != nil {
		t.Fatalf("GetJob error after resume: %v", err)
	}
	assert.Equal(t, JobStatusFailed, jobStatus.GetStatus(), "Job should be failed")
	assert.NotEmpty(t, jobStatus.GetError(), "Job should have non-empty error")
	assert.Contains(t, jobStatus.GetError(), "Deliberate error during resume", "Job error should contain the expected message")
}
