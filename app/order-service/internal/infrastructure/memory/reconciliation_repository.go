package memory

import (
	"context"

	"tickethub/app/order-service/internal/application"
)

type ReconciliationRepository struct{}

func NewReconciliationRepository() ReconciliationRepository {
	return ReconciliationRepository{}
}

func (r ReconciliationRepository) ReconcileProgram(ctx context.Context, programID int64) (application.ReconciliationResult, error) {
	return application.ReconciliationResult{ProgramID: programID}, nil
}
