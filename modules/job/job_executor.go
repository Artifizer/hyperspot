package job

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/auth"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"gorm.io/gorm"
)

// ----------------------------------------------------------------------
// Define JobQueueName type (string alias) for queues.
type JobQueueName string

var jobExecutorMu utils.DebugMutex

var jobsStore *JobsExecutor

const (
	JobQueueCompute     JobQueueName = "compute"
	JobQueueMaintenance JobQueueName = "maintenance"
	JobQueueDownload    JobQueueName = "download"
)

var errReasonCancelByAPI = fmt.Errorf("job canceled by API")

// ----------------------------------------------------------------------
// jobQueueWorker represents a pre-created jobQueueWorker that is attached to a specific queue.
type jobQueueWorker struct {
	// Store the name of the queue this worker belongs to.
	queue *JobQueue
	// (Additional fields could be added if needed.)
}

// ----------------------------------------------------------------------
// JobQueue encapsulates all queue‐specific logic.
type JobQueue struct {
	name         JobQueueName
	capacity     int
	workers      []*jobQueueWorker     // the pool of workers for this queue
	waiting      []*JobObj             // list of job IDs waiting to be executed
	running      map[uuid.UUID]*JobObj // map of running job IDs to their cancellation functions
	mu           utils.DebugMutex
	waitingJobCh chan struct{} // channel to signal new waiting jobs
}

// NewJobQueue creates a new JobQueue.
func NewJobQueue(name JobQueueName, capacity int) *JobQueue {
	return &JobQueue{
		name:         name,
		workers:      make([]*jobQueueWorker, 0, capacity),
		waiting:      make([]*JobObj, 0),
		running:      make(map[uuid.UUID]*JobObj),
		capacity:     capacity,
		waitingJobCh: make(chan struct{}, 1), // buffered so that a single signal suffices
	}
}

// AddJob adds a new job to the waiting list.
func (q *JobQueue) AddJob(job *JobObj) {
	q.mu.Lock()
	q.waiting = append(q.waiting, job)
	q.mu.Unlock()

	// Signal that a new waiting job is available (non-blocking)
	select {
	case q.waitingJobCh <- struct{}{}:
	default:
		// channel already signaled
	}
}

// GetNextJob returns the next waitingjob.
// If no jobs are waiting, returns nil.
func (q *JobQueue) GetNextJob() *JobObj {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.waiting) == 0 {
		return nil
	}
	j := q.waiting[0]
	q.waiting = q.waiting[1:]

	logging.Trace("Getting next job %s", j.GetJobID())
	return j
}

// AddRunningJob records that a job is now running.
func (q *JobQueue) AddRunningJob(job *JobObj) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job.LogTrace("adding job to the 'running' pool")

	q.running[job.GetJobID()] = job
}

// RemoveRunningJob removes a job from the running map.
func (q *JobQueue) RemoveRunningJob(jobID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	logging.Trace("removing job '%s' from the running pool", jobID)

	delete(q.running, jobID)
}

// ----------------------------------------------------------------------
// JobsExecutor simply routes jobs into their proper queue.
type JobsExecutor struct {
	queues         map[JobQueueName]*JobQueue
	shutdownSignal chan struct{}
	mu             utils.DebugMutex
}

func RegisterJobQueue(queueName JobQueueName, capacity int) (*JobQueue, error) {
	jobExecutorMu.Lock()
	defer jobExecutorMu.Unlock()

	queue := NewJobQueue(queueName, capacity)

	if jobsStore == nil {
		jobsStore = &JobsExecutor{
			queues:         make(map[JobQueueName]*JobQueue),
			shutdownSignal: make(chan struct{}),
		}
	}

	if _, ok := jobsStore.queues[queue.name]; ok {
		return nil, fmt.Errorf("job queue '%s' already registered", queue.name)
	}

	jobsStore.queues[queue.name] = queue

	for i := 0; i < queue.capacity; i++ {
		// Each worker is associated with its queue name.
		w := &jobQueueWorker{queue: queue}
		queue.workers = append(queue.workers, w)
		go jobsStore.runWorker(queue)
	}

	return queue, nil
}

