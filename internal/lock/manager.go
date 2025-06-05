package lock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/flock"
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
	FLock      *flock.Flock
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

		// Try to acquire the specific file lock using filesystem locking
		fileLock := flock.New(filename + ".lock")
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		locked, err := fileLock.TryLockContext(ctx, shortPollInterval)
		cancel()
		if err != nil {
			lm.mu.Lock()
			lm.currentLockCount--
			lm.mu.Unlock()
			return fmt.Errorf("error acquiring file lock for %s: %w", filename, err)
		}
		if locked {
			newLockInfo := &LockInfo{AcquiredAt: time.Now(), FLock: fileLock}
			lm.locks.Store(filename, newLockInfo)
			return nil
		}

		// Failed to acquire within timeout
		lm.mu.Lock()
		lm.currentLockCount--
		lm.mu.Unlock()
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout acquiring lock for file %s: %w", filename, ErrLockTimeout)
		}
		// wait a bit before retrying
		time.Sleep(shortPollInterval)
	}
}

// ReleaseLock removes the lock for the filename from the sync.Map.
func (lm *LockManager) ReleaseLock(filename string) error {
	if filename == "" {
		return ErrFilenameRequired
	}

	v, loaded := lm.locks.LoadAndDelete(filename)
	if !loaded {
		return fmt.Errorf("attempted to release a non-existent lock for file %s: %w", filename, ErrLockNotFound)
	}
	info := v.(*LockInfo)
	if info.FLock != nil {
		_ = info.FLock.Unlock()
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
			currentLockInfo, loaded := lm.locks.Load(filename)
			if loaded && currentLockInfo == lockInfo {
				_ = lockInfo.FLock.Unlock()
				lm.locks.Delete(filename)
				lm.mu.Lock()
				if lm.currentLockCount > 0 {
					lm.currentLockCount--
				}
				lm.mu.Unlock()
			}
		}
		return true
	})
}
