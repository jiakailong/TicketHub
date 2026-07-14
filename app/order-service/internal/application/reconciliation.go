package application

import (
	"context"

	therrors "tickethub/pkg/errors"
	"tickethub/pkg/observability"
)

type ReconciliationResult struct {
	ProgramID              int64                 `json:"program_id"`
	MismatchCount          int64                 `json:"mismatch_count"`
	RecordMismatchCount    int64                 `json:"record_mismatch_count"`
	InventoryMismatchCount int64                 `json:"inventory_mismatch_count"`
	RepairedInventoryCount int64                 `json:"repaired_inventory_count"`
	MatchedCount           int64                 `json:"matched_count"`
	ProcessedCount         int64                 `json:"processed_count"`
	InventoryDifferences   []InventoryDifference `json:"inventory_differences"`
}

type InventoryUsage struct {
	TicketCategoryID int64
	OccupiedCount    int64
}

type InventoryDifference struct {
	TicketCategoryID int64  `json:"ticket_category_id"`
	Total            int64  `json:"total"`
	OccupiedCount    int64  `json:"occupied_count"`
	ExpectedRemain   int64  `json:"expected_remain"`
	MySQLRemain      int64  `json:"mysql_remain"`
	RedisRemain      int64  `json:"redis_remain"`
	RedisExists      bool   `json:"redis_exists"`
	Repaired         bool   `json:"repaired"`
	Reason           string `json:"reason"`
}

type ReconciliationRepository interface {
	ReconcileProgram(ctx context.Context, programID int64) (ReconciliationResult, error)
}

type InventoryUsageRepository interface {
	ListInventoryUsage(ctx context.Context, programID int64) ([]InventoryUsage, error)
}

type ProgramInventoryReconciler interface {
	ReconcileInventory(ctx context.Context, programID int64, usages []InventoryUsage, repair bool) ([]InventoryDifference, int64, error)
}

type ReconciliationService struct {
	repo    ReconciliationRepository
	usage   InventoryUsageRepository
	program ProgramInventoryReconciler
}

func NewReconciliationService(repo ReconciliationRepository) ReconciliationService {
	return ReconciliationService{repo: repo}
}

func (s ReconciliationService) WithInventory(usage InventoryUsageRepository, program ProgramInventoryReconciler) ReconciliationService {
	s.usage = usage
	s.program = program
	return s
}

func (s ReconciliationService) ReconcileProgram(ctx context.Context, programID int64, repairInventory ...bool) (ReconciliationResult, error) {
	repair := len(repairInventory) > 0 && repairInventory[0]
	if repair && programID <= 0 {
		return ReconciliationResult{}, therrors.New(therrors.CodeInvalidArgument, "program_id is required when repairing inventory")
	}
	if s.repo == nil {
		return ReconciliationResult{}, therrors.New(therrors.CodeInfrastructure, "reconciliation repository is not configured")
	}
	result, err := s.repo.ReconcileProgram(ctx, programID)
	if err != nil {
		return ReconciliationResult{}, err
	}
	result.RecordMismatchCount = result.MismatchCount
	if programID > 0 && s.usage != nil && s.program != nil {
		usages, err := s.usage.ListInventoryUsage(ctx, programID)
		if err != nil {
			return ReconciliationResult{}, err
		}
		differences, repaired, err := s.program.ReconcileInventory(ctx, programID, usages, repair)
		if err != nil {
			return ReconciliationResult{}, err
		}
		result.InventoryDifferences = differences
		result.InventoryMismatchCount = int64(len(differences))
		result.RepairedInventoryCount = repaired
		result.MismatchCount = result.RecordMismatchCount + result.InventoryMismatchCount
	}
	if result.MismatchCount > 0 {
		observability.AddCounter("ticket_hub_reconciliation_mismatch_total", nil, float64(result.MismatchCount))
	}
	return result, nil
}
