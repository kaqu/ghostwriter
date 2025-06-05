package lock

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

const (
	testLockTimeout      = 200 * time.Millisecond
	testPollInterval     = 10 * time.Millisecond
	veryShortTimeout     = 5 * time.Millisecond
	slightlyLongerThanVS = 15 * time.Millisecond
)

func TestLockManager_NewLockManager(t *testing.T) {
	lm := NewLockManager(5, time.Second)
	if lm == nil {
		t.Fatal("NewLockManager returned nil")
	}
	if lm.maxConcurrentOps != 5 {
		t.Errorf("expected maxConcurrentOps 5, got %d", lm.maxConcurrentOps)
	}
	if lm.defaultLockTimeout != time.Second {
		t.Errorf("expected defaultLockTimeout 1s, got %v", lm.defaultLockTimeout)
	}
	lmZero := NewLockManager(0, time.Second)
	if lmZero.maxConcurrentOps != 1 {
		t.Errorf("expected maxConcurrentOps to default to 1 if 0 passed, got %d", lmZero.maxConcurrentOps)
	}
}

func TestLockManager_AcquireReleaseBasic(t *testing.T) {
	lm := NewLockManager(1, testLockTimeout)
	filename := "testfile.txt"

	err := lm.AcquireLock(filename, testLockTimeout)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if lm.GetCurrentLockCount() != 1 {
		t.Errorf("expected lock count 1, got %d", lm.GetCurrentLockCount())
	}

	// Check if lock is actually held
	if _, ok := lm.locks.Load(filename); !ok {
		t.Errorf("lock for %s not found in map after acquire", filename)
	}

	err = lm.ReleaseLock(filename)
	if err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}
	if lm.GetCurrentLockCount() != 0 {
		t.Errorf("expected lock count 0, got %d", lm.GetCurrentLockCount())
	}

	if _, ok := lm.locks.Load(filename); ok {
		t.Errorf("lock for %s still found in map after release", filename)
	}
}

func TestLockManager_AcquireEmptyFilename(t *testing.T) {
	lm := NewLockManager(1, testLockTimeout)
	err := lm.AcquireLock("", testLockTimeout)
	if !errors.Is(err, ErrFilenameRequired) {
		t.Errorf("expected ErrFilenameRequired, got %v", err)
	}
}

func TestLockManager_ReleaseEmptyFilename(t *testing.T) {
	lm := NewLockManager(1, testLockTimeout)
	err := lm.ReleaseLock("")
	if !errors.Is(err, ErrFilenameRequired) {
		t.Errorf("expected ErrFilenameRequired, got %v", err)
	}
}

func TestLockManager_ReleaseNonExistentLock(t *testing.T) {
	lm := NewLockManager(1, testLockTimeout)
	err := lm.ReleaseLock("nonexistent.txt")
	if !errors.Is(err, ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestLockManager_LockTimeout(t *testing.T) {
	lm := NewLockManager(1, testLockTimeout)
	filename := "timeout.txt"

	// Acquire the lock first
	err := lm.AcquireLock(filename, testLockTimeout)
	if err != nil {
		t.Fatalf("Initial AcquireLock failed: %v", err)
	}

	// Try to acquire it again, this should timeout
	startTime := time.Now()
	err = lm.AcquireLock(filename, veryShortTimeout)
	duration := time.Since(startTime)

	if !errors.Is(err, ErrLockTimeout) {
		t.Errorf("expected ErrLockTimeout, got %v", err)
	}
	if duration < veryShortTimeout {
		t.Errorf("second acquire returned too quickly, duration %v, expected at least %v", duration, veryShortTimeout)
	}
	if duration > veryShortTimeout+testPollInterval*2 { // Allow some buffer
		t.Errorf("second acquire took too long, duration %v, expected around %v", duration, veryShortTimeout)
	}

	// Release the original lock
	err = lm.ReleaseLock(filename)
	if err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}
}

func TestLockManager_MaxConcurrentOps(t *testing.T) {
	maxOps := 3
	lm := NewLockManager(maxOps, testLockTimeout)
	files := []string{"file1.txt", "file2.txt", "file3.txt", "file4.txt"}

	for i := 0; i < maxOps; i++ {
		err := lm.AcquireLock(files[i], veryShortTimeout)
		if err != nil {
			t.Fatalf("AcquireLock for %s failed: %v", files[i], err)
		}
		if lm.GetCurrentLockCount() != i+1 {
			t.Errorf("expected lock count %d, got %d", i+1, lm.GetCurrentLockCount())
		}
	}

	// Next lock should fail due to maxConcurrentOps, or timeout waiting for global capacity
	startTime := time.Now()
	err := lm.AcquireLock(files[maxOps], veryShortTimeout) // file4.txt
	duration := time.Since(startTime)

	if err == nil {
		t.Fatalf("AcquireLock for %s should have failed due to max ops, but succeeded", files[maxOps])
	}
	// Check if the error is a timeout related to global capacity
	if !errors.Is(err, ErrLockTimeout) {
		t.Errorf("expected ErrLockTimeout (for global capacity), got %v", err)
	}
	// Check if the error message indicates it's about global capacity
	expectedErrorMsg := fmt.Sprintf("timeout waiting for global capacity: %s while trying to lock %s", ErrLockTimeout.Error(), files[maxOps])
	if err.Error() != expectedErrorMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}

	if duration < veryShortTimeout {
		t.Errorf("AcquireLock for %s returned too quickly, duration %v", files[maxOps], duration)
	}
	if duration > veryShortTimeout+testPollInterval*2 {
		t.Errorf("AcquireLock for %s took too long, duration %v", files[maxOps], duration)
	}

	// Release one lock
	err = lm.ReleaseLock(files[0])
	if err != nil {
		t.Fatalf("ReleaseLock for %s failed: %v", files[0], err)
	}
	if lm.GetCurrentLockCount() != maxOps-1 {
		t.Errorf("expected lock count %d after release, got %d", maxOps-1, lm.GetCurrentLockCount())
	}

	// Now acquiring the 4th lock should succeed
	err = lm.AcquireLock(files[maxOps], veryShortTimeout)
	if err != nil {
		t.Fatalf("AcquireLock for %s should have succeeded after a release, but failed: %v", files[maxOps], err)
	}
	if lm.GetCurrentLockCount() != maxOps {
		t.Errorf("expected lock count %d after acquiring 4th lock, got %d", maxOps, lm.GetCurrentLockCount())
	}

	// Cleanup
	for i := 1; i < maxOps+1; i++ { // files[1], files[2], files[3] (which is files[maxOps])
		lm.ReleaseLock(files[i])
	}
}

