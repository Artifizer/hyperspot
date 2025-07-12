package utils

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/hypernetix/hyperspot/libs/logging"
)

type ExecResult struct {
	Stdout   []string
	Stderr   []string
	ExitCode int
	Duration time.Duration
}

type LineBuffer struct {
	rWriter *io.PipeWriter
	lines   []string
	done    chan struct{}
	closed  int32
}

// NewLineBuffer creates a LineBuffer and starts a goroutine to scan written data.
func NewLineBuffer(ctx context.Context) *LineBuffer {
	pr, pw := io.Pipe()
	lb := &LineBuffer{rWriter: pw, done: make(chan struct{})}

	go func() {
		defer close(lb.done)

		s := bufio.NewScanner(pr)

		scanAndAppend := func() bool {
			if s.Scan() {
				lb.lines = append(lb.lines, s.Text())
				return true
			}
			return false
		}

		for {
			select {
			case <-ctx.Done():
				// Context canceled, read remaining lines
				for scanAndAppend() {
				}
				return
			default:
			}

			if !scanAndAppend() {
				break // EOF or error
			}
		}
	}()

	return lb
}

func (lb *LineBuffer) CloseAndGetLines() []string {
	if atomic.CompareAndSwapInt32(&lb.closed, 0, 1) {
		if err := lb.rWriter.Close(); err != nil {
			logging.Error("Failed to close pipe writer: %v", err)
		}
	}

	// Wait for the scanner to finish processing
	<-lb.done
	return lb.lines
}

// Write writes data to the internal pipe for scanning.
func (lb *LineBuffer) Write(p []byte) (int, error) {
	return lb.rWriter.Write(p)
}

func (lb *LineBuffer) Close() error {
	if atomic.CompareAndSwapInt32(&lb.closed, 0, 1) {
		return lb.rWriter.Close()
	}
	return nil
}

// Exec runs a command with args in the given working directory, capturing stdout, stderr, exit code, and duration.
// It respects context for cancellation/timeouts.
func Exec(ctx context.Context, args []string, cwd string) ExecResult {
	start := time.Now()
	if len(args) == 0 {
		return ExecResult{
			Stdout:   nil,
			Stderr:   []string{"no command provided"},
			ExitCode: 1,
			Duration: time.Duration(0),
		}
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdoutBuf, stderrBuf *LineBuffer
	run := func(cmd *exec.Cmd) error {
		stdoutBuf = NewLineBuffer(ctx)
		defer func() {
			err := stdoutBuf.Close()
			if err != nil {
				logging.Error("Failed to close stdout buffer: %v", err)
			}
		}()

		stderrBuf = NewLineBuffer(ctx)
		defer func() {
			err := stderrBuf.Close()
			if err != nil {
				logging.Error("Failed to close stderr buffer: %v", err)
			}
		}()

		cmd.Stdout = stdoutBuf
		cmd.Stderr = stderrBuf

		logging.Trace("Executing: %v", args)
		return cmd.Run()
	}

	err := run(cmd)
	dur := time.Since(start)
	stdout := stdoutBuf.CloseAndGetLines()
	stderr := stderrBuf.CloseAndGetLines()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ctx.Err() != nil {
			exitCode = 1
			stderr = append(stderr, ctx.Err().Error())
		} else if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			stderr = append(stderr, err.Error())
		}
	}

	logging.Trace("Execution completed: %v, exit code: %d, duration: %s", args, exitCode, dur)

	return ExecResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: dur,
	}
}
