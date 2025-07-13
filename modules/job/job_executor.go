package job

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"gorm.io/gorm"
)

var jobExecutorMu utils.DebugMutex

var je *jobsExecutor

type jobsExecutor struct {
	queues         map[JobQueueName]*JobQueue
	jobsMap        map[uuid.UUID]*JobObj
	jobsLockMap    map[uuid.UUID]*utils.DebugMutex
	shutdownSignal chan struct{}
	mu             utils.DebugMutex
	deepTracing    bool
}

func JEInit() error {
	jobExecutorMu.Lock()
	if je == nil {
		je = &jobsExecutor{
			queues:         make(map[JobQueueName]*JobQueue),
			jobsMap:        make(map[uuid.UUID]*JobObj),
			jobsLockMap:    make(map[uuid.UUID]*utils.DebugMutex),
			shutdownSignal: make(chan struct{}),
			deepTracing:    true,
		}
	}
	jobExecutorMu.Unlock()

	if _, err := JERegisterJobQueue(JobQueueCompute); err != nil {
		return err
	}
	if _, err := JERegisterJobQueue(JobQueueMaintenance); err != nil {
		return err
	}
	if _, err := JERegisterJobQueue(JobQueueDownload); err != nil {
		return err
	}

	return nil
}

func JERegisterJobQueue(queueConfig *JobQueueConfig) (*JobQueue, error) {
	if je == nil {
		err := JEInit()
		if err != nil {
			return nil, err
		}
	}

	jobExecutorMu.Lock()
	defer jobExecutorMu.Unlock()

	if _, ok := je.queues[queueConfig.Name]; ok {
		if je.queues[queueConfig.Name].config.Capacity != queueConfig.Capacity {
			return nil, fmt.Errorf("queue '%s' already registered with different capacity", queueConfig.Name)
		}
		return je.queues[queueConfig.Name], nil
	}

	queue := NewJobQueue(queueConfig)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	for i := 0; i < queueConfig.Capacity; i++ {
		// Each worker is associated with its queue name.
		w := &jobQueueWorker{queue: queue}
		queue.workers = append(queue.workers, w)
		go je.jobWorker(i, queue)
	}

	je.queues[queueConfig.Name] = queue

	return queue, nil
}

func jeGetJobQueue(queueName JobQueueName) (*JobQueue, error) {
	if je == nil {
		if err := JEInit(); err != nil {
			return nil, err
		}
	}

	jobExecutorMu.Lock()
	defer jobExecutorMu.Unlock()

	if _, ok := je.queues[queueName]; ok {
		return je.queues[queueName], nil
	}

	return nil, fmt.Errorf("queue '%s' not registered", queueName)
}

func (e *jobsExecutor) trace(msg string, args ...interface{}) {
	if e.deepTracing {
		logging.Trace("jobsExecutor: "+msg, args...)
	}
}

// getNextJob returns the next waitingjob.
// If no jobs are waiting, returns nil.
func (e *jobsExecutor) getNextJob(queue *JobQueue, workerID int) *JobObj {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	jobID, errx := getFirstWaitingJob(string(queue.config.Name))

	if errx != nil {
		switch errx.(type) {
		case *errorx.ErrNotFound:
			return nil
		default:
			logging.Error("failed to get first waiting job: %v", errx)
			return nil
		}
	}

	job, err := e.lockJobByID(jobID, 0)
	if err != nil {
		return nil
	}

	e.trace("getNextJob(%s, worker #%d): got job %s", queue.config.Name, workerID, job.GetJobID())
	return job
}

// getQueue selects the correct JobQueue based on the Job's type.
// It is assumed that job.GetQueue() returns a JobQueueName.
func (e *jobsExecutor) getQueue(job *JobObj) (*JobQueue, error) {
	queueName := job.getQueueName()
	queue, exists := e.queues[queueName]
	if !exists {
		return nil, fmt.Errorf("queue '%s' not found", queueName)
	}
	return queue, nil
}

