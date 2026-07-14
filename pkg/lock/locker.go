package lock

import (
	"context"
	"errors"
	"time"
)

var ErrNotAcquired = errors.New("lock not acquired")

type Lock interface {
	Key() string
	Token() string
	Release(ctx context.Context) error
}

type Locker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error)
}
