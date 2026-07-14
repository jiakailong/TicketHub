package application

import (
	"context"
	"errors"
	"testing"

	"tickethub/app/program-service/internal/domain/program"
)

func TestInventoryReconciliationDetectsMismatchWithoutRepair(t *testing.T) {
	catalog := &fakeInventoryCatalog{categories: []program.TicketCategory{{ID: 1, ProgramID: 10001, Total: 100, Remain: 95}}}
	store := &fakeInventoryStateStore{remain: map[int64]int64{1: 95}}
	records := &fakeReconciliationRecordWriter{}
	service := NewInventoryReconciliationService(catalog, store, records, &fakeReconciliationIDs{})

	result, err := service.Reconcile(context.Background(), 10001, []program.InventoryUsage{{TicketCategoryID: 1, OccupiedCount: 10}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.MismatchCount != 1 || result.RepairedCount != 0 || len(result.Differences) != 1 {
		t.Fatalf("result = %+v", result)
	}
	difference := result.Differences[0]
	if difference.ExpectedRemain != 90 || difference.Reason != "MYSQL_AND_REDIS_MISMATCH" || difference.Repaired {
		t.Fatalf("difference = %+v", difference)
	}
	if catalog.updateCalls != 0 || store.setCalls != 0 {
		t.Fatalf("unexpected repair calls: mysql=%d redis=%d", catalog.updateCalls, store.setCalls)
	}
	if len(records.records) != 1 || records.records[0].HandleStatus != "PENDING" {
		t.Fatalf("records = %+v", records.records)
	}
}

func TestInventoryReconciliationRepairsMySQLAndRedis(t *testing.T) {
	catalog := &fakeInventoryCatalog{categories: []program.TicketCategory{{ID: 1, ProgramID: 10001, Total: 100, Remain: 95}}}
	store := &fakeInventoryStateStore{remain: map[int64]int64{1: 95}}
	records := &fakeReconciliationRecordWriter{}
	service := NewInventoryReconciliationService(catalog, store, records, &fakeReconciliationIDs{})

	result, err := service.Reconcile(context.Background(), 10001, []program.InventoryUsage{{TicketCategoryID: 1, OccupiedCount: 10}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.MismatchCount != 1 || result.RepairedCount != 1 || !result.Differences[0].Repaired {
		t.Fatalf("result = %+v", result)
	}
	if catalog.categories[0].Remain != 90 || store.remain[1] != 90 {
		t.Fatalf("remain after repair: mysql=%d redis=%d", catalog.categories[0].Remain, store.remain[1])
	}
	if len(records.records) != 1 || records.records[0].HandleStatus != "REPAIRED" {
		t.Fatalf("records = %+v", records.records)
	}
}

func TestInventoryReconciliationDoesNotRepairInvalidOccupancy(t *testing.T) {
	catalog := &fakeInventoryCatalog{categories: []program.TicketCategory{{ID: 1, ProgramID: 10001, Total: 10, Remain: 0}}}
	store := &fakeInventoryStateStore{remain: map[int64]int64{1: 0}}
	records := &fakeReconciliationRecordWriter{}
	service := NewInventoryReconciliationService(catalog, store, records, &fakeReconciliationIDs{})

	result, err := service.Reconcile(context.Background(), 10001, []program.InventoryUsage{
		{TicketCategoryID: 1, OccupiedCount: 15},
		{TicketCategoryID: 99, OccupiedCount: 1},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.MismatchCount != 2 || result.RepairedCount != 0 {
		t.Fatalf("result = %+v", result)
	}
	if result.Differences[0].Reason != "OCCUPIED_EXCEEDS_TOTAL" || result.Differences[1].Reason != "ORDER_CATEGORY_MISSING" {
		t.Fatalf("differences = %+v", result.Differences)
	}
	if catalog.updateCalls != 0 || store.setCalls != 0 {
		t.Fatal("invalid occupancy must not be repaired")
	}
}

func TestInventoryReconciliationIgnoresConsistentInventory(t *testing.T) {
	catalog := &fakeInventoryCatalog{categories: []program.TicketCategory{{ID: 1, ProgramID: 10001, Total: 100, Remain: 90}}}
	store := &fakeInventoryStateStore{remain: map[int64]int64{1: 90}}
	records := &fakeReconciliationRecordWriter{}
	service := NewInventoryReconciliationService(catalog, store, records, &fakeReconciliationIDs{})

	result, err := service.Reconcile(context.Background(), 10001, []program.InventoryUsage{{TicketCategoryID: 1, OccupiedCount: 10}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.MismatchCount != 0 || len(result.Differences) != 0 || len(records.records) != 0 {
		t.Fatalf("result = %+v records = %+v", result, records.records)
	}
}

func TestInventoryReconciliationRecordsPartialRepairFailure(t *testing.T) {
	catalog := &fakeInventoryCatalog{categories: []program.TicketCategory{{ID: 1, ProgramID: 10001, Total: 100, Remain: 95}}}
	store := &fakeInventoryStateStore{remain: map[int64]int64{1: 95}, setErr: errors.New("redis unavailable")}
	records := &fakeReconciliationRecordWriter{}
	service := NewInventoryReconciliationService(catalog, store, records, &fakeReconciliationIDs{})

	result, err := service.Reconcile(context.Background(), 10001, []program.InventoryUsage{{TicketCategoryID: 1, OccupiedCount: 10}}, true)
	if err == nil {
		t.Fatal("expected redis repair error")
	}
	if result.MismatchCount != 1 || result.RepairedCount != 0 {
		t.Fatalf("result = %+v", result)
	}
	if catalog.categories[0].Remain != 90 || store.remain[1] != 95 {
		t.Fatalf("partial repair state: mysql=%d redis=%d", catalog.categories[0].Remain, store.remain[1])
	}
	if len(records.records) != 1 || records.records[0].HandleStatus != "PENDING" {
		t.Fatalf("records = %+v", records.records)
	}
}

type fakeInventoryCatalog struct {
	categories  []program.TicketCategory
	updateCalls int
}

func (c *fakeInventoryCatalog) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	return append([]program.TicketCategory(nil), c.categories...), nil
}

func (c *fakeInventoryCatalog) UpdateTicketCategoryRemain(ctx context.Context, ticketCategoryID int64, remain int64) error {
	c.updateCalls++
	for index := range c.categories {
		if c.categories[index].ID == ticketCategoryID {
			c.categories[index].Remain = remain
		}
	}
	return nil
}

type fakeInventoryStateStore struct {
	remain   map[int64]int64
	setCalls int
	setErr   error
}

func (s *fakeInventoryStateStore) GetRemain(ctx context.Context, programID int64, ticketCategoryID int64) (int64, bool, error) {
	remain, ok := s.remain[ticketCategoryID]
	return remain, ok, nil
}

func (s *fakeInventoryStateStore) SetRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.remain[ticketCategoryID] = remain
	return nil
}

type fakeReconciliationRecordWriter struct {
	records []program.ReconciliationRecord
}

func (w *fakeReconciliationRecordWriter) Save(ctx context.Context, record program.ReconciliationRecord) error {
	w.records = append(w.records, record)
	return nil
}

type fakeReconciliationIDs struct {
	next int64
}

func (g *fakeReconciliationIDs) NextID() (int64, error) {
	g.next++
	return g.next, nil
}