// jobWorker is the main loop for a worker that belongs to a given queue.
func (e *jobsExecutor) jobWorker(workerID int, queue *JobQueue) {
	e.trace("jobWorker(queue %s@%p, worker #%d)", queue.config.Name, queue, workerID)
	for {
		// Check for global shutdown
		select {
		case <-e.shutdownSignal:
			e.trace("jobWorker(queue %s@%p, worker #%d): shutdown signal received", queue.config.Name, queue, workerID)
			return
		default:
			// Proceed
		}

		e.trace("jobWorker(queue %s@%p, worker #%d): waiting for next job ...", queue.config.Name, queue, workerID)

		job := e.getNextJob(queue, workerID)
		if job == nil {
			// wait for a signal that a job is available.
			select {
			case <-queue.waitingJobCh:
				e.trace("jobWorker(queue %s@%p, worker #%d): signal received", queue.config.Name, queue, workerID)
				// signal received, proceed to try getting the job again
			case <-e.shutdownSignal:
				e.trace("jobWorker(queue %s@%p, worker #%d): shutdown signal received", queue.config.Name, queue, workerID)
				return
			}
			continue
		}

		e.trace("jobWorker(queue %s@%p, worker #%d): processing job %s", queue.config.Name, queue, workerID, job.GetJobID())

		// Prepare execution context using job timeout if set.
		var ctx context.Context
		var cancelFunc context.CancelFunc

		timeout := time.Duration(job.GetTypePtr().TimeoutSec) * time.Second
		startTime := time.Now()

		if job.GetTypePtr().TimeoutSec > 0 {
			eta := time.Now().Add(timeout)
			job.LogDebug("Setting timeout to %d seconds (deadline: %s) for job %s", job.GetTypePtr().TimeoutSec, eta.Format(time.RFC3339), job.priv.JobID)
			ctx, cancelFunc = context.WithTimeout(context.Background(), timeout)
		} else {
			ctx, cancelFunc = context.WithCancel(context.Background())
		}

		job.cancel = cancelFunc

		if ctx.Err() != nil {
			job.LogError("starting the job with context error: %s", ctx.Err().Error())
		}

		err := job.setRunning()
		if err != nil {
			job.LogError("failed to set job as running: %v", err)
			err = job.setFailed(fmt.Sprintf("failed to set job as running: %v", err))
			if err != nil {
				job.LogError("failed to set job as failed: %v", err)
			}
			time.Sleep(100 * time.Millisecond) // any better ideas?
			e.unlockJob(job)
			continue
		}
		e.unlockJob(job)

		// Execute the job.
		execErr := e.executeJob(ctx, job)

		e.trace("jobWorker(queue %s@%p, worker #%d): job %s execution completed, err: %v", queue.config.Name, queue, workerID, job.GetJobID(), err)

		if job, errx := e.lockJob(job, 0); errx != nil {
			job.LogError("Failed to lock job: %v", errx)
			panic("jobWorker() - failed to lock job") // must not happen
		}

		status := job.getStatus()

		ctxErr := ctx.Err()

		// Cleanup after execution.
		cancelFunc()

		// Check job result.
		if execErr == nil {
			if status == JobStatusSkipped {
				job.LogTrace("skipped job")
			} else if setErr := job.setCompleted("completed successfully"); setErr != nil {
				job.LogError("Failed to set job as completed: %v", setErr)
			}
		} else {
			e.trace("jobWorker(queue %s@%p, worker #%d): job %s execution problem: %v, status: %s, retries: %d, max retries: %d", queue.config.Name, queue, workerID, job.GetJobID(), err, status, job.priv.Retries, job.priv.MaxRetries)

			if ctxErr == context.DeadlineExceeded || (job.GetTypePtr().TimeoutSec > 0 && time.Since(startTime) >= timeout) {
				if setErr := job.setTimedOut(""); setErr != nil {
					job.LogError("Failed to set job as timed out: %v", setErr)
				}
			} else if status == JobStatusCanceling {
				if setErr := job.setCanceled(""); setErr != nil {
					job.LogError("Failed to set job as canceled: %v", setErr)
				}
			} else if status == JobStatusSuspending {
				if setErr := job.setSuspended(""); setErr != nil {
					job.LogError("Failed to set job as suspended: %v", setErr)
				}
			} else if status == JobStatusTimedOut || status == JobStatusCanceled {
				// OK, do not retry timed out or canceled jobs
			} else if job.priv.Retries < job.priv.MaxRetries {
				e.trace("waiting %d seconds before retrying job %s ...", job.priv.RetryDelaySec, job.GetJobID())
				job.setRetrying("")

				time.AfterFunc(time.Duration(job.priv.RetryDelaySec)*time.Second, func() {
					if job, errx := e.lockJob(job, 0); errx != nil {
						job.LogError("Failed to lock job: %v", errx)
					}
					if job.GetStatus() == JobStatusRetrying {
						e.trace("retrying job %s after %d seconds ...", job.GetJobID(), job.priv.RetryDelaySec)
						e.wakeUp(job)
					}
					e.unlockJob(job)
				})
			} else {
				if setErr := job.setFailed(execErr.Error()); setErr != nil {
					job.LogError("Error on trying to set job as 'failed': %v", setErr)
				}
			}
		}

		// Job is done, unlock it.
		e.unlockJob(job)
	}
}

