package memory

import (
	"context"
	"sync"

	"tickethub/app/user-service/internal/domain/user"
)

type TicketUserRepository struct {
	mu     sync.RWMutex
	byUser map[int64][]user.TicketUser
}

func NewTicketUserRepository() *TicketUserRepository {
	return &TicketUserRepository{byUser: make(map[int64][]user.TicketUser)}
}

func (r *TicketUserRepository) Save(ctx context.Context, item user.TicketUser) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.byUser[item.UserID]
	for index, current := range items {
		if current.ID == item.ID {
			items[index] = item
			r.byUser[item.UserID] = items
			return nil
		}
	}
	r.byUser[item.UserID] = append(items, item)
	return nil
}

func (r *TicketUserRepository) ListByUserID(ctx context.Context, userID int64) ([]user.TicketUser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]user.TicketUser(nil), r.byUser[userID]...), nil
}
