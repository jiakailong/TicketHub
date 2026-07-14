package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	"tickethub/pkg/mq"
)

func TestDiscardOrderCompensationServiceRetryByIDPublishesEventAndMarksRetried(t *testing.T) {
	repo := newFakeDiscardRepository()
	if err := repo.Save(context.Background(), order.DiscardOrder{
		ProgramID:        10001,
		OrderNumber:      20260710001,
		UserID:           30001,
		TicketCategoryID: 40001,
		SeatIDs:          []int64{50001},
		TicketUserIDs:    []int64{60001},
		AmountCent:       128000,
		Reason:           "CONSUMER_DELAY",
	}); err != nil {
		t.Fatal(err)
	}
	broker := mq.NewMemoryBroker()
	inventory := &fakeDiscardInventory{}
	service := NewDiscardOrderCompensationService(repo, broker, DefaultCreateOrderTopic).WithInventory(inventory)
	service.nowFunc = func() time.Time { return time.Unix(10, 0) }

	result, err := service.RetryByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Retried != 1 || result.Skipped != 0 {
		t.Fatalf("result = %+v", result)
	}
	events, err := broker.Consume(context.Background(), DefaultCreateOrderTopic, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	var payload mq.CreateOrderEvent
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.OrderNumber != 20260710001 || payload.UserID != 30001 || payload.OrderVersion != "retry-discard" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.TicketCategoryID != 40001 || len(payload.SeatIDs) != 1 || len(payload.TicketUserIDs) != 1 || payload.AmountCent != 128000 {
		t.Fatalf("payload snapshot = %+v", payload)
	}
	if repo.records[1].Status != "RETRIED" {
		t.Fatalf("discard status = %s", repo.records[1].Status)
	}
	if inventory.reserveCalls != 1 || inventory.lastOrderNumber != 20260710001 {
		t.Fatalf("inventory reservation = %+v", inventory)
	}
}

func TestDiscardOrderCompensationRollsBackReservationWhenPublishFails(t *testing.T) {
	repo := newFakeDiscardRepository()
	_ = repo.Save(context.Background(), order.DiscardOrder{ProgramID: 1, OrderNumber: 2, UserID: 3, TicketCategoryID: 4, TicketUserIDs: []int64{5}, Reason: "CONSUMER_DELAY"})
	inventory := &fakeDiscardInventory{}
	service := NewDiscardOrderCompensationService(repo, failingProducer{}, DefaultCreateOrderTopic).WithInventory(inventory)
	if _, err := service.RetryByID(context.Background(), 1); err == nil {
		t.Fatal("expected publish error")
	}
	if inventory.reserveCalls != 1 || inventory.rollbackCalls != 1 || repo.records[1].Status != "PENDING" {
		t.Fatalf("inventory=%+v discard=%+v", inventory, repo.records[1])
	}
}

func TestDiscardOrderCompensationServiceSkipsRecordsWithoutUserID(t *testing.T) {
	repo := newFakeDiscardRepository()
	if err := repo.Save(context.Background(), order.DiscardOrder{ProgramID: 10001, OrderNumber: 1, Reason: "legacy"}); err != nil {
		t.Fatal(err)
	}
	broker := mq.NewMemoryBroker()
	service := NewDiscardOrderCompensationService(repo, broker, DefaultCreateOrderTopic)

	result, err := service.RetryProgram(context.Background(), 10001, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Retried != 0 || result.Skipped != 1 {
		t.Fatalf("result = %+v", result)
	}
}

type fakeDiscardRepository struct {
	nextID  int64
	records map[int64]order.DiscardOrder
}

type fakeDiscardInventory struct {
	reserveCalls    int
	rollbackCalls   int
	lastOrderNumber int64
}

type failingProducer struct{}

func (failingProducer) Publish(ctx context.Context, event mq.Event) error {
	return errors.New("publish failed")
}

func (i *fakeDiscardInventory) ReserveCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	i.reserveCalls++
	i.lastOrderNumber = orderNumber
	return nil
}

func (i *fakeDiscardInventory) RollbackCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	i.rollbackCalls++
	return nil
}

func newFakeDiscardRepository() *fakeDiscardRepository {
	return &fakeDiscardRepository{nextID: 1, records: make(map[int64]order.DiscardOrder)}
}

func (r *fakeDiscardRepository) Save(ctx context.Context, discard order.DiscardOrder) error {
	if discard.ID == 0 {
		discard.ID = r.nextID
		r.nextID++
	}
	if discard.Status == "" {
		discard.Status = "PENDING"
	}
	r.records[discard.ID] = discard
	return nil
}

func (r *fakeDiscardRepository) ListPending(ctx context.Context, programID int64, limit int) ([]order.DiscardOrder, error) {
	var result []order.DiscardOrder
	for _, item := range r.records {
		if item.Status != "PENDING" {
			continue
		}
		if programID > 0 && item.ProgramID != programID {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *fakeDiscardRepository) FindPendingByID(ctx context.Context, id int64) (order.DiscardOrder, error) {
	return r.records[id], nil
}

func (r *fakeDiscardRepository) MarkRetried(ctx context.Context, id int64, retriedAt time.Time) error {
	item := r.records[id]
	item.Status = "RETRIED"
	item.RetriedAt = &retriedAt
	r.records[id] = item
	return nil
}
