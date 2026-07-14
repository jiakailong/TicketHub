package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type MemoryLocker struct {
	mu    sync.Mutex
	locks map[string]memoryLockState
}

type memoryLockState struct {
	token     string
	expiresAt time.Time
}

func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{locks: make(map[string]memoryLockState)}
}

func (l *MemoryLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if state, ok := l.locks[key]; ok && state.expiresAt.After(now) {
		return nil, fmt.Errorf("lock %s is already held", key)
	}
	token := randomToken()
	l.locks[key] = memoryLockState{token: token, expiresAt: now.Add(ttl)}
	return &memoryLock{key: key, token: token, locker: l}, nil
}

type memoryLock struct {
	key    string
	token  string
	locker *MemoryLocker
}

func (l *memoryLock) Key() string   { return l.key }
func (l *memoryLock) Token() string { return l.token }

func (l *memoryLock) Release(ctx context.Context) error {
	l.locker.mu.Lock()
	defer l.locker.mu.Unlock()

	state, ok := l.locker.locks[l.key]
	if !ok {
		return nil
	}
	if state.token != l.token {
		return fmt.Errorf("lock token mismatch for %s", l.key)
	}
	delete(l.locker.locks, l.key)
	return nil
}

func randomToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