func (e *jobsExecutor) newJob(ctx context.Context, idempotencyKey uuid.UUID, jobType *JobType, paramsStr string) (*JobObj, errorx.Error) {
	job, errx := newJob(ctx, idempotencyKey, jobType, paramsStr)
	if errx != nil {
		return nil, errx
	}
	e.getJob(job)
	e.putJob(job)
	return job, nil
}

// executeJob runs the job's worker function.
// It creates a progress channel that updates job progress.
func (e *jobsExecutor) executeJob(ctx context.Context, job *JobObj) errorx.Error {
	e.trace("executeJob(%s@%p): executing job ...", job.priv.JobID, job)
	status := job.GetStatus()
	if status != JobStatusRunning {
		return errorx.NewErrInternalServerError("job %s is not running, it is %s", job.priv.JobID, status)
	}

	progress := make(chan float32)
	defer close(progress)

	// Propagate progress updates.
	go func() {
		for p := range progress {
			if err := job.setProgress(p); err != nil {
				job.LogWarn("executeJob(%s): Failed to set progress: %v", job.priv.JobID, err)
			}
		}
	}()

	if job.GetTypePtr() != nil && job.GetTypePtr().WorkerExecutionCallback != nil {
		// Execute the job-specific worker.
		errx := job.GetTypePtr().WorkerExecutionCallback(ctx, job, progress)
		if errx != nil {
			return errx
		}
	}

	return nil
}

func (e *jobsExecutor) wakeUp(job *JobObj) errorx.Error {
	e.trace("wakeUp(%s)", job.priv.JobID)
	if e.jobTryLock(job) {
		panic("wakeUp() must be called on a locked job")
	}

	queue, err := e.getQueue(job)
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	// Initialize the job in persistent storage.
	if err := job.schedule(); err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}
	queue.wakeUp()
	return nil
}

// schedule adds a job to the appropriate queue.
// job.GetQueue() determines which queue to use.
func (e *jobsExecutor) schedule(ctx context.Context, job *JobObj) errorx.Error {
	e.trace("schedule(%s)", job.GetJobID())
	_, errx := e.lockJob(job, 0)
	if errx != nil {
		return errx
	}
	defer e.unlockJob(job)

	return e.wakeUp(job)
}

