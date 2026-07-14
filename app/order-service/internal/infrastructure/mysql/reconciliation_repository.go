package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/order-service/internal/application"
	therrors "tickethub/pkg/errors"
)

type ReconciliationRepository struct {
	db *sql.DB
}

func NewReconciliationRepository(db *sql.DB) ReconciliationRepository {
	return ReconciliationRepository{db: db}
}

func (r ReconciliationRepository) ReconcileProgram(ctx context.Context, programID int64) (application.ReconciliationResult, error) {
	where := ""
	args := []any{}
	if programID > 0 {
		where = " AND rec.program_id = ?"
		args = append(args, programID)
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE order_records rec
LEFT JOIN orders ord ON ord.order_number = rec.order_number
SET rec.reconciliation_status = CASE
  WHEN ord.order_number IS NULL THEN 'ORDER_MISSING'
  ELSE 'MATCHED'
END
WHERE 1 = 1`+where, args...)
	if err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "reconcile order records failed", err)
	}
	processed, err := result.RowsAffected()
	if err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "read reconciled rows failed", err)
	}

	stats := application.ReconciliationResult{ProgramID: programID, ProcessedCount: processed}
	if err := r.db.QueryRowContext(ctx, countSQL("MATCHED", programID), countArgs("MATCHED", programID)...).Scan(&stats.MatchedCount); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "count matched order records failed", err)
	}
	if err := r.db.QueryRowContext(ctx, mismatchSQL(programID), mismatchArgs(programID)...).Scan(&stats.MismatchCount); err != nil {
		return application.ReconciliationResult{}, therrors.Wrap(therrors.CodeInfrastructure, "count mismatched order records failed", err)
	}
	return stats, nil
}

func countSQL(status string, programID int64) string {
	query := `SELECT COUNT(*) FROM order_records WHERE reconciliation_status = ?`
	if programID > 0 {
		query += ` AND program_id = ?`
	}
	return query
}

func countArgs(status string, programID int64) []any {
	args := []any{status}
	if programID > 0 {
		args = append(args, programID)
	}
	return args
}

func mismatchSQL(programID int64) string {
	query := `SELECT COUNT(*) FROM order_records WHERE reconciliation_status IS NOT NULL AND reconciliation_status <> 'MATCHED'`
	if programID > 0 {
		query += ` AND program_id = ?`
	}
	return query
}

func mismatchArgs(programID int64) []any {
	if programID <= 0 {
		return nil
	}
	return []any{programID}
}
