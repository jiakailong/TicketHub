package application

import (
	"context"
	"testing"

	"tickethub/app/program-service/internal/domain/program"
)

func TestProgramIndexSyncServiceIndexesAllBatches(t *testing.T) {
	source := &fakeProgramIndexSource{items: []program.Program{
		{ID: 1, Title: "A", Status: "ON_SALE"},
		{ID: 2, Title: "B", Status: "ON_SALE"},
		{ID: 3, Title: "C", Status: "ON_SALE"},
	}}
	indexer := &fakeProgramIndexer{}
	service := NewProgramIndexSyncService(source, indexer, 2)

	result, err := service.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !indexer.ensured || result.Indexed != 3 || result.Batches != 2 {
		t.Fatalf("result=%+v ensured=%v", result, indexer.ensured)
	}
	if len(indexer.items) != 3 || indexer.items[2].ID != 3 {
		t.Fatalf("indexed items = %+v", indexer.items)
	}
}

type fakeProgramIndexSource struct {
	items []program.Program
}

func (s *fakeProgramIndexSource) ListProgramsAfterID(ctx context.Context, afterID int64, limit int) ([]program.Program, error) {
	result := make([]program.Program, 0, limit)
	for _, item := range s.items {
		if item.ID > afterID {
			result = append(result, item)
		}
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

type fakeProgramIndexer struct {
	ensured bool
	items   []program.Program
}

func (i *fakeProgramIndexer) DeletePrograms(ctx context.Context, programIDs []int64) error {
	return nil
}

func (i *fakeProgramIndexer) EnsureIndex(ctx context.Context) error {
	i.ensured = true
	return nil
}

func (i *fakeProgramIndexer) ActivateIndex(ctx context.Context) error {
	return nil
}

func (i *fakeProgramIndexer) UpsertPrograms(ctx context.Context, items []program.Program) error {
	i.items = append(i.items, items...)
	return nil
}