func (e *jobsExecutor) try(ctx context.Context, jobID uuid.UUID, lockDuration time.Duration, f func(ctx context.Context, job *JobObj) errorx.Error) errorx.Error {
	start := time.Now()

	var job *JobObj
	var errx errorx.Error

	tryDuration := min(lockDuration, 1*time.Second)

	for time.Since(start) < tryDuration {
		job, errx = e.lockJobByID(jobID, lockDuration)
		if errx == nil {
			errx = f(ctx, job)
			e.unlockJob(job)
		}

		if errx != nil {
			switch errx.(type) {
			case *errorx.ErrLocked:
				time.Sleep(time.Millisecond * 100)
				continue
			default:
				return errx
			}
		}
		return nil
	}

	if lockDuration == 0 {
		// No timeout, do not give up and try to lock the job anyway
		job, errx = e.lockJobByID(jobID, 0)
		defer e.unlockJob(job)
		if errx == nil {
			errx = f(ctx, job)
		}
		return errx
	}

	return errorx.NewErrLocked("job %s is locked: %v", jobID, errx)
}

func (e *jobsExecutor) cancel(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	e.trace("cancel(%s, %s)", jobID, reason)
	return e.try(ctx, jobID, 2*time.Second, func(ctx context.Context, job *JobObj) errorx.Error {
		status := job.getStatus()
		switch status {
		case JobStatusInit, JobStatusSuspended, JobStatusWaiting, JobStatusRetrying:
			return job.setStatus(JobStatusCanceled, reason)
		case JobStatusRunning, JobStatusResuming, JobStatusLocked:
			errx := job.setStatus(JobStatusCanceling, reason)
			if errx != nil {
				return errx
			}
			job.cancel()
			return nil
		case JobStatusSkipped, JobStatusCanceling, JobStatusCanceled, JobStatusFailed, JobStatusTimedOut, JobStatusCompleted, JobStatusDeleted:
			return nil // skip
		case JobStatusSuspending:
			return errorx.NewErrLocked("job is in '%s' state and can not be canceled, try again later", status)
		default:
			return errorx.NewErrBadRequest("job is in '%s' state and can not be canceled", status)
		}
	})
}

func (e *jobsExecutor) suspend(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	e.trace("suspend(%s, %s)", jobID, reason)
	return e.try(ctx, jobID, 2*time.Second, func(ctx context.Context, job *JobObj) errorx.Error {
		if !job.GetTypePtr().WorkerIsSuspendable {
			return errorx.NewErrMethodNotAllowed(fmt.Sprintf("job type '%s' doesn't support suspend/resume", job.GetType()))
		}

		status := job.getStatus()

		switch status {
		case JobStatusSuspending, JobStatusSuspended, JobStatusSkipped, JobStatusCanceling, JobStatusCanceled, JobStatusFailed, JobStatusTimedOut, JobStatusCompleted, JobStatusDeleted:
			return nil // skip
		case JobStatusInit, JobStatusWaiting, JobStatusRetrying:
			if err := job.setSuspended(reason); err != nil {
				return errorx.NewErrInternalServerError(err.Error())
			}
		case JobStatusLocked, JobStatusRunning:
			if err := job.setSuspending(reason); err != nil {
				return errorx.NewErrInternalServerError(err.Error())
			}
			job.cancel()
		default:
			return errorx.NewErrLocked("job is in '%s' state and can not be suspended, try again later", status)
		}

		return nil
	})
}

