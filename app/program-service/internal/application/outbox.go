package application

import (
	"context"
	"time"

	"tickethub/pkg/mq"
)

type OutboxRepository interface {
	Save(ctx context.Context, event mq.Event) error
	Claim(ctx context.Context, limit int, lease time.Duration) ([]mq.Event, error)
	MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkRetry(ctx context.Context, eventID string, availableAt time.Time, detail string) error
}

type OutboxPublisher struct {
	repository OutboxRepository
}

func NewOutboxPublisher(repository OutboxRepository) OutboxPublisher {
	return OutboxPublisher{repository: repository}
}

func (p OutboxPublisher) Publish(ctx context.Context, event mq.Event) error {
	return p.repository.Save(ctx, event)
}
