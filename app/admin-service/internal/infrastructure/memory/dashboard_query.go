package memory

import (
	"context"

	"tickethub/app/admin-service/internal/application"
)

type DashboardQuery struct {
	Stats application.DashboardStats
}

func NewDashboardQuery() DashboardQuery {
	return DashboardQuery{Stats: application.DashboardStats{}}
}

func (q DashboardQuery) Dashboard(ctx context.Context) (application.DashboardStats, error) {
	return q.Stats, nil
}
