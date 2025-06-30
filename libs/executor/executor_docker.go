package executor

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
	jobs "github.com/hypernetix/hyperspot/libs/job"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
)

// DockerExecutor extends CodeExecutor with Docker-specific functionality
type DockerExecutor struct {
	DockerExecutorID uuid.UUID           `json:"docker_executor_id" gorm:"type:uuid;primary_key"`
	CodeExecutor     CodeExecutionResult `gorm:"foreignKey:DockerExecutorID"`
	Image            string              `json:"image" gorm:"not null"`
	cli              *client.Client
	mu               utils.DebugMutex
}

var dockerExecutors = make(map[string]*ConfigCodeExecutorDocker)

// NewDockerExecutor creates a new DockerExecutor
func newDockerExecutor(name string) *DockerExecutor {
	if cfg, ok := dockerExecutors[name]; ok {
		executor := &DockerExecutor{
			CodeExecutor: newCodeExecutor(cfg.ID, cfg.Name, cfg, ExecutorTypeDocker),
			Image:        cfg.Image,
		}
		if err := executor.Init(); err != nil {
			logging.Error("failed to initialize docker executor '%s': %s", name, err.Error())
			return nil
		}
		return executor
	}
	logging.Error("docker executor '%s' not found", name)
	return nil
}

func (e *DockerExecutor) Init() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create docker client: %s", err.Error())
	}
	e.cli = cli

	return nil
}

func (e *DockerExecutor) InstallRuntime(ctx context.Context, contextJob *jobs.JobObj) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if image exists locally
	images, err := e.cli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list docker images: %w", err)
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == e.Image {
				return nil // Image found
			}
		}
	}
	err = fmt.Errorf("image %s not found", e.Image)
	if err == nil {
		return nil
	}

	// Schedule a job to download the image
	downloadJob, err := ScheduleDockerImageDownloadJob(ctx, e.CodeExecutor.Name)
	if err != nil {
		if contextJob != nil {
			contextJob.LogError("failed to schedule docker image download job: %s", err.Error())
		} else {
			logging.Error("failed to schedule docker image download job: %s", err.Error())
		}
		return err
	}
	if contextJob != nil {
		contextJob.LogInfo("docker image not found, scheduling download job: %s", downloadJob.GetJobID())
	} else {
		logging.Info("docker image not found, scheduling download job: %s", downloadJob.GetJobID())
	}
	if err != nil {
		return fmt.Errorf("failed to schedule docker image download job: %s", err.Error())
	}

	if contextJob != nil {
		contextJob.SetLockedBy(ctx, downloadJob.GetJobID())
		jobs.JEWaitJob(ctx, downloadJob.GetJobID(), downloadJob.GetTimeoutSec())
		contextJob.SetUnlocked(ctx)
	} else {
		jobs.JEWaitJob(ctx, downloadJob.GetJobID(), downloadJob.GetTimeoutSec())
	}

	return nil
}

// GetExecutorId returns the Docker container id
func (e *DockerExecutor) GetExecutorId() string {
	return e.Image
}

// ExecutePythonCode builds and runs the Docker container executing the Python code
func (e *DockerExecutor) ExecutePythonCode(ctx context.Context, contextJob *jobs.JobObj, code string) (*CodeExecutionResult, error) {
	e.InstallRuntime(ctx, contextJob)

	e.mu.Lock()
	defer e.mu.Unlock()

	startTime := time.Now()

	cfg, ok := e.CodeExecutor.ConfigPtr.(*ConfigCodeExecutorDocker)
	if !ok {
		return nil, fmt.Errorf("docker executor config is nil")
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()

	// Create Docker container
	resp, err := e.cli.ContainerCreate(ctx,
		&container.Config{
			Image: e.Image,
			Cmd:   []string{"python", "-c", code},
		},
		&container.HostConfig{
			Resources: container.Resources{
				Memory:    int64(cfg.MemLimitMB * 1024 * 1024),
				NanoCPUs:  int64(cfg.CPUsLimit * 1e9),
				PidsLimit: &cfg.PIDsLimit,
			},
		},
		nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	e.CodeExecutor.ExecutorId = resp.ID

	// Ensure container is removed after execution
	defer e.cleanup(context.Background())

	// Start container
	if err := e.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := e.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("container wait failed: %w", err)
		}
	case status := <-statusCh:
		statusCode = status.StatusCode
	}

	// Get container logs (combining stdout/stderr)
	out, err := e.cli.ContainerLogs(context.Background(), resp.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}
	defer out.Close()

	// Create stdout and stderr buffers
	var stdout, stderr bytes.Buffer
	// Copy multiplexed output to stdout and stderr buffers
	_, err = stdcopy.StdCopy(&stdout, &stderr, out)
	if err != nil {
		return nil, fmt.Errorf("failed to read container logs: %w", err)
	}

	// Update execution results
	e.CodeExecutor.Completed = true
	e.CodeExecutor.Success = statusCode == 0
	e.CodeExecutor.ExitCode = int(statusCode)
	e.CodeExecutor.Stdout = stdout.String()
	e.CodeExecutor.Stderr = stderr.String()
	e.CodeExecutor.StartTimeMs = startTime.UnixMilli()
	e.CodeExecutor.EndTimeMs = time.Now().UnixMilli()
	e.CodeExecutor.DurationSec = time.Since(startTime).Seconds()

	if !e.CodeExecutor.Success {
		e.CodeExecutor.Error = fmt.Sprintf("execution failed with exit code %d", statusCode)
	}

	return &e.CodeExecutor, nil
}

// cleanup removes the created container
func (e *DockerExecutor) cleanup(ctx context.Context) error {
	if e.CodeExecutor.ExecutorId != "" {
		// Remove container
		_ = e.cli.ContainerRemove(ctx, e.CodeExecutor.ExecutorId, types.ContainerRemoveOptions{
			Force: true,
		})
		e.CodeExecutor.ExecutorId = ""
	}
	return nil
}

// InitDockerExecutor registers DockerExecutor in the global registry
func initDockerExecutor() error {
	for name, cfg := range executorConfig.Docker {
		cfg.Name = name
		dockerExecutors[cfg.Name] = cfg
		RegisterExecutor(cfg.Name, func() CodeExecutorIface {
			return newDockerExecutor(cfg.Name)
		})
	}
	RegisterDockerImageJob()
	return nil
}
