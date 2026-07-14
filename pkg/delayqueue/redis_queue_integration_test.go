//go:build integration

package delayqueue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisQueueVisibilityAndAck(t *testing.T) {
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis is unavailable: %v", err)
	}
	prefix := fmt.Sprintf("tickethub:test:delayqueue:%d", time.Now().UnixNano())
	queue := NewRedisQueue(client, prefix).WithVisibilityTimeout(time.Second)
	t.Cleanup(func() {
		keys, _ := client.Keys(ctx, prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(ctx, keys...).Err()
		}
		_ = client.Close()
	})
	now := time.Now().UTC()
	if err := queue.Enqueue(ctx, Message{ID: "order-1", Topic: "cancel", Payload: []byte(`{"order_number":1}`), AvailableAt: now}); err != nil {
		t.Fatal(err)
	}
	first, err := queue.ClaimDue(ctx, "cancel", now, 1)
	if err != nil || len(first) != 1 || first[0].Attempts != 1 {
		t.Fatalf("first claim=%+v err=%v", first, err)
	}
	duplicate, err := queue.ClaimDue(ctx, "cancel", now.Add(500*time.Millisecond), 1)
	if err != nil || len(duplicate) != 0 {
		t.Fatalf("visible duplicate=%+v err=%v", duplicate, err)
	}
	redelivered, err := queue.ClaimDue(ctx, "cancel", now.Add(2*time.Second), 1)
	if err != nil || len(redelivered) != 1 || redelivered[0].Attempts != 2 {
		t.Fatalf("redelivery=%+v err=%v", redelivered, err)
	}
	if err := queue.Ack(ctx, "cancel", "order-1"); err != nil {
		t.Fatal(err)
	}
	afterAck, err := queue.ClaimDue(ctx, "cancel", now.Add(time.Minute), 1)
	if err != nil || len(afterAck) != 0 {
		t.Fatalf("after ack=%+v err=%v", afterAck, err)
	}
}
