package redis

import (
	"context"
	"time"

	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/observability"
)

var rollbackCreateOrderScript = cache.LuaScript{
	Name: "rollback_create_order",
	Body: `
local count = tonumber(ARGV[1])
local order_number = ARGV[2]
local ttl = tonumber(ARGV[3])
if redis.call("SET", KEYS[3], "1", "NX", "EX", ttl) == false then
  return 0
end
redis.call("INCRBY", KEYS[1], count)
redis.call("DEL", KEYS[2])
for index = 4, #KEYS do
  if redis.call("GET", KEYS[index]) == "locked:" .. order_number then
    redis.call("DEL", KEYS[index])
  end
end
return 1
`,
}

var confirmSeatsSoldScript = cache.LuaScript{
	Name: "confirm_seats_sold",
	Body: `
local order_number = ARGV[1]
local ttl = tonumber(ARGV[2])
for index = 2, #KEYS do
  local value = redis.call("GET", KEYS[index])
  if value ~= false and value ~= "locked:" .. order_number and value ~= "sold:" .. order_number then
    return -1
  end
end
redis.call("HSET", KEYS[1], "status", "sold")
redis.call("EXPIRE", KEYS[1], ttl)
for index = 2, #KEYS do
  redis.call("SET", KEYS[index], "sold:" .. order_number)
end
return 1
`,
}

var releaseSeatsScript = cache.LuaScript{
	Name: "release_seats",
	Body: `
local count = tonumber(ARGV[1])
local order_number = ARGV[2]
local ttl = tonumber(ARGV[3])
for index = 4, #KEYS do
  local value = redis.call("GET", KEYS[index])
  if value ~= false and value ~= "locked:" .. order_number then
    return -1
  end
end
if redis.call("SET", KEYS[3], "1", "NX", "EX", ttl) == false then
  return 0
end
redis.call("INCRBY", KEYS[1], count)
redis.call("DEL", KEYS[2])
for index = 4, #KEYS do
  if redis.call("GET", KEYS[index]) == "locked:" .. order_number then
    redis.call("DEL", KEYS[index])
  end
end
return 1
`,
}

type SeatStateWriter struct {
	keys      cache.KeyBuilder
	executor  cache.LuaExecutor
	markerTTL int64
}

func NewSeatStateWriter(keys cache.KeyBuilder, executor cache.LuaExecutor) SeatStateWriter {
	return SeatStateWriter{keys: keys, executor: executor, markerTTL: 7 * 24 * 60 * 60}
}

func (w SeatStateWriter) RollbackCreateOrder(ctx context.Context, programID int64, ticketCategoryID int64, orderNumber int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	keys := []string{
		w.keys.Build("inventory", programID, ticketCategoryID),
		w.keys.Build("order-lock", orderNumber),
		w.keys.Build("order-rollback", orderNumber),
	}
	keys = append(keys, w.seatKeys(programID, seatIDs)...)
	keys = append(keys, w.ticketUserKeys(programID, ticketUserIDs)...)
	_, err := executeLua(ctx, w.executor, rollbackCreateOrderScript, keys, count, orderNumber, w.markerTTL)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "rollback create order failed", err)
	}
	return nil
}

func (w SeatStateWriter) ConfirmSeatsSold(ctx context.Context, orderNumber int64, programID int64, seatIDs []int64, ticketUserIDs []int64) error {
	keys := []string{
		w.keys.Build("order-lock", orderNumber),
	}
	keys = append(keys, w.seatKeys(programID, seatIDs)...)
	keys = append(keys, w.ticketUserKeys(programID, ticketUserIDs)...)
	result, err := executeLua(ctx, w.executor, confirmSeatsSoldScript, keys, orderNumber, w.markerTTL)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "confirm seats sold failed", err)
	}
	if toInt64(result) < 0 {
		return therrors.New(therrors.CodeSeatUnavailable, "seat ownership changed before payment confirmation")
	}
	return nil
}

func (w SeatStateWriter) ReleaseSeats(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	keys := []string{
		w.keys.Build("inventory", programID, ticketCategoryID),
		w.keys.Build("order-lock", orderNumber),
		w.keys.Build("order-release", orderNumber),
	}
	keys = append(keys, w.seatKeys(programID, seatIDs)...)
	keys = append(keys, w.ticketUserKeys(programID, ticketUserIDs)...)
	result, err := executeLua(ctx, w.executor, releaseSeatsScript, keys, count, orderNumber, w.markerTTL)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "release seats failed", err)
	}
	if toInt64(result) < 0 {
		return therrors.New(therrors.CodeSeatUnavailable, "seat ownership changed before release")
	}
	return nil
}

func executeLua(ctx context.Context, executor cache.LuaExecutor, script cache.LuaScript, keys []string, args ...any) (any, error) {
	startedAt := time.Now()
	result, err := executor.Eval(ctx, script, keys, args...)
	labels := map[string]string{"script": script.Name, "result": "ok"}
	if err != nil {
		labels["result"] = "error"
	}
	observability.ObserveHistogram("ticket_hub_redis_lua_latency_seconds", labels, time.Since(startedAt).Seconds())
	return result, err
}

func (w SeatStateWriter) seatKeys(programID int64, seatIDs []int64) []string {
	keys := make([]string, 0, len(seatIDs))
	for _, seatID := range seatIDs {
		keys = append(keys, w.keys.Build("seat", programID, seatID))
	}
	return keys
}

func (w SeatStateWriter) ticketUserKeys(programID int64, ticketUserIDs []int64) []string {
	keys := make([]string, 0, len(ticketUserIDs))
	for _, ticketUserID := range ticketUserIDs {
		keys = append(keys, w.keys.Build("ticket-user", programID, ticketUserID))
	}
	return keys
}
