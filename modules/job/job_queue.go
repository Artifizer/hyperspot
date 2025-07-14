package job

import "github.com/hypernetix/hyperspot/libs/utils"

// jobQueueWorker represents a pre-created jobQueueWorker that is attached to a specific queue.
type jobQueueWorker struct {
	// Store the name of the queue this worker belongs to.
	queue *JobQueue
	// (Additional fields could be added if needed.)
}

type JobQueueConfig struct {
	Name     JobQueueName
	Capacity int
}

type JobQueue struct {
	config       JobQueueConfig
	mu           utils.DebugMutex
	workers      []*jobQueueWorker // the pool of workers for this queue
	waitingJobCh chan struct{}     // channel to signal new waiting jobs
}

// NewJobQueue creates a new JobQueue.
func NewJobQueue(config *JobQueueConfig) *JobQueue {
	return &JobQueue{
		config:       *config,
		workers:      make([]*jobQueueWorker, 0, config.Capacity),
		waitingJobCh: make(chan struct{}, 1),
	}
}

// AddJob adds a new job to the waiting list.
func (q *JobQueue) wakeUp() {
	select {
	case q.waitingJobCh <- struct{}{}:
		// send if possible
	default:
		// channel full, skip send
	}
}

type JobQueueName string

const (
	JobQueueNameCompute     JobQueueName = "compute"
	JobQueueNameMaintenance JobQueueName = "maintenance"
	JobQueueNameDownload    JobQueueName = "download"
)

var (
	JobQueueCompute     = &JobQueueConfig{Name: JobQueueNameCompute, Capacity: 1}
	JobQueueMaintenance = &JobQueueConfig{Name: JobQueueNameMaintenance, Capacity: 5}
	JobQueueDownload    = &JobQueueConfig{Name: JobQueueNameDownload, Capacity: 10}
)
