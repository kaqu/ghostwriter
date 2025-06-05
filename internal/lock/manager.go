package lock

import (
	"fmt"
	"sync"
	"time"
)

var (
	// ErrLockTimeout is returned when acquiring a lock times out.
	ErrLockTimeout = fmt.Errorf("timeout acquiring lock")
	// ErrMaxConcurrentOpsReached is returned when the maximum number of concurrent operations is reached.
	ErrMaxConcurrentOpsReached = fmt.Errorf("maximum concurrent operations reached")
	// ErrLockNotFound is returned when trying to release a lock that does not exist.
	ErrLockNotFound = fmt.Errorf("lock not found")
	// ErrFilenameRequired is returned when a filename is empty.
	ErrFilenameRequired = fmt.Errorf("filename is required")
)

const (
	// shortPollInterval is the interval to sleep when polling for a lock.
	shortPollInterval = 10 * time.Millisecond
)

// LockInfo holds information about an acquired lock.
type LockInfo struct {
	AcquiredAt time.Time
	// OwnerID could be used for debugging, e.g., goroutine ID.
	// For simplicity, it's omitted for now as it's not strictly needed for locking logic.
}

// LockManager manages file locks to control concurrent access.
type LockManager struct {
	locks              sync.Map   // Stores filename (string) -> *LockInfo
	mu                 sync.Mutex // Protects currentLockCount
	currentLockCount   int
	maxConcurrentOps   int
	defaultLockTimeout time.Duration // Not used directly by AcquireLock, but for potential cleanup logic
}

// NewLockManager initializes and returns a new LockManager.
func NewLockManager(maxConcurrentOps int, defaultLockTimeout time.Duration) *LockManager {
	if maxConcurrentOps <= 0 {
		maxConcurrentOps = 1 // Ensure at least one operation can proceed
	}
	return &LockManager{
		maxConcurrentOps:   maxConcurrentOps,
		defaultLockTimeout: defaultLockTimeout,
	}
}

// AcquireLock attempts to acquire a lock for the given filename.
// It respects maxConcurrentOps globally and the specific timeout for the file lock.
func (lm *LockManager) AcquireLock(filename string, timeout time.Duration) error {
	if filename == "" {
		return ErrFilenameRequired
	}

	startTime := time.Now()
	deadline := startTime.Add(timeout)

	for {
		// Check global concurrency limit first
		lm.mu.Lock()
		if lm.currentLockCount >= lm.maxConcurrentOps {
			lm.mu.Unlock()
			// Global limit reached, wait or timeout
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for global capacity: %w while trying to lock %s", ErrLockTimeout, filename)
			}
			time.Sleep(shortPollInterval)
			continue // Re-check global limit and then specific file lock
		}
		// Tentatively increment, but be ready to decrement if file-specific lock fails
		lm.currentLockCount++
		lm.mu.Unlock()

		// Try to acquire the specific file lock
		newLockInfo := &LockInfo{AcquiredAt: time.Now()}
		_, loaded := lm.locks.LoadOrStore(filename, newLockInfo)

		if !loaded {
			// Lock acquired successfully
			return nil
		}

		// Lock was already present (loaded = true), decrement global count and prepare to wait for this specific lock
		lm.mu.Lock()
		lm.currentLockCount--
		lm.mu.Unlock()

		// File is locked by someone else, wait for it or timeout for this specific file
		for {
			if _, ok := lm.locks.Load(filename); !ok {
				// Lock was released, break inner loop and retry acquiring in the outer loop
				// This ensures the global lock count is checked again.
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout acquiring lock for file %s: %w", filename, ErrLockTimeout)
			}
			time.Sleep(shortPollInterval)
		}
		// If we broke from inner loop because lock was released, the outer loop will retry.
		// If we are here after deadline check, it's an error (already returned).
	}
}

// ReleaseLock removes the lock for the filename from the sync.Map.
func (lm *LockManager) ReleaseLock(filename string) error {
	if filename == "" {
		return ErrFilenameRequired
	}

	if _, loaded := lm.locks.LoadAndDelete(filename); !loaded {
		return fmt.Errorf("attempted to release a non-existent lock for file %s: %w", filename, ErrLockNotFound)
	}

	lm.mu.Lock()
	if lm.currentLockCount > 0 {
		lm.currentLockCount--
	}
	lm.mu.Unlock()
	return nil
}

// GetCurrentLockCount returns the current number of active locks. Useful for testing.
func (lm *LockManager) GetCurrentLockCount() int {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.currentLockCount
}

// CleanupExpiredLocks iterates through the map and removes locks older than defaultLockTimeout.
// This is intended to be run periodically by a background goroutine.
func (lm *LockManager) CleanupExpiredLocks() {
	now := time.Now()
	lm.locks.Range(func(key, value interface{}) bool {
		filename := key.(string)
		lockInfo, ok := value.(*LockInfo)
		if !ok {
			// Should not happen if only LockInfo is stored
			lm.locks.Delete(filename) // Remove unexpected item
			return true
		}

		if now.Sub(lockInfo.AcquiredAt) > lm.defaultLockTimeout {
			// Lock has expired, attempt to release it
			// Use ReleaseLock to correctly decrement global counter
			// We need to be careful here: if ReleaseLock is called, it will try to LoadAndDelete.
			// If another goroutine just released it, LoadAndDelete might fail.
			// It's safer to just Delete and decrement count if we know it's expired.

			// Check again before deleting, in case it was released and re-acquired.
			currentLockInfo, loaded := lm.locks.Load(filename)
			if loaded && currentLockInfo == lockInfo { // Ensure it's the same lock we deemed expired
				lm.locks.Delete(filename)
				lm.mu.Lock()
				// Only decrement if the lock we are cleaning up was indeed still considered active
				// This check helps prevent double-decrementing if a lock is released normally
				// right as cleanup is happening. The `currentLockInfo == lockInfo` check helps.
				// A more robust way might involve checking if the lockInfo on the map is still the one we read.
				// For simplicity here, if we delete it, we decrement.
				// This could be an issue if a lock expires, is deleted by cleanup,
				// then another thread tries to release it. Release would fail.
				// The main protection is that normal operations should release locks before they expire.
				if lm.currentLockCount > 0 {
					lm.currentLockCount--
				}
				lm.mu.Unlock()
			}
		}
		return true
	})
}
