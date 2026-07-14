package order

import (
	"time"

	therrors "tickethub/pkg/errors"
)

type Status string

const (
	StatusNoPay  Status = "NO_PAY"
	StatusPaid   Status = "PAY"
	StatusCancel Status = "CANCEL"
	StatusRefund Status = "REFUND"
)

type Order struct {
	OrderNumber      int64
	ProgramID        int64
	UserID           int64
	TicketCategoryID int64
	SeatIDs          []int64
	TicketUserIDs    []int64
	AmountCent       int64
	Status           Status
	CreatedAt        time.Time
	PaidAt           *time.Time
	CanceledAt       *time.Time
	RefundedAt       *time.Time
}

func New(orderNumber int64, programID int64, userID int64, amountCent int64, now time.Time) Order {
	return Order{
		OrderNumber: orderNumber,
		ProgramID:   programID,
		UserID:      userID,
		AmountCent:  amountCent,
		Status:      StatusNoPay,
		CreatedAt:   now,
	}
}

func (o *Order) MarkPaid(now time.Time) error {
	if o.Status == StatusPaid {
		return nil
	}
	if o.Status == StatusCancel {
		return therrors.New(therrors.CodeOrderStateConflict, "canceled order must be refunded instead of paid")
	}
	if o.Status != StatusNoPay {
		return therrors.New(therrors.CodeOrderStateConflict, "order cannot be paid in current state")
	}
	o.Status = StatusPaid
	o.PaidAt = &now
	return nil
}

func (o *Order) Cancel(now time.Time) error {
	if o.Status == StatusCancel {
		return nil
	}
	if o.Status != StatusNoPay {
		return therrors.New(therrors.CodeOrderStateConflict, "order cannot be canceled in current state")
	}
	o.Status = StatusCancel
	o.CanceledAt = &now
	return nil
}

func (o *Order) MarkRefund(now time.Time) error {
	if o.Status == StatusRefund {
		return nil
	}
	if o.Status != StatusCancel {
		return therrors.New(therrors.CodeOrderStateConflict, "only canceled order can be refunded")
	}
	o.Status = StatusRefund
	o.RefundedAt = &now
	return nil
}

func (o Order) TicketCount() int64 {
	if len(o.SeatIDs) > 0 {
		return int64(len(o.SeatIDs))
	}
	return int64(len(o.TicketUserIDs))
}
