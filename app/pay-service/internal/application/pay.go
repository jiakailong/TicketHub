package application

import (
	"context"
	"fmt"
	"time"

	"tickethub/app/pay-service/internal/domain/pay"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/lock"
	"tickethub/pkg/observability"
)

type PaymentRepository interface {
	Save(ctx context.Context, payment pay.Payment) error
	Find(ctx context.Context, orderNumber int64) (pay.Payment, error)
}

type OrderPaymentResult struct {
	Status         string
	RefundRequired bool
	AmountCent     int64
}

type OrderPaymentClient interface {
	GetOrderPayment(ctx context.Context, orderNumber int64, userID int64) (OrderPaymentResult, error)
	HandlePaymentCallback(ctx context.Context, orderNumber int64, amountCent int64, paid bool) (OrderPaymentResult, error)
	MarkOrderRefunded(ctx context.Context, orderNumber int64) error
}

type PayUsecase struct {
	repo    PaymentRepository
	gateway pay.Gateway
	orders  OrderPaymentClient
	locker  lock.Locker
	lockTTL time.Duration
}

func NewPayUsecase(repo PaymentRepository, gateway pay.Gateway, orders OrderPaymentClient) PayUsecase {
	return PayUsecase{repo: repo, gateway: gateway, orders: orders, lockTTL: 10 * time.Second}
}

func (u PayUsecase) WithLocker(locker lock.Locker, ttl time.Duration) PayUsecase {
	u.locker = locker
	if ttl > 0 {
		u.lockTTL = ttl
	}
	return u
}

func (u PayUsecase) CommonPay(ctx context.Context, orderNumber int64, userID int64, amountCent int64, channel string) (string, error) {
	var payURL string
	err := u.withPaymentLock(ctx, orderNumber, func() error {
		var err error
		payURL, err = u.commonPay(ctx, orderNumber, userID, amountCent, channel)
		return err
	})
	return payURL, err
}

func (u PayUsecase) commonPay(ctx context.Context, orderNumber int64, userID int64, amountCent int64, channel string) (string, error) {
	if u.orders != nil {
		current, err := u.orders.GetOrderPayment(ctx, orderNumber, userID)
		if err != nil {
			return "", err
		}
		if current.Status != "NO_PAY" {
			return "", therrors.New(therrors.CodeOrderStateConflict, "only unpaid order can start payment")
		}
		if amountCent != current.AmountCent {
			return "", therrors.New(therrors.CodeInvalidArgument, "payment amount does not match order amount")
		}
		amountCent = current.AmountCent
	}
	payment := pay.Payment{OrderNumber: orderNumber, AmountCent: amountCent, Channel: channel, Status: pay.PaymentCreated}
	if err := u.repo.Save(ctx, payment); err != nil {
		return "", err
	}
	return u.gateway.Pay(payment)
}

func (u PayUsecase) TradeCheck(ctx context.Context, orderNumber int64, userID int64, channel string) (pay.Trade, error) {
	if u.orders != nil {
		if _, err := u.orders.GetOrderPayment(ctx, orderNumber, userID); err != nil {
			return pay.Trade{}, err
		}
	}
	return u.gateway.Check(orderNumber, channel)
}

func (u PayUsecase) Refund(ctx context.Context, orderNumber int64, amountCent int64, reason string) error {
	if err := u.gateway.Refund(pay.Refund{
		RequestID:   refundRequestID(orderNumber),
		OrderNumber: orderNumber,
		AmountCent:  amountCent,
		Reason:      reason,
	}); err != nil {
		return err
	}
	current, err := u.repo.Find(ctx, orderNumber)
	if err != nil && !therrors.IsCode(err, therrors.CodeNotFound) {
		return err
	}
	current.OrderNumber = orderNumber
	current.AmountCent = amountCent
	current.Status = pay.PaymentRefunded
	return u.repo.Save(ctx, current)
}

func (u PayUsecase) Callback(ctx context.Context, orderNumber int64, amountCent int64, channel string, paid bool) error {
	return u.withPaymentLock(ctx, orderNumber, func() error {
		return u.callback(ctx, orderNumber, amountCent, channel, paid)
	})
}

func (u PayUsecase) callback(ctx context.Context, orderNumber int64, amountCent int64, channel string, paid bool) error {
	paidLabel := "false"
	if paid {
		paidLabel = "true"
	}
	observability.IncCounter("ticket_hub_payment_callback_total", map[string]string{"channel": channel, "paid": paidLabel})
	current, err := u.repo.Find(ctx, orderNumber)
	found := err == nil
	if err != nil && !therrors.IsCode(err, therrors.CodeNotFound) {
		return err
	}
	if found && current.Status == pay.PaymentRefunded {
		if paid && u.orders != nil {
			return u.orders.MarkOrderRefunded(ctx, orderNumber)
		}
		return nil
	}
	if found && current.Status == pay.PaymentPaid && !paid {
		return nil
	}
	result := OrderPaymentResult{AmountCent: amountCent}
	if u.orders != nil {
		result, err = u.orders.HandlePaymentCallback(ctx, orderNumber, amountCent, paid)
		if err != nil {
			return err
		}
	}
	if !result.RefundRequired {
		status := pay.PaymentClosed
		if paid {
			status = pay.PaymentPaid
		}
		return u.repo.Save(ctx, pay.Payment{OrderNumber: orderNumber, AmountCent: amountCent, Channel: channel, Status: status})
	}
	refundAmount := result.AmountCent
	if refundAmount <= 0 {
		refundAmount = amountCent
	}
	if err := u.gateway.Refund(pay.Refund{
		RequestID:   refundRequestID(orderNumber),
		OrderNumber: orderNumber,
		AmountCent:  refundAmount,
		Reason:      "payment received after order cancellation",
	}); err != nil {
		return err
	}
	if err := u.repo.Save(ctx, pay.Payment{
		OrderNumber: orderNumber,
		AmountCent:  refundAmount,
		Channel:     channel,
		Status:      pay.PaymentRefunded,
	}); err != nil {
		return err
	}
	return u.orders.MarkOrderRefunded(ctx, orderNumber)
}

func (u PayUsecase) withPaymentLock(ctx context.Context, orderNumber int64, fn func() error) error {
	if u.locker == nil {
		return fn()
	}
	locked, err := u.locker.Acquire(ctx, fmt.Sprintf("tickethub:payment-lock:%d", orderNumber), u.lockTTL)
	if err != nil {
		return err
	}
	defer locked.Release(ctx)
	return fn()
}

func refundRequestID(orderNumber int64) string {
	return fmt.Sprintf("late-payment-%d", orderNumber)
}
