package redis

import (
	"context"
	"testing"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
)

func TestInventoryLockerUsesPerSeatKeys(t *testing.T) {
	executor := &recordingLuaExecutor{result: int64(8)}
	locker := NewInventoryLocker(cache.NewKeyBuilder("tickethub:program"), executor, 900)
	_, err := locker.LockSeats(context.Background(), program.CreateOrderCommand{
		ProgramID:        10,
		TicketCategoryID: 20,
		SeatIDs:          []int64{31, 32},
	}, 40, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.keys) != 6 {
		t.Fatalf("keys = %v", executor.keys)
	}
	if executor.keys[4] != "tickethub:program:seat:10:31" || executor.keys[5] != "tickethub:program:seat:10:32" {
		t.Fatalf("seat keys = %v", executor.keys[4:])
	}
}

func TestInventoryLockerMapsSeatConflict(t *testing.T) {
	executor := &recordingLuaExecutor{result: int64(-2)}
	locker := NewInventoryLocker(cache.NewKeyBuilder("tickethub:program"), executor, 900)
	_, err := locker.LockSeats(context.Background(), program.CreateOrderCommand{
		ProgramID:        10,
		TicketCategoryID: 20,
		SeatIDs:          []int64{31},
	}, 40, 50)
	if !therrors.IsCode(err, therrors.CodeSeatUnavailable) {
		t.Fatalf("expected seat unavailable, got %v", err)
	}
}

func TestSeatStateWriterPassesSeatOwnershipKeys(t *testing.T) {
	executor := &recordingLuaExecutor{result: int64(1)}
	writer := NewSeatStateWriter(cache.NewKeyBuilder("tickethub:program"), executor)
	if err := writer.ConfirmSeatsSold(context.Background(), 40, 10, []int64{31, 32}, []int64{51, 52}); err != nil {
		t.Fatal(err)
	}
	if len(executor.keys) != 5 || executor.keys[1] != "tickethub:program:seat:10:31" || executor.keys[3] != "tickethub:program:ticket-user:10:51" {
		t.Fatalf("confirm keys = %v", executor.keys)
	}
	if err := writer.ReleaseSeats(context.Background(), 40, 10, 20, []int64{31, 32}, []int64{51, 52}, 2); err != nil {
		t.Fatal(err)
	}
	if len(executor.keys) != 7 || executor.keys[2] != "tickethub:program:order-release:40" || executor.keys[5] != "tickethub:program:ticket-user:10:51" {
		t.Fatalf("release keys = %v", executor.keys)
	}
}

type recordingLuaExecutor struct {
	result any
	script cache.LuaScript
	keys   []string
	args   []any
}

func (e *recordingLuaExecutor) Eval(ctx context.Context, script cache.LuaScript, keys []string, args ...any) (any, error) {
	e.script = script
	e.keys = append([]string(nil), keys...)
	e.args = append([]any(nil), args...)
	return e.result, nil
}
