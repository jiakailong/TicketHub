package mq

import (
	"context"
	"time"
)

type Header struct {
	EventID       string
	TraceID       string
	SchemaVersion string
	OccurredAt    time.Time
}

type Event struct {
	Topic   string
	Key     string
	Header  Header
	Payload []byte
	ack     func(context.Context) error
}

func (e Event) Ack(ctx context.Context) error {
	if e.ack == nil {
		return nil
	}
	return e.ack(ctx)
}

type CreateOrderEvent struct {
	OrderNumber      int64     `json:"order_number"`
	ProgramID        int64     `json:"program_id"`
	UserID           int64     `json:"user_id"`
	TicketCategoryID int64     `json:"ticket_category_id"`
	SeatIDs          []int64   `json:"seat_ids"`
	TicketUserIDs    []int64   `json:"ticket_user_ids"`
	AmountCent       int64     `json:"amount_cent"`
	IdentifierID     int64     `json:"identifier_id"`
	OrderVersion     string    `json:"order_version"`
	CreatedAt        time.Time `json:"created_at"`
}

type ProgramChangedEvent struct {
	ProgramID int64     `json:"program_id"`
	Title     string    `json:"title"`
	City      string    `json:"city"`
	Place     string    `json:"place"`
	ShowTime  time.Time `json:"show_time"`
	Status    string    `json:"status"`
	Operation string    `json:"operation"`
}
