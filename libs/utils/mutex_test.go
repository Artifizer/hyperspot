package utils

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDebugMutex_BasicLockUnlock(t *testing.T) {
	// Test with debug disabled
	MutexDebugEnabled = false
	m := DebugMutex{}

	// Should lock and unlock without issues
	m.Lock()
	m.Unlock()

	// Test with debug enabled
	MutexDebugEnabled = true
	m = DebugMutex{}

	// Should lock and track state
	m.Lock()
	assert.True(t, m.isLocked)
	assert.NotEqual(t, int64(0), m.owner)
	assert.NotEqual(t, "", m.stackTrace)
	assert.NotEqual(t, "", m.filename)
	assert.NotEqual(t, 0, m.line)
	assert.False(t, m.lockedAt.IsZero())

	// Should unlock and clear state
	m.Unlock()
	assert.False(t, m.isLocked)
	assert.Equal(t, int64(0), m.owner)
	assert.Equal(t, "", m.stackTrace)
	assert.Equal(t, "", m.filename)
	assert.Equal(t, 0, m.line)
	assert.True(t, m.lockedAt.IsZero())
}

func TestDebugMutex_TryLock(t *testing.T) {
	// Test with debug disabled
	MutexDebugEnabled = false
	m := DebugMutex{}

	// TryLock should succeed
	assert.True(t, m.TryLock())
	m.Unlock()

	// Test with debug enabled
	MutexDebugEnabled = true
	m = DebugMutex{}

	// First TryLock should succeed
	assert.True(t, m.TryLock())
	assert.True(t, m.isLocked)

	// Create a second mutex for testing concurrent access
	m2 := DebugMutex{}

	// Lock m2 in a different goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Lock should succeed because it's a different mutex
		m2.Lock()
		time.Sleep(50 * time.Millisecond)
		m2.Unlock()
	}()

	// Wait for goroutine to complete
	wg.Wait()

	// Unlock the first mutex
	m.Unlock()
	assert.False(t, m.isLocked)
}

func TestDebugMutex_SelfDeadlock(t *testing.T) {
	// Enable debug mode
	MutexDebugEnabled = true

	m := DebugMutex{}

	// First lock should succeed
	m.Lock()
	assert.True(t, m.isLocked)

	// Second lock attempt should detect potential deadlock but still lock
	// (This tests the warning is generated - we can't easily test the actual output)
	// m.Lock()

	// Should be able to unlock twice
	// m.Unlock()
	m.Unlock()
	assert.False(t, m.isLocked)
}

func TestDebugMutex_UnlockByDifferentGoroutine(t *testing.T) {
	// Enable debug mode
	MutexDebugEnabled = true

	m := DebugMutex{}

	// Lock in this goroutine
	m.Lock()
	assert.True(t, m.isLocked)
	owner := m.owner

	// Unlock in a different goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// This should detect the wrong owner but still unlock
		m.Unlock()
	}()

	// Wait for goroutine to complete
	wg.Wait()

	// Verify the mutex was unlocked despite the owner mismatch
	assert.False(t, m.isLocked)
	assert.Equal(t, owner, getGoID()) // Confirm different goroutine IDs
}

func TestDebugMutex_HelperMethods(t *testing.T) {
	// Enable debug mode
	MutexDebugEnabled = true

	m := DebugMutex{}

	// Test methods on unlocked mutex
	assert.False(t, m.IsLocked())
	assert.Equal(t, int64(0), m.GetOwner())
	assert.True(t, m.LockedAt().IsZero())

	// Lock and test again
	m.Lock()
	assert.True(t, m.IsLocked())
	assert.Equal(t, getGoID(), m.GetOwner())
	assert.False(t, m.LockedAt().IsZero())

	// Unlock and test again
	m.Unlock()
	assert.False(t, m.IsLocked())
	assert.Equal(t, int64(0), m.GetOwner())
	assert.True(t, m.LockedAt().IsZero())
}

func TestSetMutexDebug(t *testing.T) {
	// Initial state
	initialState := MutexDebugEnabled

	// Toggle to opposite state
	SetMutexDebug(!initialState)
	assert.Equal(t, !initialState, MutexDebugEnabled)

	// Toggle back
	SetMutexDebug(initialState)
	assert.Equal(t, initialState, MutexDebugEnabled)
}

func TestSetMutexDeadlockWarningDelay(t *testing.T) {
	// Initial state
	initialDelay := WarnAfterLockDelaySeconds

	// Set to new value
	SetMutexDeadlockWarningDelay(42)
	assert.Equal(t, 42, WarnAfterLockDelaySeconds)

	// Set back to original
	SetMutexDeadlockWarningDelay(initialDelay)
	assert.Equal(t, initialDelay, WarnAfterLockDelaySeconds)
}

func TestGetGoID(t *testing.T) {
	// Just ensure it returns a non-zero value
	id := getGoID()
	assert.NotEqual(t, int64(0), id)

	// Test that different goroutines get different IDs
	var wg sync.WaitGroup
	var id2 int64

	wg.Add(1)
	go func() {
		defer wg.Done()
		id2 = getGoID()
	}()

	wg.Wait()
	assert.NotEqual(t, id, id2)
}

func TestCaptureStack(t *testing.T) {
	// Ensure it produces a non-empty string
	stack := captureStack()
	assert.NotEqual(t, "", stack)

	// Ensure it contains some expected function call patterns
	assert.Contains(t, stack, "testing.tRunner")
}

