package mysql

import (
	"context"
	"database/sql"

	"tickethub/app/pay-service/internal/domain/pay"
	therrors "tickethub/pkg/errors"
)

type PaymentRepository struct {
	db *sql.DB
}

func NewPaymentRepository(db *sql.DB) PaymentRepository {
	return PaymentRepository{db: db}
}

func (r PaymentRepository) Save(ctx context.Context, item pay.Payment) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO payments (order_number, amount_cent, channel, status, created_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  amount_cent = VALUES(amount_cent),
  channel = VALUES(channel),
  status = CASE
    WHEN status = 'REFUNDED' THEN status
    WHEN status = 'PAID' AND VALUES(status) IN ('CREATED', 'CLOSED') THEN status
    ELSE VALUES(status)
  END,
  updated_at = CURRENT_TIMESTAMP(3)`,
		item.OrderNumber,
		item.AmountCent,
		item.Channel,
		string(item.Status),
	)
	if err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "save payment failed", err)
	}
	return nil
}

func (r PaymentRepository) Find(ctx context.Context, orderNumber int64) (pay.Payment, error) {
	var item pay.Payment
	var status string
	err := r.db.QueryRowContext(ctx, `
SELECT order_number, amount_cent, channel, status
FROM payments
WHERE order_number = ?`, orderNumber).Scan(
		&item.OrderNumber,
		&item.AmountCent,
		&item.Channel,
		&status,
	)
	if err == sql.ErrNoRows {
		return pay.Payment{}, therrors.New(therrors.CodeNotFound, "payment not found")
	}
	if err != nil {
		return pay.Payment{}, therrors.Wrap(therrors.CodeInfrastructure, "query payment failed", err)
	}
	item.Status = pay.PaymentStatus(status)
	return item, nil
}
