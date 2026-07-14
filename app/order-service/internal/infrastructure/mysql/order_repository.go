package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	orderapp "tickethub/app/order-service/internal/application"
	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) OrderRepository {
	return OrderRepository{db: db}
}

func (r OrderRepository) Save(ctx context.Context, item order.Order) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO orders (order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  order_number = VALUES(order_number)`,
		item.OrderNumber,
		item.ProgramID,
		item.UserID,
		nullInt64(item.TicketCategoryID),
		jsonInt64Slice(item.SeatIDs),
		jsonInt64Slice(item.TicketUserIDs),
		item.AmountCent,
		string(item.Status),
		item.CreatedAt,
		nullTime(item.PaidAt),
		nullTime(item.CanceledAt),
		nullTime(item.RefundedAt),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save order failed", err)
	}
	return nil
}

func (r OrderRepository) FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (order.Order, error) {
	return scanOrder(r.db.QueryRowContext(ctx, `
SELECT order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at
FROM orders
WHERE order_number = ? AND user_id = ?`, orderNumber, userID))
}

func (r OrderRepository) FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error) {
	return scanOrder(r.db.QueryRowContext(ctx, `
SELECT order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at
FROM orders
WHERE order_number = ?`, orderNumber))
}

func (r OrderRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]order.Order, error) {
	return r.ListByUserIDPage(ctx, userID, "", orderapp.OrderListCursor{}, limit)
}

func (r OrderRepository) ListByUserIDPage(ctx context.Context, userID int64, status order.Status, before orderapp.OrderListCursor, limit int) ([]order.Order, error) {
	query := `
SELECT order_number, program_id, user_id, ticket_category_id, seat_ids, ticket_user_ids, amount_cent, status, created_at, paid_at, canceled_at, refunded_at
FROM orders
WHERE user_id = ?`
	args := []any{userID}
	if status != "" {
		query += " AND status = ?"
		args = append(args, string(status))
	}
	if !before.CreatedAt.IsZero() {
		query += " AND (created_at < ? OR (created_at = ? AND order_number < ?))"
		args = append(args, before.CreatedAt, before.CreatedAt, before.OrderNumber)
	}
	query += " ORDER BY created_at DESC, order_number DESC LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "list orders failed", err)
	}
	defer rows.Close()
	result := make([]order.Order, 0)
	for rows.Next() {
		item, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate orders failed", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrder(row rowScanner) (order.Order, error) {
	var item order.Order
	var status string
	var ticketCategoryID sql.NullInt64
	var seatIDs sql.NullString
	var ticketUserIDs sql.NullString
	var paidAt sql.NullTime
	var canceledAt sql.NullTime
	var refundedAt sql.NullTime
	err := row.Scan(
		&item.OrderNumber,
		&item.ProgramID,
		&item.UserID,
		&ticketCategoryID,
		&seatIDs,
		&ticketUserIDs,
		&item.AmountCent,
		&status,
		&item.CreatedAt,
		&paidAt,
		&canceledAt,
		&refundedAt,
	)
	if err == sql.ErrNoRows {
		return order.Order{}, therrors.New(therrors.CodeNotFound, "order not found")
	}
	if err != nil {
		return order.Order{}, therrors.Wrap(therrors.CodeInfrastructure, "query order failed", err)
	}
	if ticketCategoryID.Valid {
		item.TicketCategoryID = ticketCategoryID.Int64
	}
	if seatIDs.Valid && seatIDs.String != "" {
		if err := json.Unmarshal([]byte(seatIDs.String), &item.SeatIDs); err != nil {
			return order.Order{}, therrors.Wrap(therrors.CodeInfrastructure, "decode order seat ids failed", err)
		}
	}
	if ticketUserIDs.Valid && ticketUserIDs.String != "" {
		if err := json.Unmarshal([]byte(ticketUserIDs.String), &item.TicketUserIDs); err != nil {
			return order.Order{}, therrors.Wrap(therrors.CodeInfrastructure, "decode order ticket user ids failed", err)
		}
	}
	item.Status = order.Status(status)
	if paidAt.Valid {
		item.PaidAt = &paidAt.Time
	}
	if canceledAt.Valid {
		item.CanceledAt = &canceledAt.Time
	}
	if refundedAt.Valid {
		item.RefundedAt = &refundedAt.Time
	}
	return item, nil
}

func (r OrderRepository) Update(ctx context.Context, item order.Order) error {
	previous, err := allowedPreviousStatus(item.Status)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE orders
	SET status = ?, paid_at = ?, canceled_at = ?, refunded_at = ?
WHERE order_number = ? AND user_id = ? AND status IN (?, ?)`,
		string(item.Status),
		nullTime(item.PaidAt),
		nullTime(item.CanceledAt),
		nullTime(item.RefundedAt),
		item.OrderNumber,
		item.UserID,
		string(previous),
		string(item.Status),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "update order failed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "read order update result failed", err)
	}
	if affected > 0 {
		return nil
	}
	var current string
	if err := r.db.QueryRowContext(ctx, `SELECT status FROM orders WHERE order_number = ? AND user_id = ?`, item.OrderNumber, item.UserID).Scan(&current); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "verify order update conflict failed", err)
	}
	if order.Status(current) != item.Status {
		return therrors.New(therrors.CodeOrderStateConflict, "order status changed concurrently")
	}
	return nil
}

func allowedPreviousStatus(target order.Status) (order.Status, error) {
	switch target {
	case order.StatusPaid, order.StatusCancel:
		return order.StatusNoPay, nil
	case order.StatusRefund:
		return order.StatusCancel, nil
	default:
		return "", therrors.New(therrors.CodeOrderStateConflict, "unsupported order status transition")
	}
}

func (r OrderRepository) ListInventoryUsage(ctx context.Context, programID int64) ([]orderapp.InventoryUsage, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT ticket_category_id,
       SUM(CASE
         WHEN COALESCE(JSON_LENGTH(seat_ids), 0) > 0 THEN JSON_LENGTH(seat_ids)
         ELSE COALESCE(JSON_LENGTH(ticket_user_ids), 0)
       END) AS occupied_count
FROM orders
WHERE program_id = ?
  AND status IN ('NO_PAY', 'PAY')
  AND ticket_category_id IS NOT NULL
GROUP BY ticket_category_id`, programID)
	if err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "list order inventory usage failed", err)
	}
	defer rows.Close()
	var result []orderapp.InventoryUsage
	for rows.Next() {
		var item orderapp.InventoryUsage
		if err := rows.Scan(&item.TicketCategoryID, &item.OccupiedCount); err != nil {
			return nil, therrors.Wrap(therrors.CodeInfrastructure, "scan order inventory usage failed", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, therrors.Wrap(therrors.CodeInfrastructure, "iterate order inventory usage failed", err)
	}
	return result, nil
}

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func nullInt64(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: value > 0}
}

func jsonInt64Slice(values []int64) sql.NullString {
	if len(values) == 0 {
		return sql.NullString{}
	}
	data, _ := json.Marshal(values)
	return sql.NullString{String: string(data), Valid: true}
}
