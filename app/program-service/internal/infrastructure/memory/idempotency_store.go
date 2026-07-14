package memory

import (
	"context"
	"sync"
	"time"

	"tickethub/app/program-service/internal/application"
	therrors "tickethub/pkg/errors"
)

type idempotencyEntry struct {
	fingerprint string
	state       application.IdempotencyState
	orderNumber int64
	expiresAt   time.Time
}

type IdempotencyStore struct {
	mu      sync.Mutex
	entries map[string]idempotencyEntry
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{entries: make(map[string]idempotencyEntry)}
}

func (s *IdempotencyStore) Begin(_ context.Context, key string, fingerprint string, ttl time.Duration) (application.IdempotencyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[key]; ok && entry.expiresAt.After(time.Now()) {
		if entry.fingerprint != fingerprint {
			return application.IdempotencyResult{}, therrors.New(therrors.CodeDuplicateSubmission, "idempotency key was reused with a different request")
		}
		return application.IdempotencyResult{State: entry.state, OrderNumber: entry.orderNumber}, nil
	}
	s.entries[key] = idempotencyEntry{fingerprint: fingerprint, state: application.IdempotencyProcessing, expiresAt: time.Now().Add(ttl)}
	return application.IdempotencyResult{State: application.IdempotencyAcquired}, nil
}

func (s *IdempotencyStore) Complete(_ context.Context, key string, fingerprint string, orderNumber int64, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok || entry.fingerprint != fingerprint {
		return therrors.New(therrors.CodeConflict, "order idempotency ownership was lost")
	}
	entry.state = application.IdempotencyCompleted
	entry.orderNumber = orderNumber
	entry.expiresAt = time.Now().Add(ttl)
	s.entries[key] = entry
	return nil
}

func (s *IdempotencyStore) Abort(_ context.Context, key string, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[key]; ok && entry.fingerprint == fingerprint && entry.state != application.IdempotencyCompleted {
		delete(s.entries, key)
	}
	return nil
}
