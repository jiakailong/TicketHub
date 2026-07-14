package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/admin-service/internal/application"
	therrors "tickethub/pkg/errors"
)

type DashboardQuery struct {
	db *sql.DB
}

func NewDashboardQuery(db *sql.DB) DashboardQuery {
	return DashboardQuery{db: db}
}

func (q DashboardQuery) Dashboard(ctx context.Context) (application.DashboardStats, error) {
	var stats application.DashboardStats
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tickethub_order.discard_orders`).Scan(&stats.DiscardOrderCount); err != nil {
		return application.DashboardStats{}, therrors.Wrap(therrors.CodeInfrastructure, "query discard order count failed", err)
	}
	if err := q.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tickethub_order.order_records
WHERE reconciliation_status IS NOT NULL AND reconciliation_status <> 'MATCHED'`).Scan(&stats.ReconciliationMismatchCount); err != nil {
		return application.DashboardStats{}, therrors.Wrap(therrors.CodeInfrastructure, "query reconciliation mismatch count failed", err)
	}
	return stats, nil
}
