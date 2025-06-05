package fileops

import (
	"errors"
	"sync"
	"time"
)

type lockHandle struct {
	name       string
	mu         *sync.Mutex
	acquiredAt time.Time
}

type LockManager struct {
	mu            sync.Mutex
	locks         map[string]*sync.Mutex
	maxConcurrent int
	timeout       time.Duration
}

func NewLockManager(maxConcurrent int, timeout time.Duration) *LockManager {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	return &LockManager{locks: make(map[string]*sync.Mutex), maxConcurrent: maxConcurrent, timeout: timeout}
}

func (lm *LockManager) Acquire(name string) (*lockHandle, error) {
	start := time.Now()
	for {
		lm.mu.Lock()
		if len(lm.locks) < lm.maxConcurrent {
			if _, ok := lm.locks[name]; !ok {
				m := &sync.Mutex{}
				m.Lock()
				lm.locks[name] = m
				lm.mu.Unlock()
				return &lockHandle{name: name, mu: m, acquiredAt: time.Now()}, nil
			}
		}
		lm.mu.Unlock()
		if time.Since(start) >= lm.timeout {
			return nil, errors.New("lock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (lm *LockManager) Release(h *lockHandle) {
	lm.mu.Lock()
	if m, ok := lm.locks[h.name]; ok && m == h.mu {
		delete(lm.locks, h.name)
	}
	lm.mu.Unlock()
	h.mu.Unlock()
}
