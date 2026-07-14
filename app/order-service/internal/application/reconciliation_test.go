package application

import (
	"context"
	"testing"
)

func TestReconciliationServiceDelegatesToRepository(t *testing.T) {
	repo := &fakeReconciliationRepository{}
	service := NewReconciliationService(repo)

	result, err := service.ReconcileProgram(context.Background(), 10001)
	if err != nil {
		t.Fatal(err)
	}
	if repo.programID != 10001 {
		t.Fatalf("program id = %d", repo.programID)
	}
	if result.MismatchCount != 2 || result.MatchedCount != 8 {
		t.Fatalf("result = %+v", result)
	}
}

func TestReconciliationServiceCombinesRecordAndInventoryDifferences(t *testing.T) {
	repo := &fakeReconciliationRepository{}
	usage := &fakeInventoryUsageRepository{usages: []InventoryUsage{{TicketCategoryID: 1, OccupiedCount: 10}}}
	program := &fakeProgramInventoryReconciler{
		differences: []InventoryDifference{{TicketCategoryID: 1, ExpectedRemain: 90, MySQLRemain: 95, RedisRemain: 95}},
		repaired:    1,
	}
	service := NewReconciliationService(repo).WithInventory(usage, program)

	result, err := service.ReconcileProgram(context.Background(), 10001, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.RecordMismatchCount != 2 || result.InventoryMismatchCount != 1 || result.MismatchCount != 3 || result.RepairedInventoryCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if usage.programID != 10001 || program.programID != 10001 || !program.repair || len(program.usages) != 1 {
		t.Fatalf("inventory call: usage_program=%d program=%d repair=%t usages=%+v", usage.programID, program.programID, program.repair, program.usages)
	}
}

type fakeReconciliationRepository struct {
	programID int64
}

func (r *fakeReconciliationRepository) ReconcileProgram(ctx context.Context, programID int64) (ReconciliationResult, error) {
	r.programID = programID
	return ReconciliationResult{ProgramID: programID, MismatchCount: 2, MatchedCount: 8, ProcessedCount: 10}, nil
}

type fakeInventoryUsageRepository struct {
	programID int64
	usages    []InventoryUsage
}

func (r *fakeInventoryUsageRepository) ListInventoryUsage(ctx context.Context, programID int64) ([]InventoryUsage, error) {
	r.programID = programID
	return append([]InventoryUsage(nil), r.usages...), nil
}

type fakeProgramInventoryReconciler struct {
	programID   int64
	usages      []InventoryUsage
	repair      bool
	differences []InventoryDifference
	repaired    int64
}

func (r *fakeProgramInventoryReconciler) ReconcileInventory(ctx context.Context, programID int64, usages []InventoryUsage, repair bool) ([]InventoryDifference, int64, error) {
	r.programID = programID
	r.usages = append([]InventoryUsage(nil), usages...)
	r.repair = repair
	return append([]InventoryDifference(nil), r.differences...), r.repaired, nil
}
