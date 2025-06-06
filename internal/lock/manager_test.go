package lock

import (
	"errors"
	"sync"
	"testing"
	"time"
)

const testTimeout = 50 * time.Millisecond

func TestNewLockManager(t *testing.T) {
	if lm := NewLockManager(); lm == nil {
		t.Fatal("NewLockManager returned nil")
	}
}

func TestAcquireAndRelease(t *testing.T) {
	lm := NewLockManager()
	h, err := lm.AcquireLock("test.txt", testTimeout)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if err := lm.ReleaseLock(h); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}
}

func TestAcquireEmptyFilename(t *testing.T) {
	lm := NewLockManager()
	if _, err := lm.AcquireLock("", testTimeout); !errors.Is(err, ErrFilenameRequired) {
		t.Errorf("expected ErrFilenameRequired, got %v", err)
	}
}

func TestReleaseNil(t *testing.T) {
	lm := NewLockManager()
	if err := lm.ReleaseLock(nil); !errors.Is(err, ErrNilLock) {
		t.Errorf("expected ErrNilLock, got %v", err)
	}
}

func TestLockTimeout(t *testing.T) {
	lm := NewLockManager()
	h, err := lm.AcquireLock("t.txt", testTimeout)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer func() {
		if err := lm.ReleaseLock(h); err != nil {
			t.Fatalf("ReleaseLock failed: %v", err)
		}
	}()
	start := time.Now()
	if _, err := lm.AcquireLock("t.txt", 10*time.Millisecond); !errors.Is(err, ErrLockTimeout) {
		t.Errorf("expected ErrLockTimeout, got %v", err)
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Errorf("AcquireLock returned too quickly")
	}
}

func TestConcurrentAcquire(t *testing.T) {
	lm := NewLockManager()
	var wg sync.WaitGroup
	success := 0
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := lm.AcquireLock("c.txt", testTimeout)
			if err == nil {
				success++
				time.Sleep(5 * time.Millisecond)
				_ = lm.ReleaseLock(h)
			}
		}()
	}
	wg.Wait()
	if success == 0 {
		t.Errorf("expected at least one successful acquire, got %d", success)
	}
}
