package orm

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Mock database lock error
type mockDBLockError struct{}

func (e mockDBLockError) Error() string {
	return "database table is locked"
}

// Test helper to create a mock database lock error
func mockDatabaseLockError() error {
	return &mockDBLockError{}
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 10*time.Millisecond, config.Backoff)
	assert.Equal(t, 100*time.Millisecond, config.MaxBackoff)
}

func TestRetryWithError_SuccessOnFirstAttempt(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return nil
	}

	err := RetryWithError(operation, DefaultRetryConfig())

	assert.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestRetryWithError_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		if attempts < 3 {
			return mockDatabaseLockError()
		}
		return nil
	}

	start := time.Now()
	err := RetryWithError(operation, DefaultRetryConfig())
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)

	// Should have slept for at least 10ms + 20ms = 30ms
	assert.GreaterOrEqual(t, duration, 30*time.Millisecond)
}

func TestRetryWithError_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: 3,
		Backoff:    1 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	}

	err := RetryWithError(operation, config)

	assert.Error(t, err)
	assert.Equal(t, 3, attempts)
	assert.Contains(t, err.Error(), "database table is locked")
}

func TestRetryWithError_NonLockErrorImmediateReturn(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return errors.New("some other error")
	}

	err := RetryWithError(operation, DefaultRetryConfig())

	assert.Error(t, err)
	assert.Equal(t, 1, attempts)
	assert.Contains(t, err.Error(), "some other error")
}

func TestRetryWithErrorAndResult_SuccessOnFirstAttempt(t *testing.T) {
	attempts := 0
	operation := func() (string, error) {
		attempts++
		return "success", nil
	}

	err := RetryWithErrorAndResult(operation, DefaultRetryConfig())

	assert.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestRetryWithErrorAndResult_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	operation := func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", mockDatabaseLockError()
		}
		return "success", nil
	}

	start := time.Now()
	err := RetryWithErrorAndResult(operation, DefaultRetryConfig())
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)

	// Should have slept for at least 10ms + 20ms = 30ms
	assert.GreaterOrEqual(t, duration, 30*time.Millisecond)
}

func TestRetryWithErrorAndResult_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	operation := func() (string, error) {
		attempts++
		return "", mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: 2,
		Backoff:    1 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	}

	err := RetryWithErrorAndResult(operation, config)

	assert.Error(t, err)
	assert.Equal(t, 2, attempts)
	assert.Contains(t, err.Error(), "database table is locked")
}

func TestRetryWithResult_SuccessOnFirstAttempt(t *testing.T) {
	attempts := 0
	operation := func() (int, error) {
		attempts++
		return 42, nil
	}

	result, err := RetryWithResult(operation, DefaultRetryConfig())

	assert.NoError(t, err)
	assert.Equal(t, 42, result)
	assert.Equal(t, 1, attempts)
}

func TestRetryWithResult_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	operation := func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, mockDatabaseLockError()
		}
		return 42, nil
	}

	start := time.Now()
	result, err := RetryWithResult(operation, DefaultRetryConfig())
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 42, result)
	assert.Equal(t, 3, attempts)

	// Should have slept for at least 10ms + 20ms = 30ms
	assert.GreaterOrEqual(t, duration, 30*time.Millisecond)
}

func TestRetryWithResult_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	operation := func() (int, error) {
		attempts++
		return attempts, mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: 2,
		Backoff:    1 * time.Millisecond,
		MaxBackoff: 10 * time.Millisecond,
	}

	result, err := RetryWithResult(operation, config)

	assert.Error(t, err)
	assert.Equal(t, 2, result) // Should return the last result
	assert.Equal(t, 2, attempts)
	assert.Contains(t, err.Error(), "database table is locked")
}

func TestRetryWithResult_NonLockErrorImmediateReturn(t *testing.T) {
	attempts := 0
	operation := func() (int, error) {
		attempts++
		return 0, errors.New("some other error")
	}

	result, err := RetryWithResult(operation, DefaultRetryConfig())

	assert.Error(t, err)
	assert.Equal(t, 0, result)
	assert.Equal(t, 1, attempts)
	assert.Contains(t, err.Error(), "some other error")
}

