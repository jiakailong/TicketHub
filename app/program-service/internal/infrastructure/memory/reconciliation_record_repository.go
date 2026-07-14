package memory

import (
	"context"
	"sync"

	"tickethub/app/program-service/internal/domain/program"
)

type ReconciliationRecordRepository struct {
	mu      sync.Mutex
	records []program.ReconciliationRecord
}

func NewReconciliationRecordRepository() *ReconciliationRecordRepository {
	return &ReconciliationRecordRepository{}
}

func (r *ReconciliationRecordRepository) Save(ctx context.Context, record program.ReconciliationRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
	return nil
}

func (r *ReconciliationRecordRepository) Records() []program.ReconciliationRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]program.ReconciliationRecord(nil), r.records...)
}
