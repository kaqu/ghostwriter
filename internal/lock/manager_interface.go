package lock

import (
	"time"

	"github.com/gofrs/flock"
)

// LockManagerInterface defines the methods a lock manager should implement.
// This is used by services that depend on lock management, allowing for easier mocking.
// FileLock represents a handle to an OS-level file lock.
type FileLock struct {
	FilePath string
	flock    *flock.Flock
}

// LockManagerInterface defines the methods a lock manager should implement.
// AcquireLock obtains an exclusive OS-level file lock and returns a handle
// which must be provided back to ReleaseLock.
type LockManagerInterface interface {
	AcquireLock(filePath string, timeout time.Duration) (*FileLock, error)
	ReleaseLock(lock *FileLock) error
}
