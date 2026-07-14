package program

import therrors "tickethub/pkg/errors"

type InventoryService struct{}

func (InventoryService) EnsureEnough(category TicketCategory, count int64) error {
	if count <= 0 {
		return therrors.New(therrors.CodeInvalidArgument, "ticket count must be positive")
	}
	if category.Remain < count {
		return therrors.New(therrors.CodeInventoryNotEnough, "ticket remain number is not enough")
	}
	return nil
}

func (InventoryService) LockSeats(seats []Seat, expectedCount int) ([]Seat, error) {
	if len(seats) < expectedCount {
		return nil, therrors.New(therrors.CodeInventoryNotEnough, "not enough candidate seats")
	}
	locked := make([]Seat, 0, expectedCount)
	for _, seat := range seats {
		if len(locked) == expectedCount {
			break
		}
		if seat.Lock() {
			locked = append(locked, seat)
		}
	}
	if len(locked) != expectedCount {
		return nil, therrors.New(therrors.CodeSeatUnavailable, "seat is unavailable")
	}
	return locked, nil
}
