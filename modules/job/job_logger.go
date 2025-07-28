package job

import (
	"fmt"

	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"go.uber.org/zap"
)

var logger *logging.Logger = logging.MainLogger.WithField(logging.ServiceField, utils.GoCallerPackageName(1)) // Initially set to main logger, can be overridden by config

func (j *JobObj) Log(level logging.Level, msg string, args ...interface{}) {
	// Format the message with arguments first
	formattedMsg := fmt.Sprintf(msg, args...)
	fields := []zap.Field{
		zap.String("job_type", j.priv.Type),
		zap.String("job_id", j.priv.JobID.String()),
	}

	textMsg := fmt.Sprintf("Job %s/%s@%p: %s", j.priv.Type, j.priv.JobID.String(), j, formattedMsg)

	logger.ConsoleLogger.Log(level, textMsg)
	logger.FileLogger.With(fields...).Log(level, formattedMsg)
}

// Job logs
func (j *JobObj) LogTrace(msg string, args ...interface{}) {
	j.Log(logging.TraceLevel, msg, args...)
}

func (j *JobObj) LogDebug(msg string, args ...interface{}) {
	j.Log(logging.DebugLevel, msg, args...)
}

// Job logs
func (j *JobObj) LogInfo(msg string, args ...interface{}) {
	j.Log(logging.InfoLevel, msg, args...)
}

// Job logs
func (j *JobObj) LogWarn(msg string, args ...interface{}) {
	j.Log(logging.WarnLevel, msg, args...)
}

// Job logs
func (j *JobObj) LogError(msg string, args ...interface{}) {
	j.Log(logging.ErrorLevel, msg, args...)
}

// Job progress logs
func (j *JobObj) logProgress(progress float32) {
	fields := []zap.Field{
		zap.String("job_type", j.priv.Type),
		zap.String("job_id", j.priv.JobID.String()),
		zap.Float32("progress", progress),
	}
	logger.ConsoleLogger.Debug(fmt.Sprintf("Job %s/%s: progress = %.1f %%", j.priv.Type, j.priv.JobID.String(), progress))
	logger.FileLogger.With(fields...).Debug("Job progress")
}
