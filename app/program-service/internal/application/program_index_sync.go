package application

import (
	"context"

	"tickethub/app/program-service/internal/domain/program"
)

type ProgramIndexSource interface {
	ListProgramsAfterID(ctx context.Context, afterID int64, limit int) ([]program.Program, error)
}

type ProgramSearchIndexer interface {
	EnsureIndex(ctx context.Context) error
	ActivateIndex(ctx context.Context) error
	UpsertPrograms(ctx context.Context, programs []program.Program) error
	DeletePrograms(ctx context.Context, programIDs []int64) error
}

type ProgramIndexSyncResult struct {
	Indexed int64
	Batches int64
}

type ProgramIndexSyncService struct {
	source    ProgramIndexSource
	indexer   ProgramSearchIndexer
	batchSize int
}

func NewProgramIndexSyncService(source ProgramIndexSource, indexer ProgramSearchIndexer, batchSize int) ProgramIndexSyncService {
	if batchSize <= 0 || batchSize > 2000 {
		batchSize = 500
	}
	return ProgramIndexSyncService{source: source, indexer: indexer, batchSize: batchSize}
}

func (s ProgramIndexSyncService) Sync(ctx context.Context) (ProgramIndexSyncResult, error) {
	if err := s.indexer.EnsureIndex(ctx); err != nil {
		return ProgramIndexSyncResult{}, err
	}
	var result ProgramIndexSyncResult
	var afterID int64
	for {
		items, err := s.source.ListProgramsAfterID(ctx, afterID, s.batchSize)
		if err != nil {
			return result, err
		}
		if len(items) == 0 {
			return result, s.indexer.ActivateIndex(ctx)
		}
		upserts := make([]program.Program, 0, len(items))
		deletes := make([]int64, 0)
		for _, item := range items {
			if item.Status == "ON_SALE" {
				upserts = append(upserts, item)
			} else {
				deletes = append(deletes, item.ID)
			}
		}
		if err := s.indexer.UpsertPrograms(ctx, upserts); err != nil {
			return result, err
		}
		if err := s.indexer.DeletePrograms(ctx, deletes); err != nil {
			return result, err
		}
		result.Indexed += int64(len(items))
		result.Batches++
		afterID = items[len(items)-1].ID
		if len(items) < s.batchSize {
			return result, s.indexer.ActivateIndex(ctx)
		}
	}
}
