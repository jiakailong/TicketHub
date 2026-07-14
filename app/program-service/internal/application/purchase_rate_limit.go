package application

import (
	"context"

	"tickethub/app/program-service/internal/domain/program"
)

type PurchaseRateLimiter interface {
	Allow(ctx context.Context, cmd program.CreateOrderCommand) (bool, error)
}
