package mq

import "context"

type Producer interface {
	Publish(ctx context.Context, event Event) error
}

type Consumer interface {
	Consume(ctx context.Context, topic string, limit int) ([]Event, error)
}
