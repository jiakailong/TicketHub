package program

import (
	"testing"

	therrors "tickethub/pkg/errors"
)

func TestInventoryServiceLockSeats(t *testing.T) {
	service := InventoryService{}
	seats := []Seat{
		{ID: 1, Status: SeatNoSold},
		{ID: 2, Status: SeatSold},
		{ID: 3, Status: SeatNoSold},
	}
	locked, err := service.LockSeats(seats, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(locked) != 2 || locked[0].Status != SeatLocked || locked[1].Status != SeatLocked {
		t.Fatalf("locked = %+v", locked)
	}
}

func TestInventoryServiceNotEnough(t *testing.T) {
	service := InventoryService{}
	err := service.EnsureEnough(TicketCategory{Remain: 1}, 2)
	if !therrors.IsCode(err, therrors.CodeInventoryNotEnough) {
		t.Fatalf("err = %v", err)
	}
}