func (e *jobsExecutor) resume(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	e.trace("resume(%s, %s)", jobID, reason)
	return e.try(ctx, jobID, 2*time.Second, func(ctx context.Context, job *JobObj) errorx.Error {
		if !job.GetTypePtr().WorkerIsSuspendable {
			return errorx.NewErrMethodNotAllowed(fmt.Sprintf("job type '%s' doesn't support suspend/resume", job.GetType()))
		}

		status := job.getStatus()

		switch status {
		case JobStatusInit, JobStatusWaiting, JobStatusRunning, JobStatusResuming, JobStatusLocked, JobStatusCanceling, JobStatusRetrying:
			return nil
		case JobStatusSuspended:
			errx := job.setStatus(JobStatusResuming, "")
			if errx != nil {
				return errx
			}

			if job.GetTypePtr().WorkerInitCallback != nil {
				job.LogDebug("initializing job worker on resume")
				errx = job.GetTypePtr().WorkerInitCallback(ctx, job)
				if errx != nil {
					errx := job.setStatus(JobStatusFailed, errx.Error())
					if errx != nil {
						return errx
					}
					return errx
				}
			}

			// Set the job status to resumed (waiting)
			errx = job.setStatus(JobStatusWaiting, reason)
			if errx != nil {
				return errx
			}

			return e.wakeUp(job)

		case JobStatusSuspending:
			return errorx.NewErrLocked("job is in '%s' state and can not be resumed, try again later", status)
		default:
			return errorx.NewErrBadRequest("job is in '%s' state and can not be resumed", status)
		}
	})
}

func (e *jobsExecutor) delete(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	e.trace("delete(%s, %s)", jobID, reason)
	return e.try(ctx, jobID, 2*time.Second, func(ctx context.Context, job *JobObj) errorx.Error {
		status := job.getStatus()
		switch status {
		case JobStatusDeleted:
			return nil
		case JobStatusInit, JobStatusWaiting, JobStatusSuspended, JobStatusSkipped, JobStatusCanceled, JobStatusFailed, JobStatusTimedOut, JobStatusCompleted, JobStatusRetrying:
			return job.delete()
		default:
			return errorx.NewErrLocked("job is in '%s' state and can not be deleted, try again later", status)
		}
	})
}

func (e *jobsExecutor) setResult(ctx context.Context, jobID uuid.UUID, result interface{}) errorx.Error {
	e.trace("setResult(%s, %v)", jobID, result)
	return e.try(ctx, jobID, 0, func(ctx context.Context, job *JobObj) errorx.Error {
		return job.setResult(result)
	})
}

func (e *jobsExecutor) setProgress(ctx context.Context, jobID uuid.UUID, progress float32) errorx.Error {
	e.trace("setProgress(%s, %f)", jobID, progress)
	return e.try(ctx, jobID, 0, func(ctx context.Context, job *JobObj) errorx.Error {
		return job.setProgress(progress)
	})
}

func (e *jobsExecutor) setLockedBy(ctx context.Context, jobID uuid.UUID, lockedBy uuid.UUID) errorx.Error {
	e.trace("setLockedBy(%s): locking by: %s", jobID, lockedBy)
	return e.try(ctx, jobID, 0, func(ctx context.Context, job *JobObj) errorx.Error {
		return job.setLockedBy(lockedBy)
	})
}

func (e *jobsExecutor) setUnlocked(ctx context.Context, jobID uuid.UUID) errorx.Error {
	e.trace("setUnlocked(%s)", jobID)
	return e.try(ctx, jobID, 0, func(ctx context.Context, job *JobObj) errorx.Error {
		status := job.getStatus()
		switch status {
		case JobStatusLocked:
			return job.setUnlocked()
		case JobStatusRunning, JobStatusWaiting, JobStatusInit, JobStatusResuming, JobStatusCanceling, JobStatusRetrying:
			return nil // skip
		default:
			return errorx.NewErrBadRequest("job is in '%s' state and can not be unlocked", status)
		}
	})
}

func (e *jobsExecutor) setRetryPolicy(ctx context.Context, jobID uuid.UUID, retryDelay time.Duration, maxRetries int, timeout time.Duration) errorx.Error {
	e.trace("setRetryPolicy(%s, %s, %d, %s)", jobID, retryDelay, maxRetries, timeout)
	return e.try(ctx, jobID, 0, func(ctx context.Context, job *JobObj) errorx.Error {
		return job.setRetryPolicy(retryDelay, maxRetries, timeout)
	})
}

