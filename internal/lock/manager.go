package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

var (
	// ErrLockTimeout is returned when acquiring a lock times out.
	ErrLockTimeout = fmt.Errorf("timeout acquiring lock")
	// ErrFilenameRequired is returned when a filename is empty.
	ErrFilenameRequired = fmt.Errorf("filename is required")
	// ErrNilLock is returned when a nil lock handle is provided to ReleaseLock.
	ErrNilLock = fmt.Errorf("nil lock handle")
)

const (
	// shortPollInterval is the interval to sleep when polling for a lock.
	shortPollInterval = 10 * time.Millisecond
)

type LockManager struct{}

// NewLockManager initializes and returns a new LockManager.
func NewLockManager() *LockManager {
	return &LockManager{}
}

// AcquireLock attempts to acquire an exclusive OS-level lock for the given file.
func (lm *LockManager) AcquireLock(filename string, timeout time.Duration) (*FileLock, error) {
	if filename == "" {
		return nil, ErrFilenameRequired
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fileLock := flock.New(filename + ".lock")
	locked, err := fileLock.TryLockContext(ctx, shortPollInterval)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrLockTimeout
		}
		return nil, fmt.Errorf("error acquiring file lock for %s: %w", filename, err)
	}
	if !locked {
		return nil, ErrLockTimeout
	}

	return &FileLock{FilePath: filename, flock: fileLock}, nil
}

// ReleaseLock releases the given OS-level lock.
func (lm *LockManager) ReleaseLock(lock *FileLock) error {
	if lock == nil {
		return ErrNilLock
	}
	if lock.flock != nil {
		_ = lock.flock.Unlock()
	}
	return nil
}