func GetJobQueue(queueName JobQueueName) (*JobQueue, error) {
	jobExecutorMu.Lock()

	var ok bool
	var queue *JobQueue

	if jobsStore == nil {
		ok = false
	} else {
		queue, ok = jobsStore.queues[queueName]
	}

	jobExecutorMu.Unlock()

	if !ok {
		var err error

		if queueName == JobQueueCompute {
			queue, err = RegisterJobQueue(JobQueueCompute, 1)
		} else if queueName == JobQueueMaintenance {
			queue, err = RegisterJobQueue(JobQueueMaintenance, 5)
		} else if queueName == JobQueueDownload {
			queue, err = RegisterJobQueue(JobQueueDownload, 10)
		} else {
			return nil, fmt.Errorf("job queue '%s' not found", queueName)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to register job queue '%s': %w", queueName, err)
		}
	}

	return queue, nil
}

// getQueue selects the correct JobQueue based on the Job's type.
// It is assumed that job.GetQueue() returns a JobQueueName.
func (e *JobsExecutor) getQueue(job *JobObj) (*JobQueue, error) {
	queueName := job.GetQueue()
	queue, exists := e.queues[queueName]
	if !exists {
		return nil, fmt.Errorf("queue '%s' not found", queueName)
	}
	return queue, nil
}

// runWorker is the main loop for a worker that belongs to a given queue.
func (e *JobsExecutor) runWorker(queue *JobQueue) {
	for {
		// Check for global shutdown
		select {
		case <-e.shutdownSignal:
			return
		default:
			// Proceed
		}

		job := queue.GetNextJob()
		if job == nil {
			// wait for a signal that a job is available.
			select {
			case <-queue.waitingJobCh:
				// signal received, proceed to try getting the job again
			case <-e.shutdownSignal:
				return
			}
			continue
		}

		jobTransitionMutex.Lock()
		status := job.GetStatus()
		if status == JobStatusCanceling || status == JobStatusCanceled {
			jobTransitionMutex.Unlock()
			continue
		}
		jobTransitionMutex.Unlock()

		// Prepare execution context using job timeout if set.
		var ctx context.Context
		var cancelFunc context.CancelFunc

		timeout := time.Duration(job.GetTypePtr().TimeoutSec) * time.Second
		startTime := time.Now()

		if job.GetTypePtr().TimeoutSec > 0 {
			eta := time.Now().Add(timeout)
			job.LogDebug("Setting timeout to %d seconds (deadline: %s) for job %s", job.GetTypePtr().TimeoutSec, eta.Format(time.RFC3339), job.private.JobID)
			ctx, cancelFunc = context.WithTimeout(context.Background(), timeout)
		} else {
			ctx, cancelFunc = context.WithCancel(context.Background())
		}

		job.cancel = cancelFunc

		// Record the job as running in the queue.
		queue.AddRunningJob(job)

		if ctx.Err() != nil {
			job.LogError("starting the job with context error: %s", ctx.Err().Error())
		}

		// Execute the job.
		err := e.executeJob(ctx, job)

		ctxErr := ctx.Err()

		// Cleanup after execution.
		cancelFunc()
		queue.RemoveRunningJob(job.private.JobID)

		// Check job result.
		if err == nil {
			job.setCompleted(ctx, "completed successfully")
		} else {
			status = job.GetStatus()

			if ctxErr == context.DeadlineExceeded || (job.GetTypePtr().TimeoutSec > 0 && time.Since(startTime) >= timeout) {
				job.setTimedOut(ctx, err)
			} else if status == JobStatusCanceling {
				job.setCancelled(ctx, fmt.Errorf("canceled as '%w'", ctxErr))
			} else if status == JobStatusSuspending {
				job.setSuspended(ctx, fmt.Errorf("suspended as '%w'", ctxErr))
			} else if status == JobStatusTimedOut || status == JobStatusCanceled {
				// OK, do not retry timed out or canceled jobs
			} else if job.private.Retries < job.private.MaxRetries {
				e.retryJob(job, queue)
			} else {
				job.setFailed(ctx, err)
			}
		}
	}
}

