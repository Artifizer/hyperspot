package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/core"
	jobs "github.com/hypernetix/hyperspot/module/job"
)

type ExecutorType string

const (
	ExecutorTypeDocker ExecutorType = "docker"
	ExecutorTypeSSH    ExecutorType = "ssh"
	ExecutorTypeLocal  ExecutorType = "localhost"
)

type CodeExecutionResult struct {
	ID           uuid.UUID    `json:"id" gorm:"type:uuid;primary_key"`
	ExecutorType ExecutorType `json:"executor_type" gorm:"not null"`
	ExecutorId   string       `json:"executor_id" gorm:"not null"`
	RuntimeId    string       `json:"runtime_id" gorm:"not null"`
	Name         string       `json:"name" gorm:"not null"`
	ConfigId     uuid.UUID    `json:"config_id" gorm:"type:uuid;not null"`
	ConfigPtr    interface{}  `json:"-" gorm:"-"`
	Completed    bool         `json:"completed" default:"false"`
	Success      bool         `json:"success" default:"false"`
	ExitCode     int          `json:"exit_code" default:"0"`
	Stdout       string       `json:"stdout" default:""`
	Stderr       string       `json:"stderr" default:""`
	Hostname     string       `json:"hostname"`
	StartTimeMs  int64        `json:"start_time" gorm:"index" doc:"unix timestamp in milliseconds"`
	EndTimeMs    int64        `json:"end_time" gorm:"index" doc:"unix timestamp in milliseconds"`
	DurationSec  float64      `json:"duration_sec"`
	Error        string       `json:"error,omitempty"`
}

type CodeExecutorIface interface {
	Init() error
	InstallRuntime(ctx context.Context, contextJob *jobs.JobObj) error
	ExecutePythonCode(ctx context.Context, contextJob *jobs.JobObj, code string) (*CodeExecutionResult, error)
	GetExecutorId() string
}

var executorRegistry = make(map[string]func() CodeExecutorIface)

func newCodeExecutor(id uuid.UUID, name string, cfg interface{}, executorType ExecutorType) CodeExecutionResult {
	return CodeExecutionResult{
		ID:           uuid.New(),
		ExecutorType: executorType,
		Name:         name,
		ConfigId:     id,
		ConfigPtr:    cfg,
		Completed:    false,
		Success:      false,
		ExitCode:     0,
		Stdout:       "",
		Stderr:       "",
		Hostname:     "",
		StartTimeMs:  time.Now().UnixMilli(),
		EndTimeMs:    0,
		DurationSec:  0,
		Error:        "",
	}
}

func RegisterExecutor(name string, factory func() CodeExecutorIface) {
	if _, exists := executorRegistry[name]; !exists {
		executorRegistry[name] = factory
	}
}

func NewCodeExecutor(ctx context.Context, contextJob *jobs.JobObj, name string) (CodeExecutorIface, error) {
	if factory, ok := executorRegistry[name]; ok {
		executor := factory()
		if err := executor.Init(); err != nil {
			return nil, fmt.Errorf("failed to initialize executor: %w", err)
		}
		return executor, nil
	}

	availableExecutors := make([]string, 0, len(executorRegistry))
	for name := range executorRegistry {
		availableExecutors = append(availableExecutors, name)
	}

	return nil, fmt.Errorf("executor not found: %s, available executors: %v", name, availableExecutors)
}

func GetCodeExecutor(ctx context.Context, contextJob *jobs.JobObj, name string) (CodeExecutorIface, error) {
	return NewCodeExecutor(ctx, contextJob, name)
}

// FIXME: executors require it's own logger!

func init() {
	core.RegisterModule(&core.Module{
		Name:       "code_executor",
		Migrations: []interface{}{
			//			&config.ConfigCodeExecutorDocker{},
			//			&config.ConfigCodeExecutorSSH{},
			//			&config.ConfigCodeExecutorLocal{},
		},
	})

	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			//			&DockerExecutor{},
			//&DockerImageDownloadJobParams{},
		},
		InitMain: initDockerExecutor,
		Name:     "docker_executor",
	})

	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			// &LocalExecutor{},
		},
		InitMain: initLocalExecutor,
		Name:     "local_executor",
	})

	core.RegisterModule(&core.Module{
		Migrations: []interface{}{
			//&SSHExecutor{},
		},
		InitMain: initSSHExecutor,
		Name:     "ssh_executor",
	})
}
