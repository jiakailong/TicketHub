package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/order-service/internal/application"
	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

type SystemOrderFinder interface {
	FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error)
}

type ShardedReconciliationRepository struct {
	controlDB *sql.DB
	orders    SystemOrderFinder
}

func NewShardedReconciliationRepository(controlDB *sql.DB, orders SystemOrderFinder) ShardedReconciliationRepository {
	return ShardedReconciliationRepository{controlDB: controlDB, orders: orders}
}

func (r ShardedReconciliationRepository) ReconcileProgram(ctx context.Context, programID int64) (application.ReconciliationResult, error) {
	query := `SELECT order_number FROM order_records`
	args := []any{}
	if programID > 0 {
		query += ` WHERE program_id = ?`
		args = append(args, programID)
	}
	rows, err := r.controlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "list order records for sharded reconciliation failed", err)
	}
	orderNumbers := make([]int64, 0)
	for rows.Next() {
		var orderNumber int64
		if err := rows.Scan(&orderNumber); err != nil {
			rows.Close()
			return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "scan order record for sharded reconciliation failed", err)
		}
		orderNumbers = append(orderNumbers, orderNumber)
	}
	if err := rows.Close(); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "close sharded reconciliation rows failed", err)
	}
	if err := rows.Err(); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "iterate sharded reconciliation rows failed", err)
	}

	tx, err := r.controlDB.BeginTx(ctx, nil)
	if err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "begin sharded reconciliation transaction failed", err)
	}
	defer tx.Rollback()
	for _, orderNumber := range orderNumbers {
		status := "MATCHED"
		if _, err := r.orders.FindByOrderNumberSystem(ctx, orderNumber); err != nil {
			if !therrors.IsCode(err, therrors.CodeNotFound) {
				return application.ReconciliationResult{}, err
			}
			status = "ORDER_MISSING"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE order_records SET reconciliation_status = ? WHERE order_number = ?`, status, orderNumber); err != nil {
			return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "update sharded reconciliation status failed", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "commit sharded reconciliation failed", err)
	}

	stats := application.ReconciliationResult{ProgramID: programID, ProcessedCount: int64(len(orderNumbers))}
	if err := r.controlDB.QueryRowContext(ctx, countSQL("MATCHED", programID), countArgs("MATCHED", programID)...).Scan(&stats.MatchedCount); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "count matched sharded order records failed", err)
	}
	if err := r.controlDB.QueryRowContext(ctx, mismatchSQL(programID), mismatchArgs(programID)...).Scan(&stats.MismatchCount); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "count mismatched sharded order records failed", err)
	}
	return stats, nil
}