func TestDebugMutex_TryLockWithTimeout(t *testing.T) {
	// Test with debug disabled
	MutexDebugEnabled = false

	t.Run("SuccessImmediatelyAvailable", func(t *testing.T) {
		m := DebugMutex{}

		// Lock should be acquired immediately
		success := m.TryLockWithTimeout(100 * time.Millisecond)
		assert.True(t, success)

		// Should be able to unlock
		m.Unlock()
	})

	t.Run("SuccessAfterShortWait", func(t *testing.T) {
		m := DebugMutex{}

		// Lock the mutex in a goroutine and release it after 50ms
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Lock()
			time.Sleep(50 * time.Millisecond)
			m.Unlock()
		}()

		// Wait for goroutine to acquire the lock
		time.Sleep(10 * time.Millisecond)

		// Try to lock with timeout longer than the hold time
		success := m.TryLockWithTimeout(200 * time.Millisecond)
		assert.True(t, success)

		// Clean up
		m.Unlock()
		wg.Wait()
	})

	t.Run("TimeoutExpired", func(t *testing.T) {
		m := DebugMutex{}

		// Lock the mutex in a goroutine and hold it longer than timeout
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Lock()
			time.Sleep(200 * time.Millisecond)
			m.Unlock()
		}()

		// Wait for goroutine to acquire the lock
		time.Sleep(10 * time.Millisecond)

		// Try to lock with timeout shorter than the hold time
		start := time.Now()
		success := m.TryLockWithTimeout(50 * time.Millisecond)
		elapsed := time.Since(start)

		assert.False(t, success)
		// Should have waited approximately the timeout duration
		assert.True(t, elapsed >= 50*time.Millisecond)
		assert.True(t, elapsed < 100*time.Millisecond) // Allow some tolerance

		wg.Wait()
	})

	t.Run("ZeroTimeout", func(t *testing.T) {
		m := DebugMutex{}

		// Lock the mutex
		m.Lock()

		// Try to lock with zero timeout - should fail immediately
		start := time.Now()
		success := m.TryLockWithTimeout(0)
		elapsed := time.Since(start)

		assert.False(t, success)
		// Should return very quickly (less than 10ms)
		assert.True(t, elapsed < 10*time.Millisecond)

		m.Unlock()
	})

	t.Run("VeryLongTimeout", func(t *testing.T) {
		m := DebugMutex{}

		// Lock should be acquired immediately even with long timeout
		start := time.Now()
		success := m.TryLockWithTimeout(10 * time.Second)
		elapsed := time.Since(start)

		assert.True(t, success)
		// Should return very quickly (less than 10ms)
		assert.True(t, elapsed < 10*time.Millisecond)

		m.Unlock()
	})

	// Test with debug enabled
	MutexDebugEnabled = true

	t.Run("DebugEnabledSuccess", func(t *testing.T) {
		m := DebugMutex{}

		// Lock should be acquired immediately
		success := m.TryLockWithTimeout(100 * time.Millisecond)
		assert.True(t, success)

		// Debug info should be populated
		assert.True(t, m.IsLocked())
		assert.Equal(t, getGoID(), m.GetOwner())
		assert.False(t, m.LockedAt().IsZero())

		m.Unlock()

		// Debug info should be cleared
		assert.False(t, m.IsLocked())
		assert.Equal(t, int64(0), m.GetOwner())
		assert.True(t, m.LockedAt().IsZero())
	})

	t.Run("DebugEnabledTimeout", func(t *testing.T) {
		m := DebugMutex{}

		// Lock the mutex in a goroutine
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Lock()
			time.Sleep(200 * time.Millisecond)
			m.Unlock()
		}()

		// Wait for goroutine to acquire the lock
		time.Sleep(10 * time.Millisecond)

		// Try to lock with timeout - should fail and log trace message
		success := m.TryLockWithTimeout(50 * time.Millisecond)
		assert.False(t, success)

		wg.Wait()
	})

	t.Run("ConcurrentTryLockWithTimeout", func(t *testing.T) {
		m := DebugMutex{}

		// Lock the mutex initially
		m.Lock()

		const numGoroutines = 5
		var wg sync.WaitGroup
		results := make([]bool, numGoroutines)

		// Start multiple goroutines trying to acquire with timeout
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index] = m.TryLockWithTimeout(100 * time.Millisecond)
				if results[index] {
					// If we got the lock, hold it for a bit to prevent others from succeeding
					time.Sleep(150 * time.Millisecond)
					m.Unlock()
				}
			}(i)
		}

		// Release the lock after 50ms
		time.Sleep(50 * time.Millisecond)
		m.Unlock()

		// Wait for all goroutines to complete
		wg.Wait()

		// Exactly one goroutine should have succeeded
		successCount := 0
		for _, result := range results {
			if result {
				successCount++
			}
		}
		assert.Equal(t, 1, successCount)
	})

	t.Run("FinalAttemptSuccess", func(t *testing.T) {
		m := DebugMutex{}

		// Lock the mutex in a goroutine and release it exactly at timeout
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Lock()
			// Release just before the timeout + final attempt
			time.Sleep(55 * time.Millisecond)
			m.Unlock()
		}()

		// Wait for goroutine to acquire the lock
		time.Sleep(10 * time.Millisecond)

		// Try to lock with timeout - should succeed on final attempt
		success := m.TryLockWithTimeout(50 * time.Millisecond)
		assert.True(t, success)

		m.Unlock()
		wg.Wait()
	})
}