// executeJob runs the job's worker function.
// It creates a progress channel that updates job progress.
func (e *JobsExecutor) executeJob(ctx context.Context, job *JobObj) error {
	progress := make(chan float32)
	defer close(progress)

	// Propagate progress updates.
	go func() {
		for p := range progress {
			job.SetProgress(ctx, p)
		}
	}()

	// Mark the job as running in persistent storage.
	if err := job.setRunning(ctx); err != nil {
		return err
	}

	if job.GetTypePtr() != nil && job.GetTypePtr().WorkerExecutionCallback != nil {
		// Execute the job-specific worker.
		err := job.GetTypePtr().WorkerExecutionCallback(ctx, job.getWorker(ctx), progress)
		if err != nil {
			return err
		}
	}

	return job.setCompleted(ctx, "")
}

// retryJob schedules a retry for the job after a delay (simple exponential backoff).
func (e *JobsExecutor) retryJob(job *JobObj, queue *JobQueue) {
	job.LogDebug("Retrying job %s", job.private.JobID.String())

	job.private.Retries++
	delay := time.Duration(job.private.Retries) * time.Duration(job.private.RetryDelaySec)
	time.AfterFunc(delay, func() {
		queue.AddJob(job)
	})
}

// schedule adds a job to the appropriate queue.
// job.GetQueue() determines which queue to use.
func (e *JobsExecutor) schedule(ctx context.Context, job *JobObj) errorx.Error {
	queue, err := e.getQueue(job)
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	// Initialize the job in persistent storage.
	if err := job.schedule(ctx); err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	queue.AddJob(job)
	return nil
}

// cancel searches all queues for the specified job and cancels it.
func (e *JobsExecutor) cancel(ctx context.Context, jobID uuid.UUID, reason error) errorx.Error {
	job, errx := e.get(ctx, jobID)
	if errx != nil {
		return errx
	}

	err := job.setCanceling(ctx, reason)
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, queue := range e.queues {
		queue.mu.Lock()
		if job, ok := queue.running[jobID]; ok {
			job.cancel()
			queue.mu.Unlock()

			job.LogDebug("canceling running job")

			// Keep job in the 'canceling' state until the worker function has completed
			return nil
		}
		// Check waitingjobs.
		for i, job := range queue.waiting {
			if job.GetJobID() == jobID {
				queue.waiting = append(queue.waiting[:i], queue.waiting[i+1:]...)
				queue.mu.Unlock()
				job.LogDebug("canceling waiting job")

				// Job is in the waiting state, so update status to 'canceled' immediately
				err = job.setCancelled(ctx, errReasonCancelByAPI)
				if err != nil {
					return errorx.NewErrInternalServerError(err.Error())
				}
				return nil
			}
		}
		queue.mu.Unlock()
	}

	// Fallback to DB
	job = &JobObj{}
	err = db.DB.First(&job.private, "job_id = ?", jobID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errorx.NewErrNotFound("job not found")
		}
		return errorx.NewErrInternalServerError(err.Error())
	}

	// Job is not being executed, so update status to 'canceled'
	err = job.setCancelled(ctx, errReasonCancelByAPI)
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	return nil
}

