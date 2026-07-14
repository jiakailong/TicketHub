package worker

import (
	"context"
	"log"
	"time"

	"tickethub/app/program-service/internal/application"
	"tickethub/pkg/mq"
	"tickethub/pkg/observability"
)

type OutboxRunner struct {
	repository application.OutboxRepository
	publisher  mq.Producer
	interval   time.Duration
	lease      time.Duration
	retryDelay time.Duration
	batchSize  int
	enabled    bool
}

func NewOutboxRunner(repository application.OutboxRepository, publisher mq.Producer) OutboxRunner {
	return OutboxRunner{
		repository: repository,
		publisher:  publisher,
		interval:   100 * time.Millisecond,
		lease:      30 * time.Second,
		retryDelay: time.Second,
		batchSize:  100,
		enabled:    repository != nil && publisher != nil,
	}
}

func (r OutboxRunner) Start(ctx context.Context) {
	if !r.enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			r.flush(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (r OutboxRunner) flush(ctx context.Context) {
	events, err := r.repository.Claim(ctx, r.batchSize, r.lease)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("program outbox claim failed: %v", err)
		}
		return
	}
	for _, event := range events {
		if err := r.publisher.Publish(ctx, event); err != nil {
			observability.IncCounter("ticket_hub_outbox_publish_total", map[string]string{"result": "failed", "topic": event.Topic})
			_ = r.repository.MarkRetry(context.WithoutCancel(ctx), event.Header.EventID, time.Now().Add(r.retryDelay), err.Error())
			continue
		}
		if err := r.repository.MarkPublished(ctx, event.Header.EventID, time.Now()); err != nil {
			log.Printf("program outbox completion failed: %v", err)
			continue
		}
		observability.IncCounter("ticket_hub_outbox_publish_total", map[string]string{"result": "published", "topic": event.Topic})
	}
}
