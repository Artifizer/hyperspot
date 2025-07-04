package orm

import (
	"time"

	"github.com/hypernetix/hyperspot/libs/db"
	"github.com/hypernetix/hyperspot/libs/errorx"
)

// RetryConfig holds configuration for database retry operations
type RetryConfig struct {
	MaxRetries int           // Maximum number of retry attempts
	Backoff    time.Duration // Initial backoff duration
	MaxBackoff time.Duration // Maximum backoff duration (optional, 0 means no limit)
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 5,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
	}
}

// RetryableOperation represents a function that can be retried
type RetryableOperation func() error

// RetryableOperationWithResult represents a function that returns a result and can be retried
type RetryableOperationWithResult[T any] func() (T, error)

// RetryWithError executes a retryable operation and returns an errorx.Error
// This is the main retry function for operations that only return errors
func RetryWithError(operation RetryableOperation, config RetryConfig) errorx.Error {
	var lastErr error
	backoff := config.Backoff

	// Ensure we attempt at least once, even if MaxRetries is 0 or negative
	maxAttempts := config.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if this is a database lock error that should be retried
		if !db.DatabaseIsLocked(err) {
			return errorx.NewErrInternalServerError(err.Error())
		}

		// If this is the last attempt, don't sleep
		if attempt == maxAttempts-1 {
			break
		}

		// Sleep with exponential backoff
		time.Sleep(backoff)

		// Calculate next backoff with exponential increase
		backoff *= 2

		// Apply maximum backoff limit if configured
		if config.MaxBackoff > 0 && backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	// Ensure we have a valid error to return
	if lastErr == nil {
		return errorx.NewErrInternalServerError("unknown error occurred during retry")
	}

	return errorx.NewErrInternalServerError(lastErr.Error())
}

// RetryWithErrorAndResult executes a retryable operation that returns a result and error
// This is the main retry function for operations that return both results and errors
func RetryWithErrorAndResult[T any](operation RetryableOperationWithResult[T], config RetryConfig) errorx.Error {
	var lastErr error
	backoff := config.Backoff

	// Ensure we attempt at least once, even if MaxRetries is 0 or negative
	maxAttempts := config.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		_, err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if this is a database lock error that should be retried
		if !db.DatabaseIsLocked(err) {
			return errorx.NewErrInternalServerError(err.Error())
		}

		// If this is the last attempt, don't sleep
		if attempt == maxAttempts-1 {
			break
		}

		// Sleep with exponential backoff
		time.Sleep(backoff)

		// Calculate next backoff with exponential increase
		backoff *= 2

		// Apply maximum backoff limit if configured
		if config.MaxBackoff > 0 && backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	// Ensure we have a valid error to return
	if lastErr == nil {
		return errorx.NewErrInternalServerError("unknown error occurred during retry")
	}

	return errorx.NewErrInternalServerError(lastErr.Error())
}

// RetryWithResult executes a retryable operation and returns both result and error
// This is useful when you need both the result and error from the operation
func RetryWithResult[T any](operation RetryableOperationWithResult[T], config RetryConfig) (T, errorx.Error) {
	var lastErr error
	var lastResult T
	backoff := config.Backoff

	// Ensure we attempt at least once, even if MaxRetries is 0 or negative
	maxAttempts := config.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := operation()
		if err == nil {
			return result, nil
		}

		lastErr = err
		lastResult = result

		// Check if this is a database lock error that should be retried
		if !db.DatabaseIsLocked(err) {
			return lastResult, errorx.NewErrInternalServerError(err.Error())
		}

		// If this is the last attempt, don't sleep
		if attempt == maxAttempts-1 {
			break
		}

		// Sleep with exponential backoff
		time.Sleep(backoff)

		// Calculate next backoff with exponential increase
		backoff *= 2

		// Apply maximum backoff limit if configured
		if config.MaxBackoff > 0 && backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	// Ensure we have a valid error to return
	if lastErr == nil {
		return lastResult, errorx.NewErrInternalServerError("unknown error occurred during retry")
	}

	return lastResult, errorx.NewErrInternalServerError(lastErr.Error())
}
