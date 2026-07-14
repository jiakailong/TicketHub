package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type RedisLuaExecutor struct {
	client redis.Cmdable
}

func NewRedisLuaExecutor(client redis.Cmdable) RedisLuaExecutor {
	return RedisLuaExecutor{client: client}
}

func (e RedisLuaExecutor) Eval(ctx context.Context, script LuaScript, keys []string, args ...any) (any, error) {
	return e.client.Eval(ctx, script.Body, keys, args...).Result()
}
