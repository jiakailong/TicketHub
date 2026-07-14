package application

import (
	"context"
	"encoding/json"
	"time"

	"tickethub/pkg/delayqueue"
)

type ExpiredOrderCloser interface {
	CloseExpired(ctx context.Context, orderNumber int64, userID int64) error
}

type CancelOrderWorker struct {
	queue   delayqueue.Queue
	orders  ExpiredOrderCloser
	nowFunc func() time.Time
}

func NewCancelOrderWorker(queue delayqueue.Queue, orders ExpiredOrderCloser) CancelOrderWorker {
	return CancelOrderWorker{queue: queue, orders: orders, nowFunc: time.Now}
}

func (w CancelOrderWorker) Poll(ctx context.Context, limit int) error {
	messages, err := w.queue.ClaimDue(ctx, CancelOrderDelayTopic, w.nowFunc(), limit)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if err := w.handleMessage(ctx, message); err != nil {
			return err
		}
		if err := w.queue.Ack(ctx, message.Topic, message.ID); err != nil {
			return err
		}
	}
	return nil
}

func (w CancelOrderWorker) handleMessage(ctx context.Context, message delayqueue.Message) error {
	var payload struct {
		OrderNumber int64 `json:"order_number"`
		UserID      int64 `json:"user_id"`
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return err
	}
	return w.orders.CloseExpired(ctx, payload.OrderNumber, payload.UserID)
}
