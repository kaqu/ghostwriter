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
	locks     sync.Map // Stores filename (string) -> *LockInfo
	semaphore chan struct{}
}

// NewLockManager initializes and returns a new LockManager.
func NewLockManager(maxConcurrentOps int) *LockManager {
	if maxConcurrentOps <= 0 {
		maxConcurrentOps = 1 // Ensure at least one operation can proceed
	}
	return &LockManager{
		semaphore: make(chan struct{}, maxConcurrentOps),
	}
}

// AcquireLock attempts to acquire a lock for the given filename.
// It enforces the configured global concurrency limit using a semaphore and
// relies on OS-level file locking for the file itself.
func (lm *LockManager) AcquireLock(filename string, timeout time.Duration) error {
	if filename == "" {
		return ErrFilenameRequired
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Acquire global semaphore slot
	select {
	case lm.semaphore <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for global capacity: %w while trying to lock %s", ErrLockTimeout, filename)
	}

	// Attempt filesystem lock
	fileLock := flock.New(filename + ".lock")
	locked, err := fileLock.TryLockContext(ctx, shortPollInterval)
	if err != nil {
		<-lm.semaphore
		return fmt.Errorf("error acquiring file lock for %s: %w", filename, err)
	}
	if !locked {
		<-lm.semaphore
		return fmt.Errorf("timeout acquiring lock for file %s: %w", filename, ErrLockTimeout)
	}

	lm.locks.Store(filename, &LockInfo{AcquiredAt: time.Now(), FLock: fileLock})
	return nil
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

	select {
	case <-lm.semaphore:
	default:
	}
	return nil
}

// GetCurrentLockCount returns the current number of active locks. Useful for testing.
func (lm *LockManager) GetCurrentLockCount() int {
	return len(lm.semaphore)
}
