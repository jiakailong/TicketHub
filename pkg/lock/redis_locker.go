package lock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisLocker struct {
	client redis.Cmdable
}

func NewRedisLocker(client redis.Cmdable) RedisLocker {
	return RedisLocker{client: client}
}

func (l RedisLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	token := randomToken()
	ok, err := l.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotAcquired
	}
	return redisLock{client: l.client, key: key, token: token}, nil
}

type redisLock struct {
	client redis.Cmdable
	key    string
	token  string
}

func (l redisLock) Key() string {
	return l.key
}

func (l redisLock) Token() string {
	return l.token
}

func (l redisLock) Release(ctx context.Context) error {
	return l.client.Eval(ctx, ReleaseScript, []string{l.key}, l.token).Err()
}