func TestRetryWithResult_ComplexType(t *testing.T) {
	attempts := 0
	operation := func() (map[string]interface{}, error) {
		attempts++
		if attempts < 2 {
			return nil, mockDatabaseLockError()
		}
		return map[string]interface{}{
			"key": "value",
			"num": 42,
		}, nil
	}

	result, err := RetryWithResult(operation, DefaultRetryConfig())

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, "value", result["key"])
	assert.Equal(t, 42, result["num"])
}

func TestRetryConfig_MaxBackoffLimit(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: 5,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 15 * time.Millisecond, // Limit backoff to 15ms
	}

	start := time.Now()
	err := RetryWithError(operation, config)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, 5, attempts)

	// Should not exceed max backoff: 10ms + 15ms + 15ms + 15ms + 15ms = 70ms max
	// But we need to account for some overhead
	assert.LessOrEqual(t, duration, 200*time.Millisecond)
}

func TestRetryConfig_ZeroMaxBackoff(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: 3,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 0, // No limit
	}

	start := time.Now()
	err := RetryWithError(operation, config)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, 3, attempts)

	// Should have exponential backoff: 10ms + 20ms = 30ms minimum
	assert.GreaterOrEqual(t, duration, 30*time.Millisecond)
}

func TestRetryWithError_ZeroRetries(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: 0,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
	}

	err := RetryWithError(operation, config)

	assert.Error(t, err)
	assert.Equal(t, 1, attempts) // Should still attempt once
	assert.Contains(t, err.Error(), "database table is locked")
}

func TestRetryWithError_NegativeRetries(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return mockDatabaseLockError()
	}

	config := RetryConfig{
		MaxRetries: -1,
		Backoff:    10 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
	}

	err := RetryWithError(operation, config)

	assert.Error(t, err)
	assert.Equal(t, 1, attempts) // Should still attempt once
	assert.Contains(t, err.Error(), "database table is locked")
}

// Test that the retry functions work with the actual db.DatabaseIsLocked function
func TestRetryWithError_RealDatabaseLockDetection(t *testing.T) {
	// This test verifies that our retry logic works with the actual database lock detection
	// We'll simulate different error types to ensure proper detection

	tests := []struct {
		name             string
		errorFunc        func() error
		shouldRetry      bool
		expectedAttempts int
	}{
		{
			name: "database lock error should retry",
			errorFunc: func() error {
				return errors.New("database table is locked")
			},
			shouldRetry:      true,
			expectedAttempts: 3, // Will retry until max attempts
		},
		{
			name: "other error should not retry",
			errorFunc: func() error {
				return errors.New("connection timeout")
			},
			shouldRetry:      false,
			expectedAttempts: 1, // Should not retry
		},
		{
			name: "nil error should succeed",
			errorFunc: func() error {
				return nil
			},
			shouldRetry:      false,
			expectedAttempts: 1, // Should succeed immediately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			operation := func() error {
				attempts++
				if attempts == 1 {
					return tt.errorFunc()
				}
				return nil // Succeed on retry
			}

			config := RetryConfig{
				MaxRetries: 3,
				Backoff:    1 * time.Millisecond,
				MaxBackoff: 10 * time.Millisecond,
			}

			err := RetryWithError(operation, config)

			if tt.shouldRetry {
				assert.NoError(t, err, "Should succeed after retries")
				assert.Equal(t, 2, attempts, "Should have retried once then succeeded")
			} else {
				if tt.errorFunc() == nil {
					assert.NoError(t, err, "Should succeed immediately")
					assert.Equal(t, 1, attempts, "Should not have retried")
				} else {
					assert.Error(t, err, "Should return error without retrying")
					assert.Equal(t, 1, attempts, "Should not have retried")
				}
			}
		})
	}
}

// Benchmark tests to ensure performance is acceptable
func BenchmarkRetryWithError_Success(b *testing.B) {
	operation := func() error {
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RetryWithError(operation, DefaultRetryConfig())
	}
}

func BenchmarkRetryWithError_NoRetries(b *testing.B) {
	operation := func() error {
		return errors.New("non-retryable error")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RetryWithError(operation, DefaultRetryConfig())
	}
}

func BenchmarkRetryWithResult_Success(b *testing.B) {
	operation := func() (int, error) {
		return 42, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RetryWithResult(operation, DefaultRetryConfig())
	}
}
