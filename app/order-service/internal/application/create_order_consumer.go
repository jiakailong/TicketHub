package application

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	"tickethub/pkg/delayqueue"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
	"tickethub/pkg/observability"
)

const CancelOrderDelayTopic = "ticket_hub.cancel_order"

type OrderRepository interface {
	Save(ctx context.Context, order order.Order) error
	FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (order.Order, error)
}

type DiscardOrderRepository interface {
	Save(ctx context.Context, discard order.DiscardOrder) error
}

type CreateOrderConsumer struct {
	orders    OrderRepository
	discards  DiscardOrderRepository
	maxDelay  time.Duration
	nowFunc   func() time.Time
	cancelQ   delayqueue.Queue
	cancelIn  time.Duration
	inventory CreateOrderInventoryRollback
}

type CreateOrderInventoryRollback interface {
	RollbackCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error
}

func NewCreateOrderConsumer(orders OrderRepository, discards DiscardOrderRepository, maxDelay time.Duration) CreateOrderConsumer {
	return CreateOrderConsumer{
		orders:   orders,
		discards: discards,
		maxDelay: maxDelay,
		nowFunc:  time.Now,
	}
}

func (c CreateOrderConsumer) WithInventoryRollback(inventory CreateOrderInventoryRollback) CreateOrderConsumer {
	c.inventory = inventory
	return c
}

func (c CreateOrderConsumer) WithCancelDelayQueue(queue delayqueue.Queue, delay time.Duration) CreateOrderConsumer {
	c.cancelQ = queue
	c.cancelIn = delay
	return c
}

func (c CreateOrderConsumer) Handle(ctx context.Context, event mq.Event) error {
	var payload mq.CreateOrderEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	delay := c.nowFunc().Sub(payload.CreatedAt)
	if delay < 0 {
		delay = 0
	}
	observability.ObserveHistogram("ticket_hub_kafka_consume_delay_seconds", map[string]string{"topic": event.Topic}, delay.Seconds())
	if existing, err := c.orders.FindByOrderNumber(ctx, payload.OrderNumber, payload.UserID); err == nil {
		if existing.Status == order.StatusNoPay {
			return c.enqueueCancelCheck(ctx, existing)
		}
		return nil
	} else if !therrors.IsCode(err, therrors.CodeNotFound) {
		return err
	}
	if c.maxDelay > 0 && delay > c.maxDelay {
		observability.IncCounter("ticket_hub_discard_order_total", map[string]string{"reason": "consumer_delay"})
		count := int64(len(payload.TicketUserIDs))
		if len(payload.SeatIDs) > 0 {
			count = int64(len(payload.SeatIDs))
		}
		if c.inventory != nil {
			if err := c.inventory.RollbackCreateOrder(ctx, payload.OrderNumber, payload.ProgramID, payload.TicketCategoryID, payload.SeatIDs, payload.TicketUserIDs, count); err != nil {
				return err
			}
		}
		return c.discards.Save(ctx, order.DiscardOrder{
			ProgramID:        payload.ProgramID,
			OrderNumber:      payload.OrderNumber,
			UserID:           payload.UserID,
			TicketCategoryID: payload.TicketCategoryID,
			SeatIDs:          append([]int64(nil), payload.SeatIDs...),
			TicketUserIDs:    append([]int64(nil), payload.TicketUserIDs...),
			AmountCent:       payload.AmountCent,
			Reason:           "CONSUMER_DELAY",
			Detail:           "create order event exceeded business delay threshold",
		})
	}
	created := order.New(payload.OrderNumber, payload.ProgramID, payload.UserID, payload.AmountCent, c.nowFunc())
	created.TicketCategoryID = payload.TicketCategoryID
	created.SeatIDs = append([]int64(nil), payload.SeatIDs...)
	created.TicketUserIDs = append([]int64(nil), payload.TicketUserIDs...)
	if err := c.orders.Save(ctx, created); err != nil {
		observability.IncCounter("ticket_hub_order_create_fail_total", map[string]string{"stage": "repository_save"})
		return err
	}
	observability.IncCounter("ticket_hub_order_created_total", nil)
	return c.enqueueCancelCheck(ctx, created)
}

func (c CreateOrderConsumer) enqueueCancelCheck(ctx context.Context, created order.Order) error {
	if c.cancelQ == nil || c.cancelIn <= 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"order_number":       created.OrderNumber,
		"user_id":            created.UserID,
		"program_id":         created.ProgramID,
		"ticket_category_id": created.TicketCategoryID,
		"seat_ids":           created.SeatIDs,
		"ticket_user_ids":    created.TicketUserIDs,
	})
	if err != nil {
		return err
	}
	return c.cancelQ.Enqueue(ctx, delayqueue.Message{
		ID:          strconv.FormatInt(created.OrderNumber, 10),
		Topic:       CancelOrderDelayTopic,
		Payload:     payload,
		AvailableAt: created.CreatedAt.Add(c.cancelIn),
	})
}