// Helper error types for more specific error checking if needed.
type timeoutError struct{ error }
type genericError struct{ error }

func TestLockManager_ConcurrentAcquireRelease(t *testing.T) {
	lm := NewLockManager(5, testLockTimeout) // Allow multiple concurrent ops
	numGoroutines := 10
	numFiles := 3 // Fewer files than goroutines to ensure contention
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			filename := fmt.Sprintf("file%d.txt", id%numFiles) // Create contention on files

			// Try to acquire lock, don't fail test immediately if it times out under heavy load
			err := lm.AcquireLock(filename, testLockTimeout*2) // Longer timeout for concurrency test
			if err != nil {
				// t.Logf("Goroutine %d: AcquireLock for %s failed (might be okay under load): %v", id, filename, err)
				return // Don't proceed to release if acquire failed
			}

			// Simulate work
			time.Sleep(time.Duration(10+id%10) * time.Millisecond)

			err = lm.ReleaseLock(filename)
			if err != nil {
				t.Errorf("Goroutine %d: ReleaseLock for %s failed: %v", id, filename, err)
			}
		}(i)
	}
	wg.Wait()

	if lm.GetCurrentLockCount() != 0 {
		t.Errorf("Expected lock count 0 after all goroutines finished, got %d", lm.GetCurrentLockCount())
		// Additionally, list remaining locks if any for debugging
		lm.locks.Range(func(key, value interface{}) bool {
			t.Logf("Remaining lock: %s -> %+v", key, value)
			return true
		})
	}
}

func TestLockManager_CleanupExpiredLocks(t *testing.T) {
	shortExpiry := 50 * time.Millisecond
	lm := NewLockManager(2, shortExpiry)

	file1 := "expire1.txt"
	file2 := "expire2.txt"

	err := lm.AcquireLock(file1, testLockTimeout)
	if err != nil {
		t.Fatalf("AcquireLock for %s failed: %v", file1, err)
	}
	// This lock should not expire quickly
	err = lm.AcquireLock(file2, testLockTimeout)
	if err != nil {
		t.Fatalf("AcquireLock for %s failed: %v", file2, err)
	}
	// Override AcquiredAt for file1 to make it seem old
	if lock, ok := lm.locks.Load(file1); ok {
		lockInfo := lock.(*LockInfo)
		lockInfo.AcquiredAt = time.Now().Add(-1 * (shortExpiry + time.Second)) // Make it older than shortExpiry
		lm.locks.Store(file1, lockInfo)
	} else {
		t.Fatalf("Could not retrieve lock for %s to modify AcquiredAt", file1)
	}

	if lm.GetCurrentLockCount() != 2 {
		t.Fatalf("Expected 2 locks initially, got %d", lm.GetCurrentLockCount())
	}

	// Wait for a bit, but less than shortExpiry, so file2 doesn't expire naturally
	time.Sleep(shortExpiry / 2)

	// At this point, file1 is "old", file2 is not.
	// The defaultLockTimeout of lm is shortExpiry.
	// So, file1 should be cleaned up.

	lm.CleanupExpiredLocks()

	if lm.GetCurrentLockCount() != 1 {
		t.Errorf("Expected 1 lock after cleanup, got %d", lm.GetCurrentLockCount())
	}

	if _, ok := lm.locks.Load(file1); ok {
		t.Errorf("%s should have been cleaned up, but still exists", file1)
	}
	if _, ok := lm.locks.Load(file2); !ok {
		t.Errorf("%s should NOT have been cleaned up, but it's gone", file2)
	}

	// Release the remaining lock
	err = lm.ReleaseLock(file2)
	if err != nil {
		t.Fatalf("Failed to release %s: %v", file2, err)
	}
	if lm.GetCurrentLockCount() != 0 {
		t.Errorf("Expected 0 locks after final release, got %d", lm.GetCurrentLockCount())
	}

	// Test cleanup on empty locks
	lm.CleanupExpiredLocks()
	if lm.GetCurrentLockCount() != 0 {
		t.Errorf("Expected 0 locks after cleanup on empty, got %d", lm.GetCurrentLockCount())
	}
}

