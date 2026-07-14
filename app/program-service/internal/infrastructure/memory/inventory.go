package memory

import (
	"context"
	"sync"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

type InventoryLocker struct {
	mu      sync.Mutex
	remain  map[int64]int64
	seatMap map[int64]program.SeatStatus
}

func NewInventoryLocker() *InventoryLocker {
	return &InventoryLocker{
		remain:  make(map[int64]int64),
		seatMap: make(map[int64]program.SeatStatus),
	}
}

func (l *InventoryLocker) Seed(ticketCategoryID int64, remain int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remain[ticketCategoryID] = remain
}

func (l *InventoryLocker) GetRemain(ctx context.Context, programID int64, ticketCategoryID int64) (int64, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	remain, ok := l.remain[ticketCategoryID]
	return remain, ok, nil
}

func (l *InventoryLocker) SetRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.remain[ticketCategoryID] = remain
	return nil
}

func (l *InventoryLocker) InitializeRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.remain[ticketCategoryID]; exists {
		return false, nil
	}
	l.remain[ticketCategoryID] = remain
	return true, nil
}

func (l *InventoryLocker) LockSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) ([]program.Seat, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := int64(len(cmd.TicketUserIDs))
	if len(cmd.SeatIDs) > 0 {
		count = int64(len(cmd.SeatIDs))
	}
	if count <= 0 {
		return nil, therrors.New(therrors.CodeInvalidArgument, "ticket user or seat is required")
	}
	if l.remain[cmd.TicketCategoryID] < count {
		return nil, therrors.New(therrors.CodeInventoryNotEnough, "ticket inventory is not enough")
	}
	for _, seatID := range cmd.SeatIDs {
		if l.seatMap[seatID] == program.SeatLocked || l.seatMap[seatID] == program.SeatSold {
			return nil, therrors.New(therrors.CodeSeatUnavailable, "seat is unavailable")
		}
	}
	l.remain[cmd.TicketCategoryID] -= count
	seats := make([]program.Seat, 0, len(cmd.SeatIDs))
	for _, seatID := range cmd.SeatIDs {
		l.seatMap[seatID] = program.SeatLocked
		seats = append(seats, program.Seat{
			ID:               seatID,
			ProgramID:        cmd.ProgramID,
			TicketCategoryID: cmd.TicketCategoryID,
			Status:           program.SeatLocked,
		})
	}
	return seats, nil
}

func (l *InventoryLocker) RollbackSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	count := int64(len(cmd.TicketUserIDs))
	if len(cmd.SeatIDs) > 0 {
		count = int64(len(cmd.SeatIDs))
	}
	l.remain[cmd.TicketCategoryID] += count
	for _, seatID := range cmd.SeatIDs {
		if l.seatMap[seatID] == program.SeatLocked {
			delete(l.seatMap, seatID)
		}
	}
	return nil
}
