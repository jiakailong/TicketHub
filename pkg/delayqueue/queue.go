package delayqueue

import (
	"context"
	"time"
)

type Message struct {
	ID          string
	Topic       string
	Payload     []byte
	AvailableAt time.Time
	Attempts    int
}

type Queue interface {
	Enqueue(ctx context.Context, msg Message) error
	ClaimDue(ctx context.Context, topic string, now time.Time, limit int) ([]Message, error)
	Ack(ctx context.Context, topic string, id string) error
}
