package order

import "time"

type TicketUserOrder struct {
	ID               int64
	OrderNumber      int64
	UserID           int64
	TicketUserID     int64
	ProgramID        int64
	TicketCategoryID int64
	SeatID           int64
	SeatInfo         string
	PriceCent        int64
}

type OrderProgram struct {
	ID           int64
	OrderNumber  int64
	ProgramID    int64
	IdentifierID int64
}

type RecordType string

const (
	RecordReduce       RecordType = "REDUCE"
	RecordIncrease     RecordType = "INCREASE"
	RecordChangeStatus RecordType = "CHANGE_STATUS"
)

type OrderRecord struct {
	ID          int64
	OrderNumber int64
	ProgramID   int64
	UserID      int64
	Type        RecordType
}

type DiscardOrder struct {
	ID               int64
	ProgramID        int64
	OrderNumber      int64
	UserID           int64
	TicketCategoryID int64
	SeatIDs          []int64
	TicketUserIDs    []int64
	AmountCent       int64
	Reason           string
	Detail           string
	Status           string
	CreatedAt        time.Time
	RetriedAt        *time.Time
}
