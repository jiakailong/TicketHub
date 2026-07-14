//go:build integration

package redis

import (
	"context"
	"fmt"
	"testing"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
)

func TestInventoryCanBeReservedAgainAfterDiscardRollback(t *testing.T) {
	ctx := context.Background()
	client := redislib.NewClient(&redislib.Options{Addr: "127.0.0.1:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis is unavailable: %v", err)
	}
	prefix := fmt.Sprintf("tickethub:test:inventory:%d", time.Now().UnixNano())
	keys := cache.NewKeyBuilder(prefix)
	locker := NewInventoryLocker(keys, cache.NewRedisLuaExecutor(client), 900)
	command := program.CreateOrderCommand{ProgramID: 10, TicketCategoryID: 20, SeatIDs: []int64{30}, TicketUserIDs: []int64{40}}
	orderNumber := int64(50)
	if err := client.Set(ctx, keys.Build("inventory", 10, 20), 5, 0).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		found, _ := client.Keys(ctx, prefix+"*").Result()
		if len(found) > 0 {
			_ = client.Del(ctx, found...).Err()
		}
		_ = client.Close()
	})
	if _, err := locker.LockSeats(ctx, command, orderNumber, 60); err != nil {
		t.Fatal(err)
	}
	if err := locker.RollbackSeats(ctx, command, orderNumber, 60); err != nil {
		t.Fatal(err)
	}
	if _, err := locker.LockSeats(ctx, command, orderNumber, 60); err != nil {
		t.Fatal(err)
	}
	if _, err := locker.LockSeats(ctx, command, orderNumber, 60); err != nil {
		t.Fatal(err)
	}
	remain, err := client.Get(ctx, keys.Build("inventory", 10, 20)).Int64()
	if err != nil || remain != 4 {
		t.Fatalf("idempotent reservation remain=%d err=%v", remain, err)
	}
	if err := locker.RollbackSeats(ctx, command, orderNumber, 60); err != nil {
		t.Fatal(err)
	}
	remain, err = client.Get(ctx, keys.Build("inventory", 10, 20)).Int64()
	if err != nil || remain != 5 {
		t.Fatalf("second rollback remain=%d err=%v", remain, err)
	}
}
