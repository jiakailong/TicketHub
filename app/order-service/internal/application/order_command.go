package application

import (
	"context"
	"fmt"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/lock"
)

type ProgramSeatClient interface {
	ConfirmSeatsSold(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64) error
	ReleaseSeats(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error
}

type OrderCommandService struct {
	orders  order.Repository
	program ProgramSeatClient
	nowFunc func() time.Time
	locker  lock.Locker
	lockTTL time.Duration
}

type systemOrderRepository interface {
	FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error)
}

type PaymentCallbackResult struct {
	Status         order.Status
	RefundRequired bool
	AmountCent     int64
}

func NewOrderCommandService(orders order.Repository, program ProgramSeatClient) OrderCommandService {
	return OrderCommandService{orders: orders, program: program, nowFunc: time.Now, lockTTL: 5 * time.Second}
}

func (s OrderCommandService) WithLocker(locker lock.Locker, ttl time.Duration) OrderCommandService {
	s.locker = locker
	if ttl > 0 {
		s.lockTTL = ttl
	}
	return s
}

func (s OrderCommandService) Cancel(ctx context.Context, orderNumber int64, userID int64) error {
	return s.withOrderLock(ctx, orderNumber, func() error {
		return s.cancel(ctx, orderNumber, userID)
	})
}

func (s OrderCommandService) cancel(ctx context.Context, orderNumber int64, userID int64) error {
	current, err := s.orders.FindByOrderNumber(ctx, orderNumber, userID)
	if err != nil {
		return err
	}
	alreadyCanceled := current.Status == order.StatusCancel
	if err := current.Cancel(s.nowFunc()); err != nil {
		return err
	}
	if !alreadyCanceled {
		if err := s.orders.Update(ctx, current); err != nil {
			return err
		}
	}
	return s.program.ReleaseSeats(ctx, current.OrderNumber, current.ProgramID, current.TicketCategoryID, current.SeatIDs, current.TicketUserIDs, current.TicketCount())
}

func (s OrderCommandService) CloseExpired(ctx context.Context, orderNumber int64, userID int64) error {
	return s.withOrderLock(ctx, orderNumber, func() error {
		current, err := s.orders.FindByOrderNumber(ctx, orderNumber, userID)
		if therrors.IsCode(err, therrors.CodeNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		switch current.Status {
		case order.StatusPaid, order.StatusRefund:
			return nil
		case order.StatusNoPay, order.StatusCancel:
			return s.cancel(ctx, orderNumber, userID)
		default:
			return therrors.New(therrors.CodeOrderStateConflict, "order cannot be closed in current state")
		}
	})
}

func (s OrderCommandService) MarkPaid(ctx context.Context, orderNumber int64, userID int64) error {
	return s.withOrderLock(ctx, orderNumber, func() error {
		return s.markPaid(ctx, orderNumber, userID)
	})
}

func (s OrderCommandService) markPaid(ctx context.Context, orderNumber int64, userID int64) error {
	current, err := s.orders.FindByOrderNumber(ctx, orderNumber, userID)
	if err != nil {
		return err
	}
	return s.persistPaid(ctx, &current)
}

func (s OrderCommandService) persistPaid(ctx context.Context, current *order.Order) error {
	alreadyPaid := current.Status == order.StatusPaid
	if err := current.MarkPaid(s.nowFunc()); err != nil {
		return err
	}
	if !alreadyPaid {
		if err := s.orders.Update(ctx, *current); err != nil {
			return err
		}
	}
	return s.program.ConfirmSeatsSold(ctx, current.OrderNumber, current.ProgramID, current.TicketCategoryID, current.SeatIDs, current.TicketUserIDs)
}

func (s OrderCommandService) HandlePaymentCallback(ctx context.Context, orderNumber int64, amountCent int64, paid bool) (PaymentCallbackResult, error) {
	var result PaymentCallbackResult
	err := s.withOrderLock(ctx, orderNumber, func() error {
		current, err := s.findSystemOrder(ctx, orderNumber)
		if err != nil {
			return err
		}
		result = PaymentCallbackResult{Status: current.Status, AmountCent: current.AmountCent}
		if amountCent != current.AmountCent {
			return therrors.New(therrors.CodeInvalidArgument, "payment amount does not match order amount")
		}
		if !paid {
			return nil
		}
		switch current.Status {
		case order.StatusCancel:
			result.RefundRequired = true
			return nil
		case order.StatusRefund:
			return nil
		case order.StatusNoPay, order.StatusPaid:
			if err := s.persistPaid(ctx, &current); err != nil {
				return err
			}
			result.Status = current.Status
			return nil
		default:
			return therrors.New(therrors.CodeOrderStateConflict, "order cannot accept payment callback in current state")
		}
	})
	return result, err
}

func (s OrderCommandService) MarkRefunded(ctx context.Context, orderNumber int64) (order.Order, error) {
	var current order.Order
	err := s.withOrderLock(ctx, orderNumber, func() error {
		found, err := s.findSystemOrder(ctx, orderNumber)
		if err != nil {
			return err
		}
		current = found
		alreadyRefunded := current.Status == order.StatusRefund
		if err := current.MarkRefund(s.nowFunc()); err != nil {
			return err
		}
		if alreadyRefunded {
			return nil
		}
		return s.orders.Update(ctx, current)
	})
	return current, err
}

func (s OrderCommandService) findSystemOrder(ctx context.Context, orderNumber int64) (order.Order, error) {
	repo, ok := s.orders.(systemOrderRepository)
	if !ok {
		return order.Order{}, therrors.New(therrors.CodeInfrastructure, "system order lookup is not supported")
	}
	return repo.FindByOrderNumberSystem(ctx, orderNumber)
}

func (s OrderCommandService) withOrderLock(ctx context.Context, orderNumber int64, fn func() error) error {
	if s.locker == nil {
		return fn()
	}
	locked, err := s.locker.Acquire(ctx, fmt.Sprintf("tickethub:order-lock:%d", orderNumber), s.lockTTL)
	if err != nil {
		return err
	}
	defer locked.Release(ctx)
	return fn()
}
