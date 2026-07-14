package redis

import (
	"context"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
)

var purchaseRateLimitScript = redislib.NewScript(`
local now = redis.call("TIME")
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)

local function refill(key, rate, burst)
  local tokens = tonumber(redis.call("HGET", key, "tokens") or burst)
  local last = tonumber(redis.call("HGET", key, "last_ms") or now_ms)
  tokens = math.min(burst, tokens + math.max(0, now_ms - last) * rate / 1000)
  return tokens
end

local user_tokens = refill(KEYS[1], tonumber(ARGV[1]), tonumber(ARGV[2]))
local program_tokens = refill(KEYS[2], tonumber(ARGV[3]), tonumber(ARGV[4]))
local allowed = user_tokens >= 1 and program_tokens >= 1
if allowed then
  user_tokens = user_tokens - 1
  program_tokens = program_tokens - 1
end
redis.call("HSET", KEYS[1], "tokens", user_tokens, "last_ms", now_ms)
redis.call("HSET", KEYS[2], "tokens", program_tokens, "last_ms", now_ms)
redis.call("PEXPIRE", KEYS[1], 60000)
redis.call("PEXPIRE", KEYS[2], 60000)
if allowed then return 1 else return 0 end
`)

type PurchaseRateLimiter struct {
	client       redislib.Cmdable
	keys         cache.KeyBuilder
	userRate     int
	userBurst    int
	programRate  int
	programBurst int
}

func NewPurchaseRateLimiter(client redislib.Cmdable, keys cache.KeyBuilder, userRate int, userBurst int, programRate int, programBurst int) PurchaseRateLimiter {
	return PurchaseRateLimiter{client: client, keys: keys, userRate: userRate, userBurst: userBurst, programRate: programRate, programBurst: programBurst}
}

func (l PurchaseRateLimiter) Allow(ctx context.Context, cmd program.CreateOrderCommand) (bool, error) {
	result, err := purchaseRateLimitScript.Run(ctx, l.client, []string{
		l.keys.Build("rate", "purchase", "user", cmd.UserID, "program", cmd.ProgramID),
		l.keys.Build("rate", "purchase", "program", cmd.ProgramID),
	}, l.userRate, l.userBurst, l.programRate, l.programBurst).Int64()
	if err != nil {
		return false, therrors.Wrap(therrors.CodeInfrastructure, "apply purchase rate limit failed", err)
	}
	return result == 1, nil
}
