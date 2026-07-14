package program

import "time"

type Program struct {
	ID       int64
	Title    string
	City     string
	Place    string
	ShowTime time.Time
	Status   string
}

type TicketCategory struct {
	ID          int64
	ProgramID   int64
	Name        string
	PriceCent   int64
	Total       int64
	Remain      int64
	SellStarted bool
}

type SeatStatus string

const (
	SeatNoSold SeatStatus = "no_sold"
	SeatLocked SeatStatus = "locked"
	SeatSold   SeatStatus = "sold"
)

type Seat struct {
	ID               int64
	ProgramID        int64
	TicketCategoryID int64
	RowCode          string
	ColCode          string
	PriceCent        int64
	Status           SeatStatus
}

func (s *Seat) Lock() bool {
	if s.Status != SeatNoSold {
		return false
	}
	s.Status = SeatLocked
	return true
}

func (s *Seat) ConfirmSold() bool {
	if s.Status != SeatLocked {
		return false
	}
	s.Status = SeatSold
	return true
}

func (s *Seat) Release() bool {
	if s.Status != SeatLocked {
		return false
	}
	s.Status = SeatNoSold
	return true
}
