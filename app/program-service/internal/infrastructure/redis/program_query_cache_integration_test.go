//go:build integration

package redis

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/domain/program"
)

func TestProgramQueryCacheAgainstRedis(t *testing.T) {
	if os.Getenv("TICKETHUB_INTEGRATION") != "1" {
		t.Skip("set TICKETHUB_INTEGRATION=1 to run Redis integration tests")
	}
	address := os.Getenv("TICKETHUB_TEST_REDIS_ADDR")
	if address == "" {
		address = "127.0.0.1:6379"
	}
	client := redislib.NewClient(&redislib.Options{Addr: address})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}

	prefix := fmt.Sprintf("tickethub:test:query-cache:%d", time.Now().UnixNano())
	source := &countingProgramSource{
		program: program.Program{ID: 105, Title: "Redis Integration Program"},
		delay:   80 * time.Millisecond,
	}
	first, _ := newTestProgramQueryCache(t, source, client, prefix)
	second, secondLocal := newTestProgramQueryCache(t, source, client, prefix)
	runConcurrentProgramQueries(t, 80, func(index int) error {
		queryCache := first
		if index%2 == 1 {
			queryCache = second
		}
		_, err := queryCache.FindProgram(context.Background(), 105)
		return err
	})
	if calls := source.programCalls.Load(); calls != 1 {
		t.Fatalf("source calls = %d, want 1", calls)
	}

	subscriberCtx, stopSubscriber := context.WithCancel(context.Background())
	defer stopSubscriber()
	NewCacheInvalidationSubscriber(client, second).Start(subscriberCtx)
	waitForSubscriber(t, client, second.invalidationChannel())
	secondLocal.Wait()
	source.program = program.Program{ID: 105, Title: "Redis Integration Updated"}
	if err := first.Invalidate(context.Background(), 105); err != nil {
		t.Fatal(err)
	}
	eventually(t, time.Second, func() bool {
		item, err := second.FindProgram(context.Background(), 105)
		return err == nil && item.Title == "Redis Integration Updated"
	})

	_ = client.Del(context.Background(), first.cacheKeys(105)...).Err()
}
