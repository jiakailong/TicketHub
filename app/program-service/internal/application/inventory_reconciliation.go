package application

import (
	"context"
	"fmt"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/observability"
)

type InventoryCatalog interface {
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	UpdateTicketCategoryRemain(ctx context.Context, ticketCategoryID int64, remain int64) error
}

type InventoryStateStore interface {
	GetRemain(ctx context.Context, programID int64, ticketCategoryID int64) (remain int64, exists bool, err error)
	SetRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) error
}

type ActiveReservationChecker interface {
	HasActiveReservations(ctx context.Context, programID int64) (bool, error)
}

type ReconciliationRecordWriter interface {
	Save(ctx context.Context, record program.ReconciliationRecord) error
}

type ReconciliationIDGenerator interface {
	NextID() (int64, error)
}

type InventoryReconciliationResult struct {
	MismatchCount int64
	RepairedCount int64
	Differences   []program.InventoryDifference
}

type InventoryReconciliationService struct {
	catalog InventoryCatalog
	store   InventoryStateStore
	records ReconciliationRecordWriter
	ids     ReconciliationIDGenerator
	nowFunc func() time.Time
}

func NewInventoryReconciliationService(catalog InventoryCatalog, store InventoryStateStore, records ReconciliationRecordWriter, ids ReconciliationIDGenerator) InventoryReconciliationService {
	return InventoryReconciliationService{catalog: catalog, store: store, records: records, ids: ids, nowFunc: time.Now}
}

func (s InventoryReconciliationService) Reconcile(ctx context.Context, programID int64, usages []program.InventoryUsage, repair bool) (InventoryReconciliationResult, error) {
	if programID <= 0 {
		return InventoryReconciliationResult{}, therrors.New(therrors.CodeInvalidArgument, "program_id is required")
	}
	if repair {
		if checker, ok := s.store.(ActiveReservationChecker); ok {
			active, err := checker.HasActiveReservations(ctx, programID)
			if err != nil {
				return InventoryReconciliationResult{}, err
			}
			if active {
				return InventoryReconciliationResult{}, therrors.New(therrors.CodeConflict, "inventory repair is blocked while active reservations exist")
			}
		}
	}
	categories, err := s.catalog.ListTicketCategories(ctx, programID)
	if err != nil {
		return InventoryReconciliationResult{}, err
	}
	usageByCategory := make(map[int64]int64, len(usages))
	for _, usage := range usages {
		if usage.TicketCategoryID <= 0 || usage.OccupiedCount < 0 {
			return InventoryReconciliationResult{}, therrors.New(therrors.CodeInvalidArgument, "inventory usage is invalid")
		}
		usageByCategory[usage.TicketCategoryID] += usage.OccupiedCount
	}

	result := InventoryReconciliationResult{Differences: make([]program.InventoryDifference, 0)}
	known := make(map[int64]struct{}, len(categories))
	for _, category := range categories {
		known[category.ID] = struct{}{}
		difference, mismatch, compareErr := s.compareCategory(ctx, programID, category, usageByCategory[category.ID], repair)
		if !mismatch {
			if compareErr != nil {
				return result, compareErr
			}
			continue
		}
		result.MismatchCount++
		if difference.Repaired {
			result.RepairedCount++
		}
		result.Differences = append(result.Differences, difference)
		if err := s.saveRecord(ctx, programID, difference); err != nil {
			return result, err
		}
		if compareErr != nil {
			return result, compareErr
		}
	}
	for categoryID, occupied := range usageByCategory {
		if _, ok := known[categoryID]; ok {
			continue
		}
		difference := program.InventoryDifference{
			TicketCategoryID: categoryID,
			OccupiedCount:    occupied,
			ExpectedRemain:   -1,
			RedisRemain:      -1,
			Reason:           "ORDER_CATEGORY_MISSING",
		}
		result.MismatchCount++
		result.Differences = append(result.Differences, difference)
		if err := s.saveRecord(ctx, programID, difference); err != nil {
			return result, err
		}
	}
	if result.MismatchCount > 0 {
		observability.AddCounter("ticket_hub_inventory_reconciliation_mismatch_total", nil, float64(result.MismatchCount))
	}
	if result.RepairedCount > 0 {
		observability.AddCounter("ticket_hub_inventory_reconciliation_repaired_total", nil, float64(result.RepairedCount))
	}
	return result, nil
}

func (s InventoryReconciliationService) compareCategory(ctx context.Context, programID int64, category program.TicketCategory, occupied int64, repair bool) (program.InventoryDifference, bool, error) {
	expected := category.Total - occupied
	redisRemain, redisExists, err := s.store.GetRemain(ctx, programID, category.ID)
	if err != nil {
		return program.InventoryDifference{}, false, err
	}
	difference := program.InventoryDifference{
		TicketCategoryID: category.ID,
		Total:            category.Total,
		OccupiedCount:    occupied,
		ExpectedRemain:   expected,
		MySQLRemain:      category.Remain,
		RedisRemain:      redisRemain,
		RedisExists:      redisExists,
	}
	if expected < 0 {
		difference.Reason = "OCCUPIED_EXCEEDS_TOTAL"
		return difference, true, nil
	}
	if redisExists && redisRemain == expected && category.Remain == expected {
		return difference, false, nil
	}
	difference.Reason = inventoryMismatchReason(category.Remain, redisRemain, redisExists, expected)
	if !repair {
		return difference, true, nil
	}
	if err := s.catalog.UpdateTicketCategoryRemain(ctx, category.ID, expected); err != nil {
		return difference, true, err
	}
	if err := s.store.SetRemain(ctx, programID, category.ID, expected); err != nil {
		return difference, true, err
	}
	difference.Repaired = true
	return difference, true, nil
}

func (s InventoryReconciliationService) saveRecord(ctx context.Context, programID int64, difference program.InventoryDifference) error {
	if s.records == nil || s.ids == nil {
		return nil
	}
	id, err := s.ids.NextID()
	if err != nil {
		return err
	}
	status := "PENDING"
	if difference.Repaired {
		status = "REPAIRED"
	}
	return s.records.Save(ctx, program.ReconciliationRecord{
		ID:               id,
		ProgramID:        programID,
		TicketCategoryID: difference.TicketCategoryID,
		Difference:       difference,
		HandleStatus:     status,
		CreatedAt:        s.nowFunc(),
	})
}

func inventoryMismatchReason(mysqlRemain int64, redisRemain int64, redisExists bool, expected int64) string {
	if !redisExists {
		return "REDIS_INVENTORY_MISSING"
	}
	if mysqlRemain != expected && redisRemain != expected {
		return "MYSQL_AND_REDIS_MISMATCH"
	}
	if mysqlRemain != expected {
		return "MYSQL_REMAIN_MISMATCH"
	}
	if redisRemain != expected {
		return "REDIS_REMAIN_MISMATCH"
	}
	return fmt.Sprintf("UNKNOWN_MISMATCH_%d", expected)
}
