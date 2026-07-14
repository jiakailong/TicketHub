package order

import "context"

type Repository interface {
	Save(ctx context.Context, order Order) error
	FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (Order, error)
	Update(ctx context.Context, order Order) error
}

type DiscardRepository interface {
	Save(ctx context.Context, discard DiscardOrder) error
}
