package application

import (
	"context"
	"strings"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

type ProgramSearchRepository interface {
	SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error)
	FindProgram(ctx context.Context, programID int64) (program.Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error)
}

type ProgramDetail struct {
	Program          program.Program
	TicketCategories []program.TicketCategory
	Seats            []program.Seat
}

type ProgramListItem struct {
	Program      program.Program
	MinPriceCent int64
}

type ProgramMinimumPriceRepository interface {
	MinPricesByProgramIDs(ctx context.Context, programIDs []int64) (map[int64]int64, error)
}

type ProgramCursorRepository interface {
	SearchProgramsAfter(ctx context.Context, keyword string, city string, cursor string, pageSize int) ([]program.Program, string, error)
}

type ProgramSuggestionRepository interface {
	SuggestPrograms(ctx context.Context, prefix string, limit int) ([]string, error)
}

type ProgramPage struct {
	Items      []ProgramListItem
	NextCursor string
}

type ProgramQueryService struct {
	repo ProgramSearchRepository
}

func NewProgramQueryService(repo ProgramSearchRepository) ProgramQueryService {
	return ProgramQueryService{repo: repo}
}

func (s ProgramQueryService) Search(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	return s.repo.SearchPrograms(ctx, strings.TrimSpace(keyword), strings.TrimSpace(city), page, pageSize)
}

func (s ProgramQueryService) SearchWithPrices(ctx context.Context, keyword string, city string, page int, pageSize int) ([]ProgramListItem, error) {
	programs, err := s.Search(ctx, keyword, city, page, pageSize)
	if err != nil {
		return nil, err
	}
	return s.withPrices(ctx, programs)
}

func (s ProgramQueryService) SearchPage(ctx context.Context, keyword string, city string, cursor string, page int, pageSize int) (ProgramPage, error) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if repository, ok := s.repo.(ProgramCursorRepository); ok && (cursor != "" || page <= 1) {
		programs, next, err := repository.SearchProgramsAfter(ctx, strings.TrimSpace(keyword), strings.TrimSpace(city), strings.TrimSpace(cursor), pageSize)
		if err != nil {
			return ProgramPage{}, err
		}
		items, err := s.withPrices(ctx, programs)
		return ProgramPage{Items: items, NextCursor: next}, err
	}
	items, err := s.SearchWithPrices(ctx, keyword, city, page, pageSize)
	return ProgramPage{Items: items}, err
}

func (s ProgramQueryService) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return []string{}, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	if repository, ok := s.repo.(ProgramSuggestionRepository); ok {
		return repository.SuggestPrograms(ctx, prefix, limit)
	}
	items, err := s.repo.SearchPrograms(ctx, prefix, "", 1, limit)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Title)
	}
	return result, nil
}

func (s ProgramQueryService) withPrices(ctx context.Context, programs []program.Program) ([]ProgramListItem, error) {
	ids := make([]int64, 0, len(programs))
	for _, item := range programs {
		ids = append(ids, item.ID)
	}
	prices := make(map[int64]int64, len(ids))
	if repository, ok := s.repo.(ProgramMinimumPriceRepository); ok && len(ids) > 0 {
		loadedPrices, err := repository.MinPricesByProgramIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		prices = loadedPrices
	} else {
		for _, item := range programs {
			categories, categoryErr := s.repo.ListTicketCategories(ctx, item.ID)
			if categoryErr != nil {
				return nil, categoryErr
			}
			for _, category := range categories {
				if prices[item.ID] == 0 || category.PriceCent < prices[item.ID] {
					prices[item.ID] = category.PriceCent
				}
			}
		}
	}
	items := make([]ProgramListItem, 0, len(programs))
	for _, item := range programs {
		items = append(items, ProgramListItem{Program: item, MinPriceCent: prices[item.ID]})
	}
	return items, nil
}

func (s ProgramQueryService) Detail(ctx context.Context, programID int64, ticketCategoryID int64) (ProgramDetail, error) {
	if programID <= 0 {
		return ProgramDetail{}, therrors.New(therrors.CodeInvalidArgument, "program_id is required")
	}
	current, err := s.repo.FindProgram(ctx, programID)
	if err != nil {
		return ProgramDetail{}, err
	}
	categories, err := s.repo.ListTicketCategories(ctx, programID)
	if err != nil {
		return ProgramDetail{}, err
	}
	var seats []program.Seat
	if ticketCategoryID > 0 {
		seats, err = s.repo.ListSeats(ctx, programID, ticketCategoryID)
		if err != nil {
			return ProgramDetail{}, err
		}
	}
	return ProgramDetail{Program: current, TicketCategories: categories, Seats: seats}, nil
}
