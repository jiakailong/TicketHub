package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/mq"
	"tickethub/pkg/observability"
)

type ProgramEventIndexer interface {
	EnsureIndex(ctx context.Context) error
	UpsertPrograms(ctx context.Context, programs []program.Program) error
	DeletePrograms(ctx context.Context, programIDs []int64) error
}

type ProgramCacheInvalidator interface {
	Invalidate(ctx context.Context, programID int64) error
}

type ProgramEventRunner struct {
	consumer    mq.Consumer
	topic       string
	indexer     ProgramEventIndexer
	cache       ProgramCacheInvalidator
	pollTimeout time.Duration
	enabled     bool
}

func NewProgramEventRunner(consumer mq.Consumer, topic string, indexer ProgramEventIndexer, cache ProgramCacheInvalidator) ProgramEventRunner {
	return ProgramEventRunner{consumer: consumer, topic: topic, indexer: indexer, cache: cache, pollTimeout: time.Second, enabled: consumer != nil && topic != "" && indexer != nil}
}

func (r ProgramEventRunner) Start(ctx context.Context) {
	if !r.enabled {
		return
	}
	go r.run(ctx)
}

func (r ProgramEventRunner) run(ctx context.Context) {
	for ctx.Err() == nil {
		pollCtx, cancel := context.WithTimeout(ctx, r.pollTimeout)
		events, err := r.consumer.Consume(pollCtx, r.topic, 32)
		cancel()
		for _, event := range events {
			if handleErr := r.handle(ctx, event); handleErr != nil {
				log.Printf("program change event failed: %v", handleErr)
				break
			}
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			log.Printf("program change consume failed: %v", err)
			time.Sleep(r.pollTimeout)
		}
	}
}

func (r ProgramEventRunner) handle(ctx context.Context, event mq.Event) error {
	var changed mq.ProgramChangedEvent
	if err := json.Unmarshal(event.Payload, &changed); err != nil {
		return err
	}
	if err := r.indexer.EnsureIndex(ctx); err != nil {
		return err
	}
	if changed.Operation == "DELETE" {
		if err := r.indexer.DeletePrograms(ctx, []int64{changed.ProgramID}); err != nil {
			return err
		}
	} else {
		if err := r.indexer.UpsertPrograms(ctx, []program.Program{{
			ID: changed.ProgramID, Title: changed.Title, City: changed.City, Place: changed.Place,
			ShowTime: changed.ShowTime, Status: changed.Status,
		}}); err != nil {
			return err
		}
	}
	if r.cache != nil {
		if err := r.cache.Invalidate(ctx, changed.ProgramID); err != nil {
			return err
		}
	}
	if err := event.Ack(ctx); err != nil {
		return err
	}
	observability.IncCounter("ticket_hub_program_index_event_total", map[string]string{"operation": changed.Operation})
	return nil
}
