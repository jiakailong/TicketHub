package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/customize-service/internal/domain/customize"
	therrors "tickethub/pkg/errors"
)

type MessageRecordRepository struct {
	db *sql.DB
}

func NewMessageRecordRepository(db *sql.DB) MessageRecordRepository {
	return MessageRecordRepository{db: db}
}

func (r MessageRecordRepository) Save(ctx context.Context, record customize.MessageRecord) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO message_records (id, message_id, topic, status, reason, created_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  status = VALUES(status),
  reason = VALUES(reason),
  updated_at = CURRENT_TIMESTAMP(3)`,
		record.ID,
		record.MessageID,
		record.Topic,
		string(record.Status),
		nullString(record.Reason),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save message record failed", err)
	}
	return nil
}

func (r MessageRecordRepository) MarkFailed(ctx context.Context, messageID string, topic string, reason string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE message_records
SET status = ?, reason = ?, updated_at = CURRENT_TIMESTAMP(3)
WHERE message_id = ? AND topic = ?`,
		string(customize.MessageFailed),
		reason,
		messageID,
		topic,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "mark message failed", err)
	}
	return nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
