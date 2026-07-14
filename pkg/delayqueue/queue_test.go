package delayqueue

import (
	"context"
	"testing"
	"time"
)

func TestMemoryQueueClaimDue(t *testing.T) {
	queue := NewMemoryQueue()
	now := time.Now()
	_ = queue.Enqueue(context.Background(), Message{ID: "1", Topic: "order_cancel", AvailableAt: now.Add(-time.Second)})
	_ = queue.Enqueue(context.Background(), Message{ID: "2", Topic: "order_cancel", AvailableAt: now.Add(time.Hour)})
	msgs, err := queue.ClaimDue(context.Background(), "order_cancel", now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].ID != "1" {
		t.Fatalf("claimed = %+v", msgs)
	}
}

func TestMemoryQueueRedeliversUnackedMessageAfterVisibilityTimeout(t *testing.T) {
	now := time.Now()
	queue := NewMemoryQueue().WithVisibilityTimeout(time.Second)
	if err := queue.Enqueue(context.Background(), Message{ID: "1", Topic: "order_cancel", AvailableAt: now}); err != nil {
		t.Fatal(err)
	}
	first, err := queue.ClaimDue(context.Background(), "order_cancel", now, 1)
	if err != nil || len(first) != 1 || first[0].Attempts != 1 {
		t.Fatalf("first claim = %+v, err=%v", first, err)
	}
	visible, err := queue.ClaimDue(context.Background(), "order_cancel", now.Add(500*time.Millisecond), 1)
	if err != nil || len(visible) != 0 {
		t.Fatalf("message must remain invisible: %+v, err=%v", visible, err)
	}
	second, err := queue.ClaimDue(context.Background(), "order_cancel", now.Add(2*time.Second), 1)
	if err != nil || len(second) != 1 || second[0].Attempts != 2 {
		t.Fatalf("second claim = %+v, err=%v", second, err)
	}
	if err := queue.Ack(context.Background(), "order_cancel", "1"); err != nil {
		t.Fatal(err)
	}
	afterAck, err := queue.ClaimDue(context.Background(), "order_cancel", now.Add(time.Minute), 1)
	if err != nil || len(afterAck) != 0 {
		t.Fatalf("acked message returned: %+v, err=%v", afterAck, err)
	}
}
