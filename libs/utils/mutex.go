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
	// globalMutex protects access to global debug settings
	globalMutex sync.RWMutex

	// MutexDebugEnabled enables mutex debugging features globally
	MutexDebugEnabled = false

	// WarnAfterLockDelaySeconds is the time to wait before warning about potential deadlocks
	WarnAfterLockDelaySeconds = 30
)

// SetMutexDebug enables or disables mutex debugging globally
func SetMutexDebug(enabled bool) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	MutexDebugEnabled = enabled
}

// SetMutexDeadlockWarningDelay sets the delay before warning about potential deadlocks
func SetMutexDeadlockWarningDelay(seconds int) {
	globalMutex.Lock()
	defer globalMutex.Unlock()
	WarnAfterLockDelaySeconds = seconds
}

// DebugMutex is a drop-in replacement for sync.Mutex with debugging capabilities
type DebugMutex struct {
	sync.Mutex
	// stateMutex protects the debug state fields to prevent data races
	stateMutex  sync.Mutex
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
	globalMutex.RLock()
	isDebugEnabled := MutexDebugEnabled
	globalMutex.RUnlock()

	if !isDebugEnabled {
		m.Mutex.Lock()
		return
	}

	// Get the current goroutine ID
	currentGoID := getGoID()

	_, file, line, _ := runtime.Caller(1)

	// Check for self-deadlock (same goroutine already holds the lock)
	m.stateMutex.Lock()
	isLocked := m.isLocked
	owner := m.owner
	filename := m.filename
	curLine := m.line
	curStackTrace := m.stackTrace
	warnStarted := m.warnStarted
	m.stateMutex.Unlock()

	if isLocked && owner == currentGoID {
		logging.Error("Potential deadlock detected - goroutine %d attempting to lock mutex at:\n\n%s:%d\n%s\nthat it already holds at:\n\n%s:%d\n%s",
			currentGoID, file, line, captureStack(), filename, curLine, curStackTrace)
		// Continue anyway to match sync.Mutex behavior
	}

	// Start a goroutine to warn if lock takes too long to acquire
	globalMutex.RLock()
	warnDebugEnabled := MutexDebugEnabled
	globalMutex.RUnlock()

	if !warnStarted && warnDebugEnabled {
		m.stateMutex.Lock()
		m.warnStarted = true
		m.stateMutex.Unlock()

		go func() {
			globalMutex.RLock()
			warningDelay := WarnAfterLockDelaySeconds
			globalMutex.RUnlock()
			timer := time.NewTimer(time.Duration(warningDelay) * time.Second)
			for {
				select {
				case <-timer.C:
					m.stateMutex.Lock()
					isLocked := m.isLocked
					owner := m.owner
					filename := m.filename
					curLine := m.line
					curStackTrace := m.stackTrace
					lockedAt := m.lockedAt
					m.stateMutex.Unlock()

					if isLocked {
						globalMutex.RLock()
						warningDelay := WarnAfterLockDelaySeconds
						globalMutex.RUnlock()
						logging.Warn("Potential deadlock - waiting for mutex for %d seconds:\n%s:%d\n%s\n\n",
							warningDelay, filename, curLine, captureStack())
						logging.Warn("Mutex is held by goroutine %d since %s, lock acquired at:\n%s:%d:\n%s",
							owner, lockedAt.Format(time.RFC3339), filename, curLine, curStackTrace)
						// Reset timer for next warning
						globalMutex.RLock()
						resetDelay := WarnAfterLockDelaySeconds
						globalMutex.RUnlock()
						timer.Reset(time.Duration(resetDelay) * time.Second)
					} else {
						// Lock was released, stop warning
						m.stateMutex.Lock()
						m.warnStarted = false
						m.stateMutex.Unlock()
						return
					}
				}
			}
		}()
	}

	// Try to acquire the lock
	m.Mutex.Lock()

	// Lock acquired, record metadata
	m.stateMutex.Lock()
	m.isLocked = true
	m.owner = currentGoID
	m.filename = file
	m.line = line
	m.lockedAt = time.Now()
	m.stackTrace = captureStack()
	m.stateMutex.Unlock()
}

// Unlock releases the mutex with debug capabilities
func (m *DebugMutex) Unlock() {
	globalMutex.RLock()
	isDebugEnabled := MutexDebugEnabled
	globalMutex.RUnlock()

	if !isDebugEnabled {
		m.Mutex.Unlock()
		return
	}

	// Get the current goroutine ID
	currentGoID := getGoID()

	// Check if the mutex is being unlocked by a different goroutine
	m.stateMutex.Lock()
	isLocked := m.isLocked
	owner := m.owner
	filename := m.filename
	curLine := m.line
	curStackTrace := m.stackTrace
	m.stateMutex.Unlock()

	if isLocked && owner != currentGoID {
		_, callerFile, line, _ := runtime.Caller(1)
		logging.Error("Mutex unlocked by goroutine %d at %s:%d\n%s\nbut was locked by goroutine #%d @ %s:%d\n%s",
			currentGoID, callerFile, line, captureStack(), owner, filename, curLine, curStackTrace)
		// Continue anyway to match sync.Mutex behavior
	}

	// Clear metadata and unlock
	m.stateMutex.Lock()
	m.isLocked = false
	m.owner = 0
	m.stackTrace = ""
	m.filename = ""
	m.line = 0
	m.lockedAt = time.Time{}
	m.stateMutex.Unlock()

	m.Mutex.Unlock()
}

func (m *DebugMutex) TryLock() bool {
	globalMutex.RLock()
	isDebugEnabled := MutexDebugEnabled
	globalMutex.RUnlock()

	if !isDebugEnabled {
		return m.Mutex.TryLock()
	}

	// Get the current goroutine ID
	currentGoID := getGoID()

	_, file, line, _ := runtime.Caller(1)

	// Try to acquire the lock without blocking
	if !m.Mutex.TryLock() {
		return false
	}

	// Lock was acquired, record metadata
	m.stateMutex.Lock()
	m.isLocked = true
	m.owner = currentGoID
	m.filename = file
	m.line = line
	m.lockedAt = time.Now()
	m.stackTrace = captureStack()
	m.stateMutex.Unlock()
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
		m.stateMutex.Lock()
		owner := m.owner
		lockedAt := m.lockedAt
		filename := m.filename
		curLine := m.line
		curStackTrace := m.stackTrace
		m.stateMutex.Unlock()

		msg = fmt.Sprintf("mutex.TryLockWithTimeout(%s) failed after %s seconds, locked by goroutine #%d since %s\n%s:%d\n%s",
			timeout, time.Since(start), owner, lockedAt.Format(time.RFC3339), filename, curLine, curStackTrace)
	}

	// final attempt
	if m.TryLock() {
		return true
	}

	globalMutex.RLock()
	debugEnabled := MutexDebugEnabled
	globalMutex.RUnlock()

	if debugEnabled {
		logging.Trace(msg)
	}
	return false
}

// GetOwner returns the ID of the goroutine that currently holds the lock
func (m *DebugMutex) GetOwner() int64 {
	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()
	if !m.isLocked {
		return 0
	}
	return m.owner
}

// IsLocked returns whether the mutex is currently locked
func (m *DebugMutex) IsLocked() bool {
	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()
	return m.isLocked
}

// LockedAt returns when the mutex was locked
func (m *DebugMutex) LockedAt() time.Time {
	m.stateMutex.Lock()
	defer m.stateMutex.Unlock()
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