func (e *jobsExecutor) setSkipped(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	e.trace("setSkipped(%s, %s)", jobID, reason)
	return e.try(ctx, jobID, 0, func(ctx context.Context, job *JobObj) errorx.Error {
		return job.setSkipped(reason)
	})
}

// getJobByID retrieves a JobObj by its jobID from the in-memory jobs map if present,
// otherwise fetches it from the database and caches it in the jobs map.
// This function ensures that the returned JobObj is the canonical instance used by the executor.
// If the job is not found in memory or the database, an error is returned.
func (e *jobsExecutor) getJobByID(jobID uuid.UUID) (*JobObj, errorx.Error) {
	e.mu.Lock()
	if job, ok := e.jobsMap[jobID]; ok {
		e.mu.Unlock()
		return job, nil
	}
	e.mu.Unlock()

	// Fallback to DB fetch
	job, errx := getJob(jobID)
	if errx != nil {
		logging.Debug("getJobByID(%s) - JOB NOT FOUND, stack: %s", jobID, string(debug.Stack()))
		return nil, errx
	}
	e.trace("getJobByID(%s) - from DB, %s@%p, stack: %s", jobID, job.priv.JobID, job, string(debug.Stack()))

	e.mu.Lock()
	e.jobsMap[jobID] = job
	e.mu.Unlock()

	return job, nil
}

// getJob retrieves a job from the in-memory cache if present, or adds the provided job to the cache if not.
// This function ensures that only one instance of a job with a given JobID exists in the cache at a time.
// The caller must invoke putJob() when the job is no longer needed to allow proper cache management.
func (e *jobsExecutor) getJob(job *JobObj) (*JobObj, errorx.Error) {
	e.trace("getJob(%s@%p), stack: %s", job.priv.JobID, job, string(debug.Stack()))
	e.mu.Lock()
	if j, ok := e.jobsMap[job.priv.JobID]; ok {
		e.mu.Unlock()
		return j, nil
	}
	e.jobsMap[job.priv.JobID] = job
	e.mu.Unlock()

	return job, nil
}

// The putJob() function removes the job from the cache if it's in the final state.
func (e *jobsExecutor) putJob(job *JobObj) {
	if job == nil {
		panic("putJob called with nil job")
	}

	e.mu.Lock()

	// put only jobs that are stored in DB, keep others in memory as they are
	// frequently accessed

	if _, ok := e.jobsMap[job.priv.JobID]; !ok {
		panic(fmt.Sprintf("putJob(%s) - job not found in map", job.priv.JobID))
	}

	s := job.getStatus()

	if s == StatusSuspended || s == StatusSkipped || s == StatusCanceled || s == StatusFailed || s == StatusTimedOut || s == StatusCompleted || s == StatusDeleted {
		delete(e.jobsMap, job.priv.JobID)
		e.trace("putJob(%s@%p) - detached from cache as status is '%s', stack: %s", job.priv.JobID, job, s, string(debug.Stack()))
	} else {
		e.trace("putJob(%s@%p) - keeping in cache as status is '%s', stack: %s", job.priv.JobID, job, s, string(debug.Stack()))
	}

	e.mu.Unlock()
}

// The lockJobByID() function retrieves the appropriate JobObj from the database and locks it.
// If the database fails to find or fetch the job, the lock will not be held.
func (e *jobsExecutor) lockJobByID(jobID uuid.UUID, timeout time.Duration) (*JobObj, errorx.Error) {
	var jobLock *utils.DebugMutex
	var ok bool

	e.mu.Lock()
	if jobLock, ok = e.jobsLockMap[jobID]; !ok {
		jobLock = &utils.DebugMutex{}
		e.jobsLockMap[jobID] = jobLock
	}
	e.mu.Unlock()

	if timeout == 0 {
		jobLock.Lock()
	} else {
		if !jobLock.TryLockWithTimeout(timeout) {
			return nil, errorx.NewErrLocked("job lock timeout")
		}
	}

	e.trace("lockJobByID(%s), stack: %s", jobID, debug.Stack())

	job, errx := e.getJobByID(jobID)
	if errx != nil {
		// Don't call unlockJob with nil job - just unlock the mutex directly
		jobLock.Unlock()
		return nil, errx
	}

	return job, nil
}

