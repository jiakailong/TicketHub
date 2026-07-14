package memory

import (
	"context"
	"sync"

	"tickethub/app/pay-service/internal/domain/pay"
	therrors "tickethub/pkg/errors"
)

type PaymentRepository struct {
	mu       sync.RWMutex
	payments map[int64]pay.Payment
}

func (r *PaymentRepository) Find(ctx context.Context, orderNumber int64) (pay.Payment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	payment, ok := r.payments[orderNumber]
	if !ok {
		return pay.Payment{}, therrors.New(therrors.CodeNotFound, "payment not found")
	}
	return payment, nil
}

func NewPaymentRepository() *PaymentRepository {
	return &PaymentRepository{payments: make(map[int64]pay.Payment)}
}

func (r *PaymentRepository) Save(ctx context.Context, payment pay.Payment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payments[payment.OrderNumber] = payment
	return nil
}
