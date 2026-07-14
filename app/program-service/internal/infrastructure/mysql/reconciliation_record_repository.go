package mysql

import (
	"context"
	"database/sql"
	"encoding/json"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

type ReconciliationRecordRepository struct {
	db *sql.DB
}

func NewReconciliationRecordRepository(db *sql.DB) ReconciliationRecordRepository {
	return ReconciliationRecordRepository{db: db}
}

func (r ReconciliationRecordRepository) Save(ctx context.Context, record program.ReconciliationRecord) error {
	payload, err := json.Marshal(record.Difference)
	if err != nil {
		return therrors.Wrap(therrors.CodeInternal, "encode inventory reconciliation record failed", err)
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO program_records (id, program_id, identifier_id, record_type, payload, handle_status, created_at)
VALUES (?, ?, ?, 'INVENTORY_RECONCILIATION', ?, ?, ?)`,
		record.ID,
		record.ProgramID,
		record.TicketCategoryID,
		payload,
		record.HandleStatus,
		record.CreatedAt,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save inventory reconciliation record failed", err)
	}
	return nil
}
