package memory

import (
	"context"
	"sync"
	"time"

	"tickethub/app/order-service/internal/domain/order"
	therrors "tickethub/pkg/errors"
)

type DiscardRepository struct {
	mu      sync.RWMutex
	nextID  int64
	records map[int64]order.DiscardOrder
}

func NewDiscardRepository() *DiscardRepository {
	return &DiscardRepository{nextID: 1, records: make(map[int64]order.DiscardOrder)}
}

func (r *DiscardRepository) Save(ctx context.Context, item order.DiscardOrder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == 0 {
		item.ID = r.nextID
		r.nextID++
	}
	if item.Status == "" {
		item.Status = "PENDING"
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	r.records[item.ID] = item
	return nil
}

func (r *DiscardRepository) ListPending(ctx context.Context, programID int64, limit int) ([]order.DiscardOrder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	result := make([]order.DiscardOrder, 0, limit)
	for _, item := range r.records {
		if item.Status != "PENDING" {
			continue
		}
		if programID > 0 && item.ProgramID != programID {
			continue
		}
		result = append(result, item)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (r *DiscardRepository) FindPendingByID(ctx context.Context, id int64) (order.DiscardOrder, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.records[id]
	if !ok || item.Status != "PENDING" {
		return order.DiscardOrder{}, therrors.New(therrors.CodeNotFound, "pending discard order not found")
	}
	return item, nil
}

func (r *DiscardRepository) MarkRetried(ctx context.Context, id int64, retriedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.records[id]
	if !ok || item.Status != "PENDING" {
		return therrors.New(therrors.CodeNotFound, "pending discard order not found")
	}
	item.Status = "RETRIED"
	item.RetriedAt = &retriedAt
	r.records[id] = item
	return nil
}