func TestLockManager_AcquireReleaseStress(t *testing.T) {
	lm := NewLockManager(10, 200*time.Millisecond) // Increased maxOps for stress
	numGoroutines := 50                            // Many goroutines
	iterations := 20                               // Each goroutine acquires/releases multiple times
	files := make([]string, numGoroutines/2)       // Create contention
	for i := range files {
		files[i] = fmt.Sprintf("stressfile%d.txt", i)
	}

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				filename := files[(goroutineID+j)%len(files)] // Cycle through files

				lockAttemptTimeout := 100 * time.Millisecond // Shorter timeout for individual attempt
				// For stress test, make timeout slightly random to vary contention
				// #nosec G404 -- fine for test
				lockAttemptTimeout = time.Duration(rand.Intn(50)+80) * time.Millisecond

				err := lm.AcquireLock(filename, lockAttemptTimeout)
				if err == nil {
					// Simulate some work
					// #nosec G404 -- fine for test
					time.Sleep(time.Duration(rand.Intn(10)+1) * time.Millisecond)

					releaseErr := lm.ReleaseLock(filename)
					if releaseErr != nil {
						// This is a critical error in a stress test
						t.Errorf("Goroutine %d: Failed to release lock for %s: %v", goroutineID, filename, releaseErr)
					}
				} else {
					// Timeouts are acceptable under heavy stress, so just log them
					// t.Logf("Goroutine %d: Failed to acquire lock for %s (timeout: %v): %v", goroutineID, filename, lockAttemptTimeout, err)
				}
			}
		}(i)
	}
	wg.Wait()

	// After all operations, all locks should be released.
	if lm.GetCurrentLockCount() != 0 {
		t.Errorf("Expected final lock count to be 0, got %d", lm.GetCurrentLockCount())
		lm.locks.Range(func(key, value interface{}) bool {
			t.Logf("Lingering lock: %s, Info: %+v", key, value.(*LockInfo))
			return true
		})
	}
}

func TestLockManager_GlobalCapacityTimeoutThenAcquire(t *testing.T) {
	lm := NewLockManager(1, testLockTimeout) // Max 1 op
	file1 := "global_cap_file1.txt"
	file2 := "global_cap_file2.txt"

	// Goroutine 1: Acquire lock on file1 and hold it
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := lm.AcquireLock(file1, testLockTimeout)
		if err != nil {
			t.Errorf("g1: AcquireLock for %s failed: %v", file1, err)
			return
		}
		// Hold the lock for a bit longer than file2's acquire attempt timeout
		time.Sleep(slightlyLongerThanVS + 50*time.Millisecond)

		err = lm.ReleaseLock(file1)
		if err != nil {
			t.Errorf("g1: ReleaseLock for %s failed: %v", file1, err)
		}
	}()

	// Wait for goroutine 1 to acquire the lock
	time.Sleep(testPollInterval * 2) // Give time for g1 to acquire

	// Main goroutine: Try to acquire lock on file2.
	// This should initially block due to global capacity (lm.maxConcurrentOps = 1).
	// It should wait up to `veryShortTimeout` for global capacity.
	// After g1 releases file1, this should then be able to acquire file2.

	startTime := time.Now()
	// Use a longer timeout for file2's lock itself, the key is that the *initial* wait for global capacity should be short.
	// The AcquireLock's timeout parameter is for the *entire operation* of acquiring the lock.
	// The internal polling for global capacity uses shortPollInterval and respects the deadline.
	err := lm.AcquireLock(file2, slightlyLongerThanVS+100*time.Millisecond) // Total timeout for file2
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("Main: AcquireLock for %s failed: %v. Duration: %v", file2, err, duration)
	} else {
		lm.ReleaseLock(file2)
	}

	// We expect this to have waited for g1 to release file1.
	// So duration should be > slightlyLongerThanVS
	if duration < slightlyLongerThanVS {
		t.Errorf("Main: AcquireLock for %s completed too quickly (%v), expected to wait for global capacity.", file2, duration)
	}

	wg.Wait() // Wait for g1 to finish
	if lm.GetCurrentLockCount() != 0 {
		t.Errorf("Expected final lock count 0, got %d", lm.GetCurrentLockCount())
	}
}
