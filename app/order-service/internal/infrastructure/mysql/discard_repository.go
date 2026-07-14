package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

type DiscardRepository struct {
	db *sql.DB
}

func NewDiscardRepository(db *sql.DB) DiscardRepository {
	return DiscardRepository{db: db}
}

func (r DiscardRepository) Save(ctx context.Context, item order.DiscardOrder) error {
	_, err := r.db.ExecContext(ctx, `
	INSERT INTO discard_orders (program_id, order_number, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, reason, detail, status)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING')`,
		item.ProgramID,
		item.OrderNumber,
		item.UserID,
		nullInt64(item.TicketCategoryID),
		jsonInt64Slice(item.SeatIDs),
		jsonInt64Slice(item.TicketUserIDs),
		item.AmountCent,
		item.Reason,
		item.Detail,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save discard order failed", err)
	}
	return nil
}

func (r DiscardRepository) ListPending(ctx context.Context, programID int64, limit int) ([]order.DiscardOrder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{"PENDING"}
	where := "WHERE status = ?"
	if programID > 0 {
		where += " AND program_id = ?"
		args = append(args, programID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `
	SELECT id, program_id, order_number, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, reason, detail, status, created_at, retried_at
FROM discard_orders
`+where+`
ORDER BY created_at
LIMIT ?`, args...)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "list pending discard orders failed", err)
	}
	defer rows.Close()
	return scanDiscardOrders(rows)
}

func (r DiscardRepository) FindPendingByID(ctx context.Context, id int64) (order.DiscardOrder, error) {
	row := r.db.QueryRowContext(ctx, `
	SELECT id, program_id, order_number, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, reason, detail, status, created_at, retried_at
FROM discard_orders
WHERE id = ? AND status = 'PENDING'`, id)
	item, err := scanDiscardOrder(row)
	if err == sql.ErrNoRows {
		return order.DiscardOrder{}, therrors.New(therrors.CodeNotFound, "pending discard order not found")
	}
	if err != nil {
		return order.DiscardOrder{}, therrors.Wrap(therrors.CodeInfrastructure, "query pending discard order failed", err)
	}
	return item, nil
}

func (r DiscardRepository) MarkRetried(ctx context.Context, id int64, retriedAt time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE discard_orders
SET status = 'RETRIED', retried_at = ?
WHERE id = ? AND status = 'PENDING'`,
		retriedAt,
		id,
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "mark discard order retried failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read discard order affected rows failed", err)
	}
	if affected == 0 {
		return therrors.New(therrors.CodeNotFound, "pending discard order not found")
	}
	return nil
}

type discardScanner interface {
	Scan(dest ...any) error
}

func scanDiscardOrders(rows *sql.Rows) ([]order.DiscardOrder, error) {
	var result []order.DiscardOrder
	for rows.Next() {
		item, err := scanDiscardOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate discard orders failed", err)
	}
	return result, nil
}

func scanDiscardOrder(row discardScanner) (order.DiscardOrder, error) {
	var item order.DiscardOrder
	var userID sql.NullInt64
	var ticketCategoryID sql.NullInt64
	var seatIDs sql.NullString
	var ticketUserIDs sql.NullString
	var detail sql.NullString
	var retriedAt sql.NullTime
	err := row.Scan(
		&item.ID,
		&item.ProgramID,
		&item.OrderNumber,
		&userID,
		&ticketCategoryID,
		&seatIDs,
		&ticketUserIDs,
		&item.AmountCent,
		&item.Reason,
		&detail,
		&item.Status,
		&item.CreatedAt,
		&retriedAt,
	)
	if err != nil {
		return order.DiscardOrder{}, err
	}
	if userID.Valid {
		item.UserID = userID.Int64
	}
	if ticketCategoryID.Valid {
		item.TicketCategoryID = ticketCategoryID.Int64
	}
	if seatIDs.Valid && seatIDs.String != "" {
		if err := json.Unmarshal([]byte(seatIDs.String), &item.SeatIDs); err != nil {
			return order.DiscardOrder{}, err
		}
	}
	if ticketUserIDs.Valid && ticketUserIDs.String != "" {
		if err := json.Unmarshal([]byte(ticketUserIDs.String), &item.TicketUserIDs); err != nil {
			return order.DiscardOrder{}, err
		}
	}
	if detail.Valid {
		item.Detail = detail.String
	}
	if retriedAt.Valid {
		item.RetriedAt = &retriedAt.Time
	}
	return item, nil
}