// the lockJob() locks given job
func (e *jobsExecutor) lockJob(job *JobObj, timeout time.Duration) (*JobObj, errorx.Error) {
	var jobLock *utils.DebugMutex
	var ok bool

	e.mu.Lock()
	if jobLock, ok = e.jobsLockMap[job.priv.JobID]; !ok {
		jobLock = &utils.DebugMutex{}
		e.jobsLockMap[job.priv.JobID] = jobLock
	}
	e.mu.Unlock()

	e.trace("lockJob(%s@%p), stack: %s", job.priv.JobID, job, string(debug.Stack()))

	if timeout == 0 {
		jobLock.Lock()
	} else if !jobLock.TryLockWithTimeout(timeout) {
		return nil, errorx.NewErrLocked("job lock timeout")
	}

	return e.getJob(job)
}

func (e *jobsExecutor) jobTryLock(job *JobObj) bool {
	var jobLock *utils.DebugMutex
	var ok bool

	e.mu.Lock()
	if jobLock, ok = e.jobsLockMap[job.priv.JobID]; !ok {
		jobLock = &utils.DebugMutex{}
		e.jobsLockMap[job.priv.JobID] = jobLock
	}
	e.mu.Unlock()

	return jobLock.TryLock()
}

// The unlockJob() function releases the lock on the job.
func (e *jobsExecutor) unlockJob(job *JobObj) {
	if job == nil {
		panic("unlockJob called with nil job")
	}

	var jobLock *utils.DebugMutex
	var ok bool

	e.putJob(job)

	e.mu.Lock()
	if jobLock, ok = e.jobsLockMap[job.priv.JobID]; !ok {
		panic(fmt.Sprintf("job lock for job %s not found", job.priv.JobID))
	}
	e.mu.Unlock()

	e.trace("unlockJob(%s@%p), stack: %s", job.priv.JobID, job, string(debug.Stack()))

	jobLock.Unlock()
}

func (e *jobsExecutor) getByIdempotencyKey(ctx context.Context, idempotencyKey uuid.UUID) (*JobObj, errorx.Error) {
	job := &JobObj{}
	err := db.DB().First(&job.priv, "idempotency_key = ?", idempotencyKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.NewErrNotFound("job not found")
		}
		return nil, errorx.NewErrInternalServerError(err.Error())
	}

	return e.getJobByID(job.GetJobID())
}

// Add this method to the JobsExecutor struct
func (e *jobsExecutor) wait(ctx context.Context, jobID uuid.UUID, timeout time.Duration) errorx.Error {
	pollingInterval := min(timeout/2, 1*time.Second)

	e.trace("wait(%s): waiting for job to complete with polling interval %.2f seconds...", jobID, pollingInterval.Seconds())

	// Create a context with the job's timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check job status periodically
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errorx.NewErrLocked("job wait operation canceled")
		case <-timeoutCtx.Done():
			currentJob, errx := e.lockJobByID(jobID, 0)
			if errx != nil {
				return errx
			}
			status := currentJob.getStatus()
			if currentJob.statusIsFinal(status) {
				e.trace("wait(%s): wait operation completed, job status: %s", jobID, status)
				e.unlockJob(currentJob)
				return nil
			} else {
				e.trace("wait(%s): wait timed out after %.2f seconds, job has not been completed yet (%s)", jobID, timeout.Seconds(), status)
				e.unlockJob(currentJob)
				return errorx.NewErrLocked("job has not been completed yet...")
			}
		case <-ticker.C:
			currentJob, errx := e.lockJobByID(jobID, 100*time.Millisecond)
			if errx != nil {
				return errx
			}

			status := currentJob.getStatus()
			e.trace("wait(%s): wait tick occurred, operation status: %s", jobID, status)
			if currentJob.statusIsFinal(status) {
				e.trace("wait(%s): wait operation completed, job status: %s", jobID, status)
				e.unlockJob(currentJob)
				return nil
			}

			e.unlockJob(currentJob)
		}
	}
}

