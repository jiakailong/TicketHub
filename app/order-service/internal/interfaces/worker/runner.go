package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"tickethub/app/order-service/internal/application"
	"tickethub/pkg/mq"
	"tickethub/pkg/observability"
)

type Runner struct {
	events           mq.Consumer
	createTopic      string
	createOrders     application.CreateOrderConsumer
	cancelOrders     application.CancelOrderWorker
	pollInterval     time.Duration
	createBatchSize  int
	cancelBatchSize  int
	mappingRefresher ShardMappingRefresher
	mappingInterval  time.Duration
}

type ShardMappingRefresher interface {
	Refresh(ctx context.Context) error
}

func NewRunner(events mq.Consumer, createTopic string, createOrders application.CreateOrderConsumer, cancelOrders application.CancelOrderWorker) Runner {
	if createTopic == "" {
		createTopic = application.DefaultCreateOrderTopic
	}
	return Runner{
		events:          events,
		createTopic:     createTopic,
		createOrders:    createOrders,
		cancelOrders:    cancelOrders,
		pollInterval:    time.Second,
		createBatchSize: 32,
		cancelBatchSize: 32,
		mappingInterval: 5 * time.Second,
	}
}

func (r Runner) WithShardMappingRefresher(refresher ShardMappingRefresher, interval time.Duration) Runner {
	r.mappingRefresher = refresher
	if interval > 0 {
		r.mappingInterval = interval
	}
	return r
}

func (r Runner) Start(ctx context.Context) {
	if r.events != nil {
		go r.consumeCreateOrders(ctx)
	}
	go r.pollCancelOrders(ctx)
	if r.mappingRefresher != nil {
		go r.refreshShardMappings(ctx)
	}
}

func (r Runner) refreshShardMappings(ctx context.Context) {
	ticker := time.NewTicker(r.mappingInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := r.mappingRefresher.Refresh(ctx); err != nil && ctx.Err() == nil {
			observability.IncCounter("ticket_hub_shard_mapping_refresh_total", map[string]string{"result": "failed"})
			log.Printf("order-service shard mapping refresh failed, keeping previous snapshot: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r Runner) consumeCreateOrders(ctx context.Context) {
	for ctx.Err() == nil {
		pollCtx, cancel := context.WithTimeout(ctx, r.pollInterval)
		events, err := r.events.Consume(pollCtx, r.createTopic, r.createBatchSize)
		cancel()
		for _, event := range events {
			r.handleCreateOrder(ctx, event)
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			observability.IncCounter("ticket_hub_kafka_consume_fail_total", map[string]string{"topic": r.createTopic})
			log.Printf("order-service create order consume failed: %v", err)
			time.Sleep(r.pollInterval)
		}
	}
}

func (r Runner) handleCreateOrder(ctx context.Context, event mq.Event) {
	for ctx.Err() == nil {
		if err := r.createOrders.Handle(ctx, event); err != nil {
			observability.IncCounter("ticket_hub_order_create_fail_total", map[string]string{"stage": "event_handler"})
			log.Printf("order-service create order event failed, retrying: %v", err)
			if !waitForRetry(ctx, r.pollInterval) {
				return
			}
			continue
		}
		if err := event.Ack(ctx); err != nil {
			observability.IncCounter("ticket_hub_kafka_consume_fail_total", map[string]string{"topic": r.createTopic, "stage": "commit"})
			log.Printf("order-service create order commit failed, retrying: %v", err)
			if !waitForRetry(ctx, r.pollInterval) {
				return
			}
			continue
		}
		return
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r Runner) pollCancelOrders(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := r.cancelOrders.Poll(ctx, r.cancelBatchSize); err != nil && ctx.Err() == nil {
			log.Printf("order-service cancel order poll failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
