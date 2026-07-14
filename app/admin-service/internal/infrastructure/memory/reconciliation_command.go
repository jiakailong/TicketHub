package memory

import (
	"context"

	"tickethub/app/admin-service/internal/application"
)

type ReconciliationCommand struct{}

func NewReconciliationCommand() ReconciliationCommand {
	return ReconciliationCommand{}
}

func (c ReconciliationCommand) Run(ctx context.Context, programID int64, repairInventory bool) (application.ReconciliationResult, error) {
	return application.ReconciliationResult{ProgramID: programID}, nil
}
