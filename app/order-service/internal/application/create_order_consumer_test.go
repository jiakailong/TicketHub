package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	"tickethub/pkg/delayqueue"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

func TestCreateOrderConsumerSavesNewOrder(t *testing.T) {
	repo := newFakeOrderRepo()
	discards := &fakeDiscardRepo{}
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	consumer := NewCreateOrderConsumer(repo, discards, time.Minute)
	consumer.nowFunc = func() time.Time { return now }

	if err := consumer.Handle(context.Background(), createOrderEvent(t, now)); err != nil {
		t.Fatal(err)
	}
	if repo.saveCount != 1 {
		t.Fatalf("expected one save, got %d", repo.saveCount)
	}
	saved := repo.items[1001]
	if saved.TicketCategoryID != 5001 || len(saved.SeatIDs) != 2 || len(saved.TicketUserIDs) != 2 || saved.AmountCent != 176000 {
		t.Fatalf("saved order snapshot = %+v", saved)
	}
}

func TestCreateOrderConsumerEnqueuesCancelCheck(t *testing.T) {
	repo := newFakeOrderRepo()
	queue := delayqueue.NewMemoryQueue()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	consumer := NewCreateOrderConsumer(repo, &fakeDiscardRepo{}, time.Minute).WithCancelDelayQueue(queue, 15*time.Minute)
	consumer.nowFunc = func() time.Time { return now }

	if err := consumer.Handle(context.Background(), createOrderEvent(t, now)); err != nil {
		t.Fatal(err)
	}
	due, err := queue.ClaimDue(context.Background(), CancelOrderDelayTopic, now.Add(14*time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("cancel check should not be due yet: %+v", due)
	}
	due, err = queue.ClaimDue(context.Background(), CancelOrderDelayTopic, now.Add(16*time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != "1001" {
		t.Fatalf("expected due cancel check, got %+v", due)
	}
	var payload struct {
		TicketCategoryID int64   `json:"ticket_category_id"`
		SeatIDs          []int64 `json:"seat_ids"`
		TicketUserIDs    []int64 `json:"ticket_user_ids"`
	}
	if err := json.Unmarshal(due[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TicketCategoryID != 5001 || len(payload.SeatIDs) != 2 || len(payload.TicketUserIDs) != 2 {
		t.Fatalf("cancel payload = %+v", payload)
	}
}

func TestCreateOrderConsumerIgnoresDuplicateOrder(t *testing.T) {
	repo := newFakeOrderRepo()
	repo.items[1001] = order.New(1001, 2001, 3001, 0, time.Now())
	consumer := NewCreateOrderConsumer(repo, &fakeDiscardRepo{}, time.Minute)
	consumer.nowFunc = time.Now

	if err := consumer.Handle(context.Background(), createOrderEvent(t, time.Now())); err != nil {
		t.Fatal(err)
	}
	if repo.saveCount != 0 {
		t.Fatalf("duplicate event should not save, got %d", repo.saveCount)
	}
}

func TestCreateOrderConsumerRepairsMissingCancelTaskForDuplicateOrder(t *testing.T) {
	ctx := context.Background()
	repo := newFakeOrderRepo()
	now := time.Now().UTC()
	existing := order.New(1001, 2001, 3001, 8800, now.Add(-10*time.Minute))
	if err := repo.Save(ctx, existing); err != nil {
		t.Fatal(err)
	}
	queue := delayqueue.NewMemoryQueue()
	consumer := NewCreateOrderConsumer(repo, &fakeDiscardRepo{}, time.Hour).WithCancelDelayQueue(queue, 15*time.Minute)
	if err := consumer.Handle(ctx, createOrderEvent(t, now)); err != nil {
		t.Fatal(err)
	}
	due, err := queue.ClaimDue(ctx, CancelOrderDelayTopic, now.Add(6*time.Minute), 1)
	if err != nil || len(due) != 1 {
		t.Fatalf("repaired cancel task = %+v, err=%v", due, err)
	}
}

func TestCreateOrderConsumerDiscardsDelayedEvent(t *testing.T) {
	repo := newFakeOrderRepo()
	discards := &fakeDiscardRepo{}
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	inventory := &fakeCreateOrderInventory{}
	consumer := NewCreateOrderConsumer(repo, discards, time.Minute).WithInventoryRollback(inventory)
	consumer.nowFunc = func() time.Time { return now }

	if err := consumer.Handle(context.Background(), createOrderEvent(t, now.Add(-2*time.Minute))); err != nil {
		t.Fatal(err)
	}
	if len(discards.items) != 1 || discards.items[0].Reason != "CONSUMER_DELAY" {
		t.Fatalf("expected delayed discard, got %+v", discards.items)
	}
	if discards.items[0].TicketCategoryID != 5001 || len(discards.items[0].SeatIDs) != 2 {
		t.Fatalf("discard snapshot = %+v", discards.items[0])
	}
	if repo.saveCount != 0 {
		t.Fatalf("delayed event should not save order, got %d", repo.saveCount)
	}
	if inventory.rollbackCount != 1 || inventory.orderNumber != 1001 {
		t.Fatalf("inventory rollback = %+v", inventory)
	}
}

func createOrderEvent(t *testing.T, createdAt time.Time) mq.Event {
	t.Helper()
	payload, err := json.Marshal(mq.CreateOrderEvent{
		OrderNumber:      1001,
		ProgramID:        2001,
		UserID:           3001,
		TicketCategoryID: 5001,
		SeatIDs:          []int64{6001, 6002},
		TicketUserIDs:    []int64{7001, 7002},
		AmountCent:       176000,
		IdentifierID:     4001,
		CreatedAt:        createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return mq.Event{Topic: "ticket_hub.create_order", Key: "1001", Payload: payload}
}

type fakeOrderRepo struct {
	items     map[int64]order.Order
	saveCount int
}

func newFakeOrderRepo() *fakeOrderRepo {
	return &fakeOrderRepo{items: make(map[int64]order.Order)}
}

func (r *fakeOrderRepo) Save(ctx context.Context, item order.Order) error {
	r.saveCount++
	r.items[item.OrderNumber] = item
	return nil
}

func (r *fakeOrderRepo) FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (order.Order, error) {
	item, ok := r.items[orderNumber]
	if !ok || item.UserID != userID {
		return order.Order{}, therrors.New(therrors.CodeNotFound, "order not found")
	}
	return item, nil
}

type fakeDiscardRepo struct {
	items []order.DiscardOrder
}

type fakeCreateOrderInventory struct {
	rollbackCount int
	orderNumber   int64
}

func (i *fakeCreateOrderInventory) RollbackCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	i.rollbackCount++
	i.orderNumber = orderNumber
	return nil
}

func (r *fakeDiscardRepo) Save(ctx context.Context, discard order.DiscardOrder) error {
	r.items = append(r.items, discard)
	return nil
}
