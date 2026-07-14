package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/mq"
)

func TestCreateOrderUsecasePublishesEvent(t *testing.T) {
	inventory := &fakeInventory{}
	publisher := &fakePublisher{}
	usecase := NewCreateOrderUsecase(fakeOrderNumbers(1001), fakeIDs(2001), inventory, publisher, fakePricing(256000)).WithIdempotency(newFakeIdempotencyStore())
	usecase.nowFunc = func() time.Time { return time.Unix(1, 0) }

	result, err := usecase.CreateAsync(context.Background(), sampleCreateOrderCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderNumber != 1001 {
		t.Fatalf("order number = %d", result.OrderNumber)
	}
	if !inventory.locked {
		t.Fatalf("expected inventory to be locked")
	}
	if len(publisher.events) != 1 || publisher.events[0].Topic != CreateOrderTopic {
		t.Fatalf("expected create order event, got %+v", publisher.events)
	}
	var payload mq.CreateOrderEvent
	if err := json.Unmarshal(publisher.events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TicketCategoryID != 1 || len(payload.SeatIDs) != 2 || len(payload.TicketUserIDs) != 2 || payload.AmountCent != 256000 {
		t.Fatalf("payload snapshot = %+v", payload)
	}
}

func TestCreateOrderUsecaseRollbacksInventoryWhenPublishFails(t *testing.T) {
	inventory := &fakeInventory{}
	publisher := &fakePublisher{err: errors.New("kafka unavailable")}
	usecase := NewCreateOrderUsecase(fakeOrderNumbers(1001), fakeIDs(2001), inventory, publisher, fakePricing(256000)).WithIdempotency(newFakeIdempotencyStore())

	if _, err := usecase.CreateAsync(context.Background(), sampleCreateOrderCommand()); err == nil {
		t.Fatal("expected publish error")
	}
	if !inventory.rolledBack {
		t.Fatal("expected inventory rollback")
	}
}

func TestCreateOrderUsecaseReplaysCompletedIdempotentRequest(t *testing.T) {
	inventory := &fakeInventory{}
	publisher := &fakePublisher{}
	store := newFakeIdempotencyStore()
	usecase := NewCreateOrderUsecase(fakeOrderNumbers(1001), fakeIDs(2001), inventory, publisher, fakePricing(256000)).WithIdempotency(store)

	first, err := usecase.CreateAsync(context.Background(), sampleCreateOrderCommand())
	if err != nil {
		t.Fatal(err)
	}
	second, err := usecase.CreateAsync(context.Background(), sampleCreateOrderCommand())
	if err != nil {
		t.Fatal(err)
	}
	if first.OrderNumber != second.OrderNumber || inventory.lockCount != 1 || len(publisher.events) != 1 {
		t.Fatalf("first=%+v second=%+v locks=%d events=%d", first, second, inventory.lockCount, len(publisher.events))
	}
}

func sampleCreateOrderCommand() program.CreateOrderCommand {
	return program.CreateOrderCommand{
		RequestID:        "request-1001",
		UserID:           3001,
		ProgramID:        4001,
		TicketCategoryID: 1,
		SeatIDs:          []int64{10, 11},
		TicketUserIDs:    []int64{20, 21},
	}
}

type fakeIdempotencyStore struct {
	completed map[string]int64
}

func newFakeIdempotencyStore() *fakeIdempotencyStore {
	return &fakeIdempotencyStore{completed: make(map[string]int64)}
}

func (s *fakeIdempotencyStore) Begin(ctx context.Context, key string, fingerprint string, ttl time.Duration) (IdempotencyResult, error) {
	if orderNumber := s.completed[key]; orderNumber > 0 {
		return IdempotencyResult{State: IdempotencyCompleted, OrderNumber: orderNumber}, nil
	}
	return IdempotencyResult{State: IdempotencyAcquired}, nil
}

func (s *fakeIdempotencyStore) Complete(ctx context.Context, key string, fingerprint string, orderNumber int64, ttl time.Duration) error {
	s.completed[key] = orderNumber
	return nil
}

func (s *fakeIdempotencyStore) Abort(ctx context.Context, key string, fingerprint string) error {
	return nil
}

type fakeOrderNumbers int64

func (g fakeOrderNumbers) NextOrderNumber(userID int64) (int64, error) {
	return int64(g), nil
}

type fakeIDs int64

func (g fakeIDs) NextID() (int64, error) {
	return int64(g), nil
}

type fakePricing int64

func (p fakePricing) CalculateAmount(ctx context.Context, cmd program.CreateOrderCommand) (int64, error) {
	return int64(p), nil
}

type fakeInventory struct {
	locked     bool
	rolledBack bool
	lockCount  int
}

func (i *fakeInventory) LockSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) ([]program.Seat, error) {
	i.locked = true
	i.lockCount++
	return []program.Seat{{ID: 10, Status: program.SeatLocked}}, nil
}

func (i *fakeInventory) RollbackSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) error {
	i.rolledBack = true
	return nil
}

type fakePublisher struct {
	events []mq.Event
	err    error
}

func (p *fakePublisher) Publish(ctx context.Context, event mq.Event) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, event)
	return nil
}