// suspend removes a job from the waiting or running queue and sets its status to suspended
func (e *JobsExecutor) suspend(ctx context.Context, jobID uuid.UUID) errorx.Error {
	job, errx := JEGetJob(ctx, jobID)
	if errx != nil {
		return errx
	}

	if !job.GetTypePtr().WorkerIsSuspendable {
		return errorx.NewErrMethodNotAllowed(fmt.Sprintf("job type '%s' doesn't support suspend/resume", job.GetType()))
	}

	err := job.setSuspending(ctx, nil)
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// First, try to find the job in any of the queues
	var foundJob *JobObj

	for _, queue := range e.queues {
		queue.mu.Lock()
		defer queue.mu.Unlock()

		// Check if the job is in the waiting queue
		for i, job := range queue.waiting {
			if job.GetJobID() != jobID {
				continue
			}

			if job.GetTenantID() != auth.GetTenantID() || job.GetUserID() != auth.GetUserID() {
				return errorx.NewErrNotFound("job not found")
			}

			// Remove job from waiting queue
			queue.waiting = append(queue.waiting[:i], queue.waiting[i+1:]...)
			err = job.setSuspended(ctx, nil)
			if err != nil {
				return errorx.NewErrInternalServerError(err.Error())
			}
			return nil
		}

		// If not found in waiting, check if it's running
		if foundJob == nil {
			job, exists := queue.running[jobID]

			if !exists {
				continue
			}

			if job.GetTenantID() != auth.GetTenantID() || job.GetUserID() != auth.GetUserID() {
				return errorx.NewErrNotFound("job not found")
			}

			job.LogInfo("canceling job %s before suspend", jobID)
			job.cancel()

			// Remove from running map
			delete(queue.running, jobID)
			return nil
		}
	}

	return errorx.NewErrBadRequest("job is in '%s' state and can not be suspended", job.GetStatus())
}

// resume puts a suspended job back in the waiting queue
func (e *JobsExecutor) resume(ctx context.Context, jobID uuid.UUID) errorx.Error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Get the tenant ID
	tenantID := auth.GetTenantID()

	// Get the job from the database
	job, getErr := JEGetJob(ctx, jobID)
	if getErr != nil {
		return errorx.NewErrNotFound(fmt.Sprintf("job '%s' not found", jobID))
	}

	// Verify tenant ID
	if job.private.TenantID != tenantID {
		return errorx.NewErrNotFound(fmt.Sprintf("job '%s' not found", jobID))
	}

	status := job.GetStatus()
	if status == JobStatusInit || status == JobStatusWaiting || status == JobStatusRunning {
		return nil
	}

	// Set the job status to resumed (waiting)
	resumeErr := job.setResumed(ctx)
	if resumeErr != nil {
		return errorx.NewErrInternalServerError(resumeErr.Error())
	}

	var err error
	var worker JobWorker

	if job.GetTypePtr().WorkerInitCallback != nil {
		jobWorkerMutex.Lock()
		defer jobWorkerMutex.Unlock()

		job.LogDebug("initializing job worker on resume")
		worker, err = job.GetTypePtr().WorkerInitCallback(ctx, job)
		if err != nil {
			return errorx.NewErrInternalServerError(err.Error())
		}
		job.worker = worker
	}

	// Add the job back to the appropriate queue
	queue, queueErr := e.getQueue(job)
	if queueErr != nil {
		return errorx.NewErrInternalServerError(queueErr.Error())
	}

	queue.AddJob(job)

	return nil
}

// get retrieves a running job from any of the queues.
func (e *JobsExecutor) get(ctx context.Context, jobID uuid.UUID) (*JobObj, errorx.Error) {
	for _, queue := range e.queues {
		queue.mu.Lock()
		if job, ok := queue.running[jobID]; ok {
			queue.mu.Unlock()
			return job, nil
		}

		for _, job := range queue.waiting {
			if job.GetJobID() == jobID {
				queue.mu.Unlock()
				return job, nil
			}
		}
		queue.mu.Unlock()
	}

	// Fallback to DB fetch
	logging.Debug("job '%s' not found in queues, falling back to DB fetch", jobID)
	return getJob(ctx, jobID)
}

func (e *JobsExecutor) getByIdempotencyKey(ctx context.Context, idempotencyKey uuid.UUID) (*JobObj, errorx.Error) {
	job := &JobObj{}
	err := db.DB.First(&job.private, "idempotency_key = ?", idempotencyKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.NewErrNotFound("job not found")
		}
		return nil, errorx.NewErrInternalServerError(err.Error())
	}

	return e.get(ctx, job.private.JobID)
}

