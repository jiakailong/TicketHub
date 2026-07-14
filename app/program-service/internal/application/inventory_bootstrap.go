package application

import (
	"context"

	"tickethub/app/program-service/internal/domain/program"
)

type InventoryBootstrapCatalog interface {
	ListTicketCategoriesAfterID(ctx context.Context, afterID int64, limit int) ([]program.TicketCategory, error)
}

type InventoryInitializer interface {
	InitializeRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) (bool, error)
}

type InventoryBootstrapResult struct {
	Initialized int64
	Existing    int64
}

type InventoryBootstrapService struct {
	catalog   InventoryBootstrapCatalog
	store     InventoryInitializer
	batchSize int
}

func NewInventoryBootstrapService(catalog InventoryBootstrapCatalog, store InventoryInitializer, batchSize int) InventoryBootstrapService {
	if batchSize <= 0 || batchSize > 2000 {
		batchSize = 500
	}
	return InventoryBootstrapService{catalog: catalog, store: store, batchSize: batchSize}
}

func (s InventoryBootstrapService) Bootstrap(ctx context.Context) (InventoryBootstrapResult, error) {
	var result InventoryBootstrapResult
	var afterID int64
	for {
		categories, err := s.catalog.ListTicketCategoriesAfterID(ctx, afterID, s.batchSize)
		if err != nil {
			return result, err
		}
		if len(categories) == 0 {
			return result, nil
		}
		for _, category := range categories {
			initialized, err := s.store.InitializeRemain(ctx, category.ProgramID, category.ID, category.Remain)
			if err != nil {
				return result, err
			}
			if initialized {
				result.Initialized++
			} else {
				result.Existing++
			}
		}
		afterID = categories[len(categories)-1].ID
		if len(categories) < s.batchSize {
			return result, nil
		}
	}
}
