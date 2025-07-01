package test_module

import (
	"context"
	"fmt"
	"time"

	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/job"
	"github.com/hypernetix/hyperspot/libs/utils"
)

var testModuleJobQueue = job.JobQueueName("test-module-job-queue")

// Job group definition
var JOB_GROUP_TEST = &job.JobGroup{
	Name:        "test",
	Queue:       testModuleJobQueue, // Use the dedicated queue with 2 max parallel executors
	Description: "Test jobs group",
}

// TestModuleJobParams represents the parameters for a test job
type TestModuleJobParams struct {
	// Duration in seconds for the job to run
	DurationLimitSec int    `json:"duration_limit_sec,omitempty" default:"10" doc:"Duration in seconds for the job to run"`
	ModelName        string `json:"model_name,omitempty" default:"mock~mock-model-0.5B" doc:"Model name to use for the job"`
}

// testModuleJobWorker is the main worker function for the test job
func testModuleJobWorker(ctx context.Context, worker job.JobWorker, progress chan<- float32) error {
	// Convert job params to the correct type
	j, ok := worker.(*job.JobObj)
	if !ok {
		return fmt.Errorf("internal error, worker is not a job.JobObj")
	}

	params, ok := j.GetParamsPtr().(*TestModuleJobParams)
	if !ok {
		return fmt.Errorf("invalid job parameters type; expected *TestModuleJobParams")
	}

	// Get the total duration in seconds
	totalDuration := params.DurationLimitSec
	if totalDuration <= 0 {
		totalDuration = 10 // Default to 10 seconds if invalid duration
	}

	// Calculate the number of steps (0.5 second increments)
	steps := totalDuration * 2

	j.LogInfo("Starting test job for %d seconds (%d steps)", totalDuration, steps)

	// Execute the job with 0.5 second increments
	for i := 0; i < steps; i++ {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Continue execution
		}

		// Sleep for 0.25 seconds
		time.Sleep(250 * time.Millisecond)

		// Update progress (0-100%)
		progress := float32(i+1) / float32(steps) * 100
		j.SetProgress(ctx, progress)
	}

	return nil
}

// testModuleJobInit initializes the test job
func testModuleJobInit(ctx context.Context, j *job.JobObj) (job.JobWorker, errorx.Error) {
	// Get and validate job parameters
	jobParams, ok := j.GetParamsPtr().(*TestModuleJobParams)
	if !ok {
		return nil, errorx.NewErrInternalServerError("invalid job parameters type; expected *TestModuleJobParams")
	}

	if jobParams.DurationLimitSec <= 0 {
		j.LogWarn("Invalid duration %d, defaulting to 10 seconds", jobParams.DurationLimitSec)
		jobParams.DurationLimitSec = 10
	}

	return j, nil
}

// RegisterTestModuleJob registers the test job type
func RegisterTestModuleJob() *job.JobType {
	_, err := job.RegisterJobQueue(testModuleJobQueue, 2)
	if err != nil {
		panic(fmt.Sprintf("failed to register job queue: %s", err.Error()))
	}

	job.RegisterJobGroup(JOB_GROUP_TEST)

	jobParams := &TestModuleJobParams{}
	utils.InitStructWithDefaults(jobParams)

	return job.RegisterJobType(
		job.JobTypeParams{
			Group:                     JOB_GROUP_TEST,
			Name:                      "test-module",
			Description:               "Execute a test job that simulates work by sleeping",
			Params:                    jobParams,
			WorkerInitCallback:        testModuleJobInit,
			WorkerExecutionCallback:   testModuleJobWorker,
			WorkerStateUpdateCallback: nil,
			WorkerSuspendCallback:     nil,
			WorkerResumeCallback:      nil,
			WorkerIsSuspendable:       true,
			Timeout:                   time.Hour,
			RetryDelay:                10 * time.Second,
			MaxRetries:                3,
		},
	)
}

// InitTestModuleJobs initializes all test module jobs
func InitTestModuleJobs() error {
	RegisterTestModuleJob()
	return nil
}

// Update test_module.go to call this function:
// Add the following to the InitModule function:
//
//   core.RegisterModule(&core.Module{
//     Name:          "test_module",
//     InitAPIRoutes: registerTestModuleAPIRoutes,
//     InitMain:      InitTestModuleJobs,
//   })
//
