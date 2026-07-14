package application

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

const DefaultCreateOrderTopic = "ticket_hub.create_order"

type DiscardOrderCompensationRepository interface {
	Save(ctx context.Context, discard order.DiscardOrder) error
	ListPending(ctx context.Context, programID int64, limit int) ([]order.DiscardOrder, error)
	FindPendingByID(ctx context.Context, id int64) (order.DiscardOrder, error)
	MarkRetried(ctx context.Context, id int64, retriedAt time.Time) error
}

type CompensationResult struct {
	Requested int `json:"requested"`
	Retried   int `json:"retried"`
	Skipped   int `json:"skipped"`
}

type DiscardOrderCompensationService struct {
	discards  DiscardOrderCompensationRepository
	events    mq.Producer
	topic     string
	nowFunc   func() time.Time
	inventory DiscardOrderInventory
}

type DiscardOrderInventory interface {
	ReserveCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error
	RollbackCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error
}

func NewDiscardOrderCompensationService(discards DiscardOrderCompensationRepository, events mq.Producer, topic string) DiscardOrderCompensationService {
	if topic == "" {
		topic = DefaultCreateOrderTopic
	}
	return DiscardOrderCompensationService{
		discards: discards,
		events:   events,
		topic:    topic,
		nowFunc:  time.Now,
	}
}

func (s DiscardOrderCompensationService) WithInventory(inventory DiscardOrderInventory) DiscardOrderCompensationService {
	s.inventory = inventory
	return s
}

func (s DiscardOrderCompensationService) ListPending(ctx context.Context, programID int64, limit int) ([]order.DiscardOrder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.discards.ListPending(ctx, programID, limit)
}

func (s DiscardOrderCompensationService) RetryByID(ctx context.Context, id int64) (CompensationResult, error) {
	if id <= 0 {
		return CompensationResult{}, therrors.New(therrors.CodeInvalidArgument, "discard order id is required")
	}
	item, err := s.discards.FindPendingByID(ctx, id)
	if err != nil {
		return CompensationResult{}, err
	}
	result, err := s.retry(ctx, []order.DiscardOrder{item})
	if err != nil {
		return CompensationResult{}, err
	}
	result.Requested = 1
	return result, nil
}

func (s DiscardOrderCompensationService) RetryProgram(ctx context.Context, programID int64, limit int) (CompensationResult, error) {
	items, err := s.ListPending(ctx, programID, limit)
	if err != nil {
		return CompensationResult{}, err
	}
	result, err := s.retry(ctx, items)
	if err != nil {
		return CompensationResult{}, err
	}
	result.Requested = len(items)
	return result, nil
}

func (s DiscardOrderCompensationService) retry(ctx context.Context, items []order.DiscardOrder) (CompensationResult, error) {
	result := CompensationResult{Requested: len(items)}
	for _, item := range items {
		if item.UserID <= 0 {
			result.Skipped++
			continue
		}
		now := s.nowFunc()
		count := int64(len(item.TicketUserIDs))
		if len(item.SeatIDs) > 0 {
			count = int64(len(item.SeatIDs))
		}
		if s.inventory == nil {
			return result, therrors.New(therrors.CodeInfrastructure, "discard order inventory reservation is not configured")
		}
		if err := s.inventory.ReserveCreateOrder(ctx, item.OrderNumber, item.ProgramID, item.TicketCategoryID, item.SeatIDs, item.TicketUserIDs, count); err != nil {
			return result, err
		}
		payload, err := json.Marshal(mq.CreateOrderEvent{
			OrderNumber:      item.OrderNumber,
			ProgramID:        item.ProgramID,
			UserID:           item.UserID,
			TicketCategoryID: item.TicketCategoryID,
			SeatIDs:          append([]int64(nil), item.SeatIDs...),
			TicketUserIDs:    append([]int64(nil), item.TicketUserIDs...),
			AmountCent:       item.AmountCent,
			OrderVersion:     "retry-discard",
			CreatedAt:        now,
		})
		if err != nil {
			return result, err
		}
		if err := s.events.Publish(ctx, mq.Event{
			Topic: s.topic,
			Key:   strconv.FormatInt(item.OrderNumber, 10),
			Header: mq.Header{
				EventID:       strconv.FormatInt(item.ID, 10),
				SchemaVersion: "v1",
				OccurredAt:    now,
			},
			Payload: payload,
		}); err != nil {
			_ = s.inventory.RollbackCreateOrder(ctx, item.OrderNumber, item.ProgramID, item.TicketCategoryID, item.SeatIDs, item.TicketUserIDs, count)
			return result, err
		}
		if err := s.discards.MarkRetried(ctx, item.ID, now); err != nil {
			return result, err
		}
		result.Retried++
	}
	return result, nil
}
