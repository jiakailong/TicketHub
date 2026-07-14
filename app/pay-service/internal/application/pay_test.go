package application

import (
	"context"
	"testing"

	"tickethub/app/pay-service/internal/domain/pay"
	"tickethub/app/pay-service/internal/infrastructure/memory"
	therrors "tickethub/pkg/errors"
)

func TestLatePaymentCallbackRefundsOnceAndMarksOrderRefunded(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPaymentRepository()
	gateway := &fakePaymentGateway{}
	orders := &fakeOrderPaymentClient{result: OrderPaymentResult{Status: "CANCEL", RefundRequired: true, AmountCent: 9900}}
	service := NewPayUsecase(repo, gateway, orders)

	if err := service.Callback(ctx, 1001, 9900, "mock", true); err != nil {
		t.Fatal(err)
	}
	if err := service.Callback(ctx, 1001, 9900, "mock", true); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Find(ctx, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != pay.PaymentRefunded || gateway.refundCalls != 1 || orders.markRefundedCalls != 2 {
		t.Fatalf("payment=%+v refunds=%d marks=%d", stored, gateway.refundCalls, orders.markRefundedCalls)
	}
}

func TestCallbackValidatesOrderBeforeSavingPaidPayment(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPaymentRepository()
	orders := &fakeOrderPaymentClient{handleErr: therrors.New(therrors.CodeInvalidArgument, "amount mismatch")}
	service := NewPayUsecase(repo, &fakePaymentGateway{}, orders)
	if err := service.Callback(ctx, 1004, 1, "mock", true); err == nil {
		t.Fatal("expected callback validation error")
	}
	if _, err := repo.Find(ctx, 1004); !therrors.IsCode(err, therrors.CodeNotFound) {
		t.Fatalf("invalid callback persisted payment: %v", err)
	}
}

func TestFailedCallbackDoesNotDowngradePaidPayment(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPaymentRepository()
	service := NewPayUsecase(repo, &fakePaymentGateway{}, &fakeOrderPaymentClient{result: OrderPaymentResult{Status: "PAY", AmountCent: 9900}})
	if err := service.Callback(ctx, 1002, 9900, "mock", true); err != nil {
		t.Fatal(err)
	}
	if err := service.Callback(ctx, 1002, 9900, "mock", false); err != nil {
		t.Fatal(err)
	}
	stored, _ := repo.Find(ctx, 1002)
	if stored.Status != pay.PaymentPaid {
		t.Fatalf("status = %s", stored.Status)
	}
}

func TestCommonPayUsesAuthoritativeOrderAmount(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPaymentRepository()
	orders := &fakeOrderPaymentClient{result: OrderPaymentResult{Status: "NO_PAY", AmountCent: 9900}}
	service := NewPayUsecase(repo, &fakePaymentGateway{}, orders)

	if _, err := service.CommonPay(ctx, 1003, 3001, 1, "mock"); err == nil {
		t.Fatal("expected amount mismatch")
	}
	if _, err := service.CommonPay(ctx, 1003, 3001, 9900, "mock"); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Find(ctx, 1003)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AmountCent != 9900 {
		t.Fatalf("amount = %d", stored.AmountCent)
	}
}

type fakePaymentGateway struct {
	refundCalls int
}

func (g *fakePaymentGateway) Pay(payment pay.Payment) (string, error) {
	return "https://pay.local", nil
}

func (g *fakePaymentGateway) Check(orderNumber int64, channel string) (pay.Trade, error) {
	return pay.Trade{OrderNumber: orderNumber, Channel: channel}, nil
}

func (g *fakePaymentGateway) Refund(refund pay.Refund) error {
	g.refundCalls++
	return nil
}

type fakeOrderPaymentClient struct {
	result            OrderPaymentResult
	handleCalls       int
	markRefundedCalls int
	handleErr         error
}

func (c *fakeOrderPaymentClient) GetOrderPayment(ctx context.Context, orderNumber int64, userID int64) (OrderPaymentResult, error) {
	return c.result, nil
}

func (c *fakeOrderPaymentClient) HandlePaymentCallback(ctx context.Context, orderNumber int64, amountCent int64, paid bool) (OrderPaymentResult, error) {
	c.handleCalls++
	if c.handleErr != nil {
		return OrderPaymentResult{}, c.handleErr
	}
	return c.result, nil
}

func (c *fakeOrderPaymentClient) MarkOrderRefunded(ctx context.Context, orderNumber int64) error {
	c.markRefundedCalls++
	return nil
}
