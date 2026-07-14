package mysql

import (
	"context"
	"database/sql"
	"time"

	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) OutboxRepository {
	return OutboxRepository{db: db}
}

func (r OutboxRepository) Save(ctx context.Context, event mq.Event) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO program_outbox
  (event_id, topic, event_key, trace_id, schema_version, occurred_at, payload, status, available_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE event_id = VALUES(event_id)`,
		event.Header.EventID,
		event.Topic,
		event.Key,
		sql.NullString{String: event.Header.TraceID, Valid: event.Header.TraceID != ""},
		event.Header.SchemaVersion,
		event.Header.OccurredAt,
		event.Payload,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save program outbox event failed", err)
	}
	return nil
}

func (r OutboxRepository) Claim(ctx context.Context, limit int, lease time.Duration) ([]mq.Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "begin program outbox claim failed", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
SELECT event_id, topic, event_key, trace_id, schema_version, occurred_at, payload
FROM program_outbox
WHERE available_at <= CURRENT_TIMESTAMP(3)
  AND (status = 'PENDING' OR (status = 'PROCESSING' AND lease_until < CURRENT_TIMESTAMP(3)))
ORDER BY id
LIMIT ?
FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "claim program outbox events failed", err)
	}
	events := make([]mq.Event, 0, limit)
	for rows.Next() {
		var event mq.Event
		var traceID sql.NullString
		if err := rows.Scan(
			&event.Header.EventID,
			&event.Topic,
			&event.Key,
			&traceID,
			&event.Header.SchemaVersion,
			&event.Header.OccurredAt,
			&event.Payload,
		); err != nil {
			rows.Close()
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan program outbox event failed", err)
		}
		event.Header.TraceID = traceID.String
		events = append(events, event)
	}
	if err := rows.Close(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "close program outbox rows failed", err)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate program outbox events failed", err)
	}
	leaseUntil := time.Now().Add(lease)
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `
UPDATE program_outbox
SET status = 'PROCESSING', lease_until = ?, attempts = attempts + 1
WHERE event_id = ?`, leaseUntil, event.Header.EventID); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "lease program outbox event failed", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "commit program outbox claim failed", err)
	}
	return events, nil
}

func (r OutboxRepository) MarkPublished(ctx context.Context, eventID string, publishedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE program_outbox
SET status = 'PUBLISHED', published_at = ?, lease_until = NULL, last_error = NULL
WHERE event_id = ? AND status = 'PROCESSING'`, publishedAt, eventID)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "mark program outbox event published failed", err)
	}
	return nil
}

func (r OutboxRepository) MarkRetry(ctx context.Context, eventID string, availableAt time.Time, detail string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE program_outbox
SET status = 'PENDING', available_at = ?, lease_until = NULL, last_error = ?
WHERE event_id = ? AND status = 'PROCESSING'`, availableAt, detail, eventID)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "reschedule program outbox event failed", err)
	}
	return nil
}