// delete removes a job from the system.
// It will attempt to cancel the job (if waiting) and then remove it from the persistent store.
// Deletion of running jobs is not allowed.
func (e *JobsExecutor) delete(ctx context.Context, jobID uuid.UUID, reason error) errorx.Error {
	// Iterate over all queues to see if this job is in memory.
	for _, queue := range e.queues {
		queue.mu.Lock()
		// Check if job is running.
		if job, ok := queue.running[jobID]; ok {
			queue.mu.Unlock()

			if job.GetTenantID() != auth.GetTenantID() || job.GetUserID() != auth.GetUserID() {
				return errorx.NewErrNotFound("job not found")
			}

			return errorx.NewErrNotImplemented("deleting a running job is not supported yet")
		}

		foundWaiting := false

		// Check if job is waiting.
		for i, job := range queue.waiting {
			if job.GetJobID() != jobID {
				continue
			}

			if job.GetTenantID() != auth.GetTenantID() || job.GetUserID() != auth.GetUserID() {
				return errorx.NewErrNotFound("job not found")
			}

			queue.waiting = append(queue.waiting[:i], queue.waiting[i+1:]...)
			job.setCancelled(ctx, reason)
			foundWaiting = true
			break
		}
		queue.mu.Unlock()
		if foundWaiting {
			break
		}
	}

	// Delete the job record from the database.
	if err := db.DB.Delete(&Job{}, "tenant_id = ? AND job_id = ?", auth.GetTenantID(), jobID).Error; err != nil {
		return errorx.NewErrInternalServerError("failed to delete job from database: %v", err)
	}

	return nil
}

// Add this method to the JobsExecutor struct
func (e *JobsExecutor) wait(ctx context.Context, jobID uuid.UUID, timeoutSec time.Duration) errorx.Error {
	logging.Trace("Waiting for job %s to complete ...", jobID)

	// Create a context with the job's timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeoutSec)
	defer cancel()

	// Check job status periodically
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			msg := fmt.Sprintf("job %s wait timed out after %.1f seconds, job is still running", jobID, timeoutSec.Seconds())
			logging.Error(msg)
			return errorx.NewStatusAccepted(msg)
		case <-ticker.C:
			// Get current job status
			currentJob, errx := e.get(ctx, jobID)
			if errx != nil {
				return errx
			}

			// Check if job is completed
			switch currentJob.GetStatus() {
			case JobStatusCompleted, JobStatusTimedOut, JobStatusFailed, JobStatusCanceled, JobStatusSuspended, JobStatusSkipped:
				currentJob.LogTrace("job wait operation completed")
				return nil
			}
		}
	}
}

// Shutdown signals all workers to stop.
func (e *JobsExecutor) shutdown() {
	close(e.shutdownSignal)
}

// ----------------------------------------------------------------------
// Convenience wrapper functions

func JEScheduleJob(ctx context.Context, job *JobObj) errorx.Error {
	return jobsStore.schedule(ctx, job)
}

func JEWaitJob(ctx context.Context, jobID uuid.UUID, timeoutSec time.Duration) errorx.Error {
	return jobsStore.wait(ctx, jobID, timeoutSec)
}

func JEGetJob(ctx context.Context, jobID uuid.UUID) (*JobObj, errorx.Error) {
	return jobsStore.get(ctx, jobID)
}

func JEGetJobByIdempotencyKey(ctx context.Context, idempotencyKey uuid.UUID) (*JobObj, errorx.Error) {
	return jobsStore.getByIdempotencyKey(ctx, idempotencyKey)
}

func JECancelJob(ctx context.Context, jobID uuid.UUID, reason error) errorx.Error {
	return jobsStore.cancel(ctx, jobID, reason)
}

func JEDeleteJob(ctx context.Context, jobID uuid.UUID, reason error) errorx.Error {
	return jobsStore.delete(ctx, jobID, reason)
}

func JESuspendJob(ctx context.Context, jobID uuid.UUID) errorx.Error {
	return jobsStore.suspend(ctx, jobID)
}

func JEResumeJob(ctx context.Context, jobID uuid.UUID) errorx.Error {
	return jobsStore.resume(ctx, jobID)
}

func initJobQueues() error {
	RegisterJobQueue(JobQueueCompute, 1)
	RegisterJobQueue(JobQueueMaintenance, 5)
	RegisterJobQueue(JobQueueDownload, 10)
	return nil
}
