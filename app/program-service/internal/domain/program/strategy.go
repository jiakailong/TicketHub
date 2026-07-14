package program

import "context"

type CreateOrderCommand struct {
	RequestID        string
	UserID           int64
	ProgramID        int64
	TicketCategoryID int64
	SeatIDs          []int64
	TicketUserIDs    []int64
}

type CreateOrderResult struct {
	OrderNumber int64
}

func (c CreateOrderCommand) TicketCount() int64 {
	if len(c.SeatIDs) > 0 {
		return int64(len(c.SeatIDs))
	}
	return int64(len(c.TicketUserIDs))
}

type OrderCreateStrategy interface {
	Version() string
	Create(ctx context.Context, cmd CreateOrderCommand) (CreateOrderResult, error)
}
