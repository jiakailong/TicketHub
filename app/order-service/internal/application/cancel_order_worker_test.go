package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	"tickethub/pkg/delayqueue"
	"tickethub/pkg/lock"
)

func TestCancelOrderWorkerCancelsNoPayOrder(t *testing.T) {
	ctx := context.Background()
	repo := newFakeStatefulOrderRepo()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	item := order.New(1001, 2001, 3001, 99, now.Add(-30*time.Minute))
	item.TicketCategoryID = 4001
	item.SeatIDs = []int64{5001, 5002}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatal(err)
	}
	queue := delayqueue.NewMemoryQueue()
	payload, _ := json.Marshal(map[string]any{"order_number": int64(1001), "user_id": int64(3001), "program_id": int64(2001)})
	if err := queue.Enqueue(ctx, delayqueue.Message{ID: "1001", Topic: CancelOrderDelayTopic, Payload: payload, AvailableAt: now}); err != nil {
		t.Fatal(err)
	}
	program := &fakeProgramSeatClient{}
	commands := NewOrderCommandService(repo, program)
	commands.nowFunc = func() time.Time { return now }
	worker := NewCancelOrderWorker(queue, commands)
	worker.nowFunc = func() time.Time { return now }

	if err := worker.Poll(ctx, 1); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.FindByOrderNumber(ctx, 1001, 3001)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != order.StatusCancel {
		t.Fatalf("status = %s", updated.Status)
	}
	if !program.released {
		t.Fatal("expected seats to be released")
	}
	if program.releaseTicketCategoryID != 4001 || len(program.releaseSeatIDs) != 2 {
		t.Fatalf("release ticket category=%d seat ids=%v", program.releaseTicketCategoryID, program.releaseSeatIDs)
	}
	if program.releaseCount != 2 {
		t.Fatalf("release count = %d", program.releaseCount)
	}
}

func TestCloseExpiredUsesSameOrderLockAsPayment(t *testing.T) {
	ctx := context.Background()
	repo := newFakeStatefulOrderRepo()
	_ = repo.Save(ctx, order.New(1003, 2001, 3001, 99, time.Now()))
	program := &fakeProgramSeatClient{}
	locker := lock.NewMemoryLocker()
	held, err := locker.Acquire(ctx, "tickethub:order-lock:1003", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release(ctx)
	service := NewOrderCommandService(repo, program).WithLocker(locker, time.Minute)
	if err := service.CloseExpired(ctx, 1003, 3001); err == nil {
		t.Fatal("expected close to wait for the shared order lock")
	}
	stored, _ := repo.FindByOrderNumber(ctx, 1003, 3001)
	if stored.Status != order.StatusNoPay || program.released {
		t.Fatalf("order changed without lock: order=%+v released=%v", stored, program.released)
	}
}

func TestCloseExpiredPersistsCancelBeforeReleasingInventory(t *testing.T) {
	ctx := context.Background()
	repo := newFakeStatefulOrderRepo()
	item := order.New(1002, 2001, 3001, 99, time.Now())
	item.TicketUserIDs = []int64{4001}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatal(err)
	}
	program := &fakeProgramSeatClient{onRelease: func() {
		stored, _ := repo.FindByOrderNumber(ctx, 1002, 3001)
		if stored.Status != order.StatusCancel {
			t.Fatalf("status during release = %s", stored.Status)
		}
	}}
	service := NewOrderCommandService(repo, program)
	if err := service.CloseExpired(ctx, 1002, 3001); err != nil {
		t.Fatal(err)
	}
}

type fakeStatefulOrderRepo struct {
	items map[int64]order.Order
}

func newFakeStatefulOrderRepo() *fakeStatefulOrderRepo {
	return &fakeStatefulOrderRepo{items: make(map[int64]order.Order)}
}

func (r *fakeStatefulOrderRepo) Save(ctx context.Context, item order.Order) error {
	r.items[item.OrderNumber] = item
	return nil
}

func (r *fakeStatefulOrderRepo) FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (order.Order, error) {
	return r.items[orderNumber], nil
}

func (r *fakeStatefulOrderRepo) FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error) {
	return r.items[orderNumber], nil
}

func (r *fakeStatefulOrderRepo) Update(ctx context.Context, item order.Order) error {
	r.items[item.OrderNumber] = item
	return nil
}

type fakeProgramSeatClient struct {
	released                bool
	confirmed               bool
	releaseTicketCategoryID int64
	releaseSeatIDs          []int64
	releaseCount            int64
	onRelease               func()
}

func (c *fakeProgramSeatClient) ConfirmSeatsSold(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64) error {
	c.confirmed = true
	return nil
}

func (c *fakeProgramSeatClient) ReleaseSeats(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	if c.onRelease != nil {
		c.onRelease()
	}
	c.released = true
	c.releaseTicketCategoryID = ticketCategoryID
	c.releaseSeatIDs = append([]int64(nil), seatIDs...)
	c.releaseCount = count
	return nil
}
