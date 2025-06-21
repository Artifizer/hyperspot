package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	jobs "github.com/hypernetix/hyperspot/libs/job"
	"github.com/hypernetix/hyperspot/libs/logging"
)

var sshExecutors = make(map[string]*ConfigCodeExecutorSSH)

// SSHExecutor implements CodeExecutor for remote SSH execution (stubbed).
type SSHExecutor struct {
	DockerExecutorID uuid.UUID           `json:"docker_executor_id" gorm:"type:uuid;primary_key"`
	CodeExecutor     CodeExecutionResult `gorm:"foreignKey:DockerExecutorID"`
	host             string
	port             int
	username         string
	password         string
	privateKey       string
	timeoutSec       float64
	// In a real implementation, SSH client connection fields would be added here.
}

// NewSSHExecutor creates a new SSHExecutor.
func newSSHExecutor(cfg *ConfigCodeExecutorSSH) *SSHExecutor {
	return &SSHExecutor{
		CodeExecutor: newCodeExecutor(cfg.ID, cfg.Name, cfg, ExecutorTypeSSH),
		host:         cfg.Host,
		port:         cfg.Port,
		username:     cfg.Username,
		password:     cfg.Password,
		privateKey:   cfg.PrivateKey,
		timeoutSec:   cfg.TimeoutSec,
	}
}

// Init sets the timeout for SSH execution.
func (e *SSHExecutor) Init() error {
	logging.Error("SSHExecutor.Init() not supported for now")
	return nil // FIXME: not supported for now
}

func (e *SSHExecutor) InstallRuntime(ctx context.Context, contextJob *jobs.JobObj) error {
	return nil // FIXME: not supported for now
}

// GetExecutorId returns the SSH executor id.
func (e *SSHExecutor) GetExecutorId() string {
	return fmt.Sprintf("%s@%s:%d", e.username, e.host, e.port)
}

// ExecutePythonCode simulates executing Python code via SSH.
func (e *SSHExecutor) ExecutePythonCode(ctx context.Context, contextJob *jobs.JobObj, code string) (*CodeExecutionResult, error) {
	startTime := time.Now()
	// For demonstration, simulate remote execution delay.
	time.Sleep(2 * time.Second) // FIXME: remove this

	// Update execution results
	e.CodeExecutor.Completed = true
	e.CodeExecutor.Success = true
	e.CodeExecutor.ExitCode = 0
	e.CodeExecutor.Stdout = "SSH executor simulated output"
	e.CodeExecutor.Stderr = ""
	e.CodeExecutor.StartTimeMs = startTime.UnixMilli()
	e.CodeExecutor.EndTimeMs = time.Now().UnixMilli()
	e.CodeExecutor.DurationSec = time.Since(startTime).Seconds()

	return &e.CodeExecutor, nil
}

// Automatically register the SSH executor.
func initSSHExecutor() error {
	for name, cfg := range executorConfig.SSH {
		cfg.Name = name
		sshExecutors[cfg.Name] = cfg
		RegisterExecutor(cfg.Name, func() CodeExecutorIface {
			return newSSHExecutor(cfg)
		})
	}
	return nil
}
