package memory

import (
	"context"
	"sync"

	"tickethub/app/user-service/internal/domain/user"
	therrors "tickethub/pkg/errors"
)

type UserRepository struct {
	mu       sync.RWMutex
	byID     map[int64]user.User
	byMobile map[string]int64
}

func NewUserRepository() *UserRepository {
	return &UserRepository{
		byID:     make(map[int64]user.User),
		byMobile: make(map[string]int64),
	}
}

func (r *UserRepository) Save(ctx context.Context, item user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[item.ID] = item
	r.byMobile[item.Mobile] = item.ID
	return nil
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.byID[id]
	if !ok {
		return user.User{}, therrors.New(therrors.CodeNotFound, "user not found")
	}
	return item, nil
}

func (r *UserRepository) FindByMobile(ctx context.Context, mobile string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMobile[mobile]
	if !ok {
		return user.User{}, therrors.New(therrors.CodeNotFound, "user not found")
	}
	return r.byID[id], nil
}
