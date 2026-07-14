package application

import (
	"context"
	"testing"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

func TestHandlePaymentCallbackMarksOrderPaid(t *testing.T) {
	ctx := context.Background()
	repo := newFakeStatefulOrderRepo()
	item := order.New(1001, 2001, 3001, 9900, time.Now())
	item.TicketCategoryID = 4001
	item.SeatIDs = []int64{5001}
	if err := repo.Save(ctx, item); err != nil {
		t.Fatal(err)
	}
	program := &fakeProgramSeatClient{}
	service := NewOrderCommandService(repo, program)

	result, err := service.HandlePaymentCallback(ctx, 1001, 9900, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != order.StatusPaid || result.RefundRequired || !program.confirmed {
		t.Fatalf("result=%+v confirmed=%v", result, program.confirmed)
	}
	updated, _ := repo.FindByOrderNumberSystem(ctx, 1001)
	if updated.Status != order.StatusPaid {
		t.Fatalf("status = %s", updated.Status)
	}
}

func TestHandleLatePaymentRequestsRefundAndMarksRefunded(t *testing.T) {
	ctx := context.Background()
	repo := newFakeStatefulOrderRepo()
	item := order.New(1002, 2001, 3001, 9900, time.Now())
	if err := item.Cancel(time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = repo.Save(ctx, item)
	service := NewOrderCommandService(repo, &fakeProgramSeatClient{})

	result, err := service.HandlePaymentCallback(ctx, 1002, 9900, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RefundRequired || result.Status != order.StatusCancel {
		t.Fatalf("result = %+v", result)
	}
	updated, err := service.MarkRefunded(ctx, 1002)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != order.StatusRefund || updated.RefundedAt == nil {
		t.Fatalf("updated = %+v", updated)
	}
}

func TestHandlePaymentCallbackRejectsAmountMismatch(t *testing.T) {
	ctx := context.Background()
	repo := newFakeStatefulOrderRepo()
	_ = repo.Save(ctx, order.New(1003, 2001, 3001, 9900, time.Now()))
	service := NewOrderCommandService(repo, &fakeProgramSeatClient{})
	_, err := service.HandlePaymentCallback(ctx, 1003, 1, true)
	if !therrors.IsCode(err, therrors.CodeInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