// Shutdown signals all workers to stop.
func (e *jobsExecutor) shutdown() {
	e.trace("shutdown()")
	// Check if shutdown channel is not already closed
	select {
	case <-e.shutdownSignal:
		// Channel already closed, do nothing
	default:
		// Channel is open, safe to close
		close(e.shutdownSignal)
	}
	time.Sleep(100 * time.Millisecond) // Allow workers to shut down properly
}

// ----------------------------------------------------------------------
// Convenience wrapper functions

// Public interface available for the modules

func JENewJob(ctx context.Context, idempotencyKey uuid.UUID, jobType *JobType, paramsStr string) (*JobObj, errorx.Error) {
	return je.newJob(ctx, idempotencyKey, jobType, paramsStr)
}

func JEScheduleJob(ctx context.Context, job *JobObj) errorx.Error {
	return je.schedule(ctx, job)
}

func JEWaitJob(ctx context.Context, jobID uuid.UUID, timeout time.Duration) errorx.Error {
	return je.wait(ctx, jobID, timeout)
}

func JEGetJobByID(ctx context.Context, jobID uuid.UUID) (*JobObj, errorx.Error) {
	return je.getJobByID(jobID)
}

func JEGetJob(ctx context.Context, job *JobObj) (*JobObj, errorx.Error) {
	return je.getJob(job)
}

func jeGetJobByIdempotencyKey(ctx context.Context, idempotencyKey uuid.UUID) (*JobObj, errorx.Error) {
	return je.getByIdempotencyKey(ctx, idempotencyKey)
}

func JECancelJob(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	return je.cancel(ctx, jobID, reason)
}

func JESetSkipped(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	return je.setSkipped(ctx, jobID, reason)
}

func JESetResult(ctx context.Context, jobID uuid.UUID, result interface{}) errorx.Error {
	return je.setResult(ctx, jobID, result)
}

func JESetProgress(ctx context.Context, jobID uuid.UUID, progress float32) errorx.Error {
	return je.setProgress(ctx, jobID, progress)
}

func JESetLockedBy(ctx context.Context, jobID uuid.UUID, lockedBy uuid.UUID) errorx.Error {
	return je.setLockedBy(ctx, jobID, lockedBy)
}

func JESetUnlocked(ctx context.Context, jobID uuid.UUID) errorx.Error {
	return je.setUnlocked(ctx, jobID)
}

// Private interface available for the job executor itself

func jeDeleteJob(ctx context.Context, jobID uuid.UUID, reason string) errorx.Error {
	return je.delete(ctx, jobID, reason)
}

func jeSuspendJob(ctx context.Context, jobID uuid.UUID) errorx.Error {
	return je.suspend(ctx, jobID, "")
}

func jeResumeJob(ctx context.Context, jobID uuid.UUID) errorx.Error {
	return je.resume(ctx, jobID, "")
}

func jeSetRetryPolicy(ctx context.Context, job *JobObj, retryDelay time.Duration, maxRetries int, timeout time.Duration) errorx.Error {
	if job.priv.Status == JobStatusInit {
		// FIXME: this is a hack to allow setting retry policy for jobs that are not stored into DB yet
		return job.setRetryPolicy(retryDelay, maxRetries, timeout)
	} else {
		return je.setRetryPolicy(ctx, job.priv.JobID, retryDelay, maxRetries, timeout)
	}
}

func JEShutdown() {
	jobExecutorMu.Lock()
	defer jobExecutorMu.Unlock()

	close(je.shutdownSignal)
}
