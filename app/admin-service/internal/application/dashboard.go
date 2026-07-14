package application

import "context"

type DashboardStats struct {
	DiscardOrderCount           int64
	ReconciliationMismatchCount int64
}

type DashboardQuery interface {
	Dashboard(ctx context.Context) (DashboardStats, error)
}
