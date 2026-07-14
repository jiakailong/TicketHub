package redis

import (
	"context"
	"strconv"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/application"
	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
)

var beginIdempotencyScript = redislib.NewScript(`
local fingerprint = redis.call("HGET", KEYS[1], "fingerprint")
if fingerprint ~= false then
  if fingerprint ~= ARGV[1] then
    return {-2, 0}
  end
  local status = redis.call("HGET", KEYS[1], "status")
  if status == "completed" then
    return {2, tonumber(redis.call("HGET", KEYS[1], "order_number") or "0")}
  end
  return {0, 0}
end
redis.call("HSET", KEYS[1], "fingerprint", ARGV[1], "status", "processing")
redis.call("PEXPIRE", KEYS[1], ARGV[2])
return {1, 0}
`)

var completeIdempotencyScript = redislib.NewScript(`
if redis.call("HGET", KEYS[1], "fingerprint") ~= ARGV[1] then
  return 0
end
redis.call("HSET", KEYS[1], "status", "completed", "order_number", ARGV[2])
redis.call("PEXPIRE", KEYS[1], ARGV[3])
return 1
`)

var abortIdempotencyScript = redislib.NewScript(`
if redis.call("HGET", KEYS[1], "fingerprint") == ARGV[1]
  and redis.call("HGET", KEYS[1], "status") == "processing" then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type IdempotencyStore struct {
	client redislib.Cmdable
	keys   cache.KeyBuilder
}

func NewIdempotencyStore(client redislib.Cmdable, keys cache.KeyBuilder) IdempotencyStore {
	return IdempotencyStore{client: client, keys: keys}
}

func (s IdempotencyStore) Begin(ctx context.Context, key string, fingerprint string, ttl time.Duration) (application.IdempotencyResult, error) {
	value, err := beginIdempotencyScript.Run(ctx, s.client, []string{s.key(key)}, fingerprint, ttl.Milliseconds()).Result()
	if err != nil {
		return application.IdempotencyResult{}, therrors.Wrap(therrors.CodeInfrastructure, "begin order idempotency failed", err)
	}
	parts, ok := value.([]any)
	if !ok || len(parts) != 2 {
		return application.IdempotencyResult{}, therrors.New(therrors.CodeInfrastructure, "invalid idempotency response")
	}
	code := redisResultInt64(parts[0])
	switch code {
	case 1:
		return application.IdempotencyResult{State: application.IdempotencyAcquired}, nil
	case 2:
		return application.IdempotencyResult{State: application.IdempotencyCompleted, OrderNumber: redisResultInt64(parts[1])}, nil
	case 0:
		return application.IdempotencyResult{State: application.IdempotencyProcessing}, nil
	default:
		return application.IdempotencyResult{}, therrors.New(therrors.CodeDuplicateSubmission, "idempotency key was reused with a different request")
	}
}

func (s IdempotencyStore) Complete(ctx context.Context, key string, fingerprint string, orderNumber int64, ttl time.Duration) error {
	result, err := completeIdempotencyScript.Run(ctx, s.client, []string{s.key(key)}, fingerprint, orderNumber, ttl.Milliseconds()).Int64()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "complete order idempotency failed", err)
	}
	if result != 1 {
		return therrors.New(therrors.CodeConflict, "order idempotency ownership was lost")
	}
	return nil
}

func (s IdempotencyStore) Abort(ctx context.Context, key string, fingerprint string) error {
	if _, err := abortIdempotencyScript.Run(ctx, s.client, []string{s.key(key)}, fingerprint).Result(); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "abort order idempotency failed", err)
	}
	return nil
}

func (s IdempotencyStore) key(key string) string {
	return s.keys.Build("idempotency", "create-order", key)
}

func redisResultInt64(value any) int64 {
	switch current := value.(type) {
	case int64:
		return current
	case string:
		parsed, _ := strconv.ParseInt(current, 10, 64)
		return parsed
	case []byte:
		parsed, _ := strconv.ParseInt(string(current), 10, 64)
		return parsed
	default:
		return 0
	}
}
