package utils

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hypernetix/hyperspot/libs/logging"
)

var (
	// MutexDebugEnabled enables mutex debugging features globally
	MutexDebugEnabled = false

	// WarnAfterLockDelaySeconds is the time to wait before warning about potential deadlocks
	WarnAfterLockDelaySeconds = 30
)

// SetMutexDebug enables or disables mutex debugging globally
func SetMutexDebug(enabled bool) {
	MutexDebugEnabled = enabled
}

// SetMutexDeadlockWarningDelay sets the delay before warning about potential deadlocks
func SetMutexDeadlockWarningDelay(seconds int) {
	WarnAfterLockDelaySeconds = seconds
}

// DebugMutex is a drop-in replacement for sync.Mutex with debugging capabilities
type DebugMutex struct {
	sync.Mutex
	filename    string
	line        int
	owner       int64
	stackTrace  string
	lockedAt    time.Time
	isLocked    bool
	warnStarted bool
}

// Lock acquires the mutex with debug capabilities
func (m *DebugMutex) Lock() {
	if !MutexDebugEnabled {
		m.Mutex.Lock()
		return
	}

	// Get the current goroutine ID
	currentGoID := getGoID()

	_, file, line, _ := runtime.Caller(1)

	// Check for self-deadlock (same goroutine already holds the lock)
	if m.isLocked && m.owner == currentGoID {
		logging.Error("Potential deadlock detected - goroutine %d attempting to lock mutex at:\n\n%s:%d\n%s\nthat it already holds at:\n\n%s:%d\n%s",
			currentGoID, file, line, captureStack(), m.filename, m.line, m.stackTrace)
		// Continue anyway to match sync.Mutex behavior
	}

	// Start a goroutine to warn if lock takes too long to acquire
	if !m.warnStarted && MutexDebugEnabled {
		m.warnStarted = true
		go func() {
			timer := time.NewTimer(time.Duration(WarnAfterLockDelaySeconds) * time.Second)
			for {
				select {
				case <-timer.C:
					if m.isLocked {
						logging.Warn("Potential deadlock - waiting for mutex for %d seconds:\n%s:%d\n%s\n\n",
							WarnAfterLockDelaySeconds, m.filename, m.line, captureStack())
						logging.Warn("Mutex is held by goroutine %d since %s, lock acquired at:\n%s:%d:\n%s",
							m.owner, m.lockedAt.Format(time.RFC3339), m.filename, m.line, m.stackTrace)
						// Reset timer for next warning
						timer.Reset(time.Duration(WarnAfterLockDelaySeconds) * time.Second)
					} else {
						// Lock was released, stop warning
						m.warnStarted = false
						return
					}
				}
			}
		}()
	}

	// Try to acquire the lock
	m.Mutex.Lock()

	// Lock acquired, record metadata
	m.isLocked = true
	m.owner = currentGoID
	m.filename = file
	m.line = line
	m.lockedAt = time.Now()
	m.stackTrace = captureStack()
}

// Unlock releases the mutex with debug capabilities
func (m *DebugMutex) Unlock() {
	if !MutexDebugEnabled {
		m.Mutex.Unlock()
		return
	}

	// Get the current goroutine ID
	currentGoID := getGoID()

	// Check if the mutex is being unlocked by a different goroutine
	if m.isLocked && m.owner != currentGoID {
		_, filename, line, _ := runtime.Caller(1)
		logging.Error("Mutex unlocked by goroutine %d at %s:%d\n%s\nbut was locked by goroutine #%d @ %s:%d\n%s",
			currentGoID, filename, line, captureStack(), m.owner, m.filename, m.line, m.stackTrace)
		// Continue anyway to match sync.Mutex behavior
	}

	// Clear metadata and unlock
	m.isLocked = false
	m.owner = 0
	m.stackTrace = ""
	m.filename = ""
	m.line = 0
	m.lockedAt = time.Time{}
	m.Mutex.Unlock()
}

func (m *DebugMutex) TryLock() bool {
	if !MutexDebugEnabled {
		return m.Mutex.TryLock()
	}

	// Get the current goroutine ID
	currentGoID := getGoID()

	_, file, line, _ := runtime.Caller(1)

	// Check for self-deadlock (same goroutine already holds the lock)
	// if m.isLocked && m.owner == currentGoID {
	//	logging.Error("Potential deadlock detected - goroutine %d attempting to lock mutex at:\n%s:%d\n%s\nthat it already holds at:\n%s:%d\n%s",
	//		currentGoID, file, line, captureStack(), m.filename, m.line, m.stackTrace)
	//	// Continue anyway to match sync.Mutex behavior
	// }

	// Try to acquire the lock without blocking
	if !m.Mutex.TryLock() {
		return false
	}

	// Lock was acquired, record metadata
	m.isLocked = true
	m.owner = currentGoID
	m.filename = file
	m.line = line
	m.lockedAt = time.Now()
	m.stackTrace = captureStack()
	return true
}

func (m *DebugMutex) TryLockWithTimeout(timeout time.Duration) bool {
	start := time.Now()

	for time.Since(start) < timeout {
		if m.TryLock() {
			return true
		}
		// Small sleep to avoid busy waiting
		time.Sleep(5 * time.Millisecond)
	}

	var msg string
	if MutexDebugEnabled {
		msg = fmt.Sprintf("mutex.TryLockWithTimeout(%s) failed after %s seconds, locked by goroutine #%d since %s\n%s:%d\n%s",
			timeout, time.Since(start), m.owner, m.lockedAt.Format(time.RFC3339), m.filename, m.line, m.stackTrace)
	}

	// final attempt
	if m.TryLock() {
		return true
	}

	if MutexDebugEnabled {
		logging.Trace(msg)
	}
	return false
}

// GetOwner returns the ID of the goroutine that currently holds the lock
func (m *DebugMutex) GetOwner() int64 {
	if !m.isLocked {
		return 0
	}
	return m.owner
}

// IsLocked returns whether the mutex is currently locked
func (m *DebugMutex) IsLocked() bool {
	return m.isLocked
}

// LockedAt returns when the mutex was locked
func (m *DebugMutex) LockedAt() time.Time {
	return m.lockedAt
}

// getGoID extracts the goroutine ID from runtime stack
func getGoID() int64 {
	buf := make([]byte, 64)
	n := runtime.Stack(buf, false)
	idStr := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	idStr = strings.TrimSpace(strings.Split(idStr, " ")[0])
	id := int64(0)
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		logging.Warn("Failed to parse goroutine ID from '%s': %v", idStr, err)
		id = 0
	}
	return id
}

// captureStack captures the current stack trace for debugging
func captureStack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	stack := string(buf[:n])

	// Skip the first few lines which include this function and the Lock method
	lines := strings.Split(stack, "\n")
	if len(lines) > 4 {
		return strings.Join(lines[4:], "\n")
	}
	return stack
}
