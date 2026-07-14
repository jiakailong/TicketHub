package memory

import (
	"context"
	"sort"
	"sync"

	orderapp "tickethub/app/order-service/internal/application"
	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

type OrderRepository struct {
	mu     sync.RWMutex
	orders map[int64]order.Order
}

func (r *OrderRepository) ListInventoryUsage(ctx context.Context, programID int64) ([]orderapp.InventoryUsage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	counts := make(map[int64]int64)
	for _, item := range r.orders {
		if item.ProgramID != programID || item.TicketCategoryID <= 0 {
			continue
		}
		if item.Status != order.StatusNoPay && item.Status != order.StatusPaid {
			continue
		}
		counts[item.TicketCategoryID] += item.TicketCount()
	}
	result := make([]orderapp.InventoryUsage, 0, len(counts))
	for categoryID, count := range counts {
		result = append(result, orderapp.InventoryUsage{TicketCategoryID: categoryID, OccupiedCount: count})
	}
	return result, nil
}

func NewOrderRepository() *OrderRepository {
	return &OrderRepository{orders: make(map[int64]order.Order)}
}

func (r *OrderRepository) Save(ctx context.Context, item order.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[item.OrderNumber] = item
	return nil
}

func (r *OrderRepository) FindByOrderNumber(ctx context.Context, orderNumber int64, userID int64) (order.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.orders[orderNumber]
	if !ok || item.UserID != userID {
		return order.Order{}, therrors.New(therrors.CodeNotFound, "order not found")
	}
	return item, nil
}

func (r *OrderRepository) FindByOrderNumberSystem(ctx context.Context, orderNumber int64) (order.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.orders[orderNumber]
	if !ok {
		return order.Order{}, therrors.New(therrors.CodeNotFound, "order not found")
	}
	return item, nil
}

func (r *OrderRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]order.Order, error) {
	return r.ListByUserIDPage(ctx, userID, "", orderapp.OrderListCursor{}, limit)
}

func (r *OrderRepository) ListByUserIDPage(ctx context.Context, userID int64, status order.Status, before orderapp.OrderListCursor, limit int) ([]order.Order, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]order.Order, 0)
	for _, item := range r.orders {
		if item.UserID == userID && (status == "" || item.Status == status) && (before.CreatedAt.IsZero() || item.CreatedAt.Before(before.CreatedAt) || (item.CreatedAt.Equal(before.CreatedAt) && item.OrderNumber < before.OrderNumber)) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].OrderNumber > result[j].OrderNumber
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *OrderRepository) Update(ctx context.Context, item order.Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.orders[item.OrderNumber]; !ok {
		return therrors.New(therrors.CodeNotFound, "order not found")
	}
	r.orders[item.OrderNumber] = item
	return nil
}
