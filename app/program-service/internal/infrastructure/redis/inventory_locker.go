package redis

import (
	"context"
	"strconv"
	"strings"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
)

var lockInventoryScript = cache.LuaScript{
	Name: "lock_inventory",
	Body: `
local inventory = tonumber(redis.call("GET", KEYS[1]) or "0")
local count = tonumber(ARGV[1])
if redis.call("HGET", KEYS[2], "order_number") == ARGV[2] then
  return inventory
end
if inventory < count then
  return -1
end
for index = 5, #KEYS do
  if redis.call("EXISTS", KEYS[index]) == 1 then
    return -2
  end
end
redis.call("DECRBY", KEYS[1], count)
redis.call("HSET", KEYS[2],
  "order_number", ARGV[2],
  "program_id", ARGV[3],
  "ticket_category_id", ARGV[4],
  "seat_ids", ARGV[5],
  "identifier_id", ARGV[6])
redis.call("EXPIRE", KEYS[2], ARGV[7])
redis.call("DEL", KEYS[3], KEYS[4])
for index = 5, #KEYS do
  redis.call("SET", KEYS[index], "locked:" .. ARGV[2], "EX", ARGV[7])
end
return inventory - count
`,
}

type InventoryLocker struct {
	keys     cache.KeyBuilder
	executor cache.LuaExecutor
	ttlSec   int64
}

func NewInventoryLocker(keys cache.KeyBuilder, executor cache.LuaExecutor, ttlSec int64) InventoryLocker {
	if ttlSec <= 0 {
		ttlSec = 15 * 60
	}
	return InventoryLocker{keys: keys, executor: executor, ttlSec: ttlSec}
}

func (l InventoryLocker) LockSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) ([]program.Seat, error) {
	count := len(cmd.TicketUserIDs)
	if len(cmd.SeatIDs) > 0 {
		count = len(cmd.SeatIDs)
	}
	if count <= 0 {
		return nil, therrors.New(therrors.CodeInvalidArgument, "ticket user or seat is required")
	}

	keys := []string{
		l.keys.Build("inventory", cmd.ProgramID, cmd.TicketCategoryID),
		l.keys.Build("order-lock", orderNumber),
		l.keys.Build("order-rollback", orderNumber),
		l.keys.Build("order-release", orderNumber),
	}
	keys = append(keys, l.resourceKeys(cmd.ProgramID, cmd.SeatIDs, cmd.TicketUserIDs)...)
	result, err := executeLua(ctx, l.executor, lockInventoryScript, keys,
		count,
		orderNumber,
		cmd.ProgramID,
		cmd.TicketCategoryID,
		joinInt64(cmd.SeatIDs),
		identifierID,
		l.ttlSec,
	)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "lock inventory failed", err)
	}
	if toInt64(result) < 0 {
		if toInt64(result) == -2 {
			return nil, therrors.New(therrors.CodeSeatUnavailable, "seat is already locked or sold")
		}
		return nil, therrors.New(therrors.CodeInventoryNotEnough, "ticket inventory is not enough")
	}
	return lockedSeatPlaceholders(cmd), nil
}

func (l InventoryLocker) RollbackSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) error {
	count := len(cmd.TicketUserIDs)
	if len(cmd.SeatIDs) > 0 {
		count = len(cmd.SeatIDs)
	}
	keys := []string{
		l.keys.Build("inventory", cmd.ProgramID, cmd.TicketCategoryID),
		l.keys.Build("order-lock", orderNumber),
		l.keys.Build("order-rollback", orderNumber),
	}
	keys = append(keys, l.resourceKeys(cmd.ProgramID, cmd.SeatIDs, cmd.TicketUserIDs)...)
	_, err := executeLua(ctx, l.executor, rollbackCreateOrderScript, keys, count, orderNumber, l.ttlSec)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "rollback inventory failed", err)
	}
	return nil
}

func (l InventoryLocker) seatKeys(programID int64, seatIDs []int64) []string {
	keys := make([]string, 0, len(seatIDs))
	for _, seatID := range seatIDs {
		keys = append(keys, l.keys.Build("seat", programID, seatID))
	}
	return keys
}

func (l InventoryLocker) resourceKeys(programID int64, seatIDs []int64, ticketUserIDs []int64) []string {
	keys := l.seatKeys(programID, seatIDs)
	for _, ticketUserID := range ticketUserIDs {
		keys = append(keys, l.keys.Build("ticket-user", programID, ticketUserID))
	}
	return keys
}

func joinInt64(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",")
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	default:
		return 0
	}
}

func lockedSeatPlaceholders(cmd program.CreateOrderCommand) []program.Seat {
	seats := make([]program.Seat, 0, len(cmd.SeatIDs))
	for _, seatID := range cmd.SeatIDs {
		seats = append(seats, program.Seat{
			ID:               seatID,
			ProgramID:        cmd.ProgramID,
			TicketCategoryID: cmd.TicketCategoryID,
			Status:           program.SeatLocked,
		})
	}
	return seats
}
