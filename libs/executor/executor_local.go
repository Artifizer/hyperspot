package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/config"
	jobs "github.com/hypernetix/hyperspot/libs/job"
)

var localExecutors = make(map[string]*ConfigCodeExecutorLocal)

// LocalExecutor implements CodeExecutor for local execution.
type LocalExecutor struct {
	DockerExecutorID uuid.UUID           `json:"docker_executor_id" gorm:"type:uuid;primary_key"`
	CodeExecutor     CodeExecutionResult `gorm:"foreignKey:DockerExecutorID"`
	executorPath     string
	timeoutSec       float64
	workDir          string
}

// NewLocalExecutor creates a new LocalExecutor.
func newLocalExecutor(cfg *ConfigCodeExecutorLocal) *LocalExecutor {
	return &LocalExecutor{
		CodeExecutor: newCodeExecutor(cfg.ID, cfg.Name, cfg, ExecutorTypeLocal),
		executorPath: cfg.CodeExecutorPath,
		timeoutSec:   cfg.TimeoutSec,
	}
}

// Init sets the execution timeout.
func (e *LocalExecutor) Init() error {
	// Create workDir under the configured home directory
	homeDir := config.Get().Server.HomeDir
	executorDir := filepath.Join(homeDir, "local_executor")
	if err := os.MkdirAll(executorDir, 0755); err != nil {
		return fmt.Errorf("failed to create executor directory: %w", err)
	}

	workDir, err := os.MkdirTemp(executorDir, "run-")
	if err != nil {
		return fmt.Errorf("failed to create local executor workDir: %w", err)
	}
	e.workDir = workDir
	return nil
}

func (e *LocalExecutor) InstallRuntime(ctx context.Context, contextJob *jobs.JobObj) error {
	return nil // FIXME: not supported for now
}

// GetExecutorId returns an empty string as localhost doesn't have an executor id.
func (e *LocalExecutor) GetExecutorId() string {
	return ""
}

// ExecutePythonCode writes the code to a temporary directory and executes it.
func (e *LocalExecutor) ExecutePythonCode(ctx context.Context, contextJob *jobs.JobObj, code string) (*CodeExecutionResult, error) {
	startTime := time.Now()

	// Write code to file
	codePath := filepath.Join(e.workDir, "code.py")
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write code file: %w", err)
	}

	// Execute the code using python
	cmd := exec.Command("python", "code.py")
	cmd.Dir = e.workDir
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Update execution results
	e.CodeExecutor.Completed = true
	e.CodeExecutor.Success = exitCode == 0
	e.CodeExecutor.ExitCode = exitCode
	e.CodeExecutor.Stdout = string(output)
	e.CodeExecutor.Stderr = ""
	e.CodeExecutor.StartTimeMs = startTime.UnixMilli()
	e.CodeExecutor.EndTimeMs = time.Now().UnixMilli()
	e.CodeExecutor.DurationSec = time.Since(startTime).Seconds()

	if !e.CodeExecutor.Success {
		e.CodeExecutor.Error = fmt.Sprintf("execution failed with exit code %d", exitCode)
	}

	return &e.CodeExecutor, nil
}

// Automatically register the localhost executor.
func initLocalExecutor() error {
	for name, cfg := range executorConfig.Local {
		cfg.Name = name
		localExecutors[cfg.Name] = cfg
		RegisterExecutor(cfg.Name, func() CodeExecutorIface {
			return newLocalExecutor(cfg)
		})
	}
	return nil
}
