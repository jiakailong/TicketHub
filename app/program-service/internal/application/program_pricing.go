package application

import (
	"context"
	"math"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

type TicketCategorySource interface {
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
}

type ProgramPricingService struct {
	categories TicketCategorySource
}

func NewProgramPricingService(categories TicketCategorySource) ProgramPricingService {
	return ProgramPricingService{categories: categories}
}

func (s ProgramPricingService) CalculateAmount(ctx context.Context, cmd program.CreateOrderCommand) (int64, error) {
	count := cmd.TicketCount()
	if count <= 0 {
		return 0, therrors.New(therrors.CodeInvalidArgument, "ticket user or seat is required")
	}
	items, err := s.categories.ListTicketCategories(ctx, cmd.ProgramID)
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if item.ID != cmd.TicketCategoryID {
			continue
		}
		if !item.SellStarted {
			return 0, therrors.New(therrors.CodeConflict, "ticket category is not on sale")
		}
		if item.PriceCent <= 0 || item.PriceCent > math.MaxInt64/count {
			return 0, therrors.New(therrors.CodeInvalidArgument, "ticket price is invalid")
		}
		return item.PriceCent * count, nil
	}
	return 0, therrors.New(therrors.CodeNotFound, "ticket category not found")
}
