package program

import "context"

type Repository interface {
	FindProgram(ctx context.Context, programID int64) (Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]TicketCategory, error)
	ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]Seat, error)
}
