package user

import "context"

type Repository interface {
	Save(ctx context.Context, user User) error
	FindByID(ctx context.Context, id int64) (User, error)
	FindByMobile(ctx context.Context, mobile string) (User, error)
}

type TicketUserRepository interface {
	Save(ctx context.Context, ticketUser TicketUser) error
	ListByUserID(ctx context.Context, userID int64) ([]TicketUser, error)
}
