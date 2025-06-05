package lock

import "time"

// LockManagerInterface defines the methods a lock manager should implement.
// This is used by services that depend on lock management, allowing for easier mocking.
type LockManagerInterface interface {
	AcquireLock(filename string, timeout time.Duration) error
	ReleaseLock(filename string) error
}
