package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

type ProgramRepository struct {
	mu         sync.RWMutex
	programs   map[int64]program.Program
	categories map[int64][]program.TicketCategory
	seats      map[int64][]program.Seat
}

func (r *ProgramRepository) SaveProgramWithEvent(ctx context.Context, item program.Program, categories []program.TicketCategory, event mq.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.programs[item.ID] = item
	r.categories[item.ID] = append([]program.TicketCategory(nil), categories...)
	return nil
}

func (r *ProgramRepository) ListTicketCategoriesAfterID(ctx context.Context, afterID int64, limit int) ([]program.TicketCategory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var items []program.TicketCategory
	for _, categories := range r.categories {
		for _, category := range categories {
			if category.ID > afterID {
				items = append(items, category)
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func NewProgramRepository() *ProgramRepository {
	repo := &ProgramRepository{
		programs:   make(map[int64]program.Program),
		categories: make(map[int64][]program.TicketCategory),
		seats:      make(map[int64][]program.Seat),
	}
	repo.SeedDefaults()
	return repo
}

func (r *ProgramRepository) SeedDefaults() {
	now := time.Now().Add(30 * 24 * time.Hour)
	r.programs[10001] = program.Program{
		ID:       10001,
		Title:    "TicketHub Live 2026",
		City:     "Shanghai",
		Place:    "Mercedes-Benz Arena",
		ShowTime: now,
		Status:   "ON_SALE",
	}
	r.categories[10001] = []program.TicketCategory{
		{ID: 1, ProgramID: 10001, Name: "A区", PriceCent: 128000, Total: 1000, Remain: 1000, SellStarted: true},
		{ID: 2, ProgramID: 10001, Name: "B区", PriceCent: 88000, Total: 2000, Remain: 2000, SellStarted: true},
	}
	r.seats[1] = []program.Seat{
		{ID: 100, ProgramID: 10001, TicketCategoryID: 1, RowCode: "A", ColCode: "01", PriceCent: 128000, Status: program.SeatNoSold},
		{ID: 101, ProgramID: 10001, TicketCategoryID: 1, RowCode: "A", ColCode: "02", PriceCent: 128000, Status: program.SeatNoSold},
	}
}

func (r *ProgramRepository) SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []program.Program
	for _, item := range r.programs {
		if keyword != "" && !strings.Contains(strings.ToLower(item.Title), strings.ToLower(keyword)) {
			continue
		}
		if city != "" && !strings.EqualFold(item.City, city) {
			continue
		}
		matched = append(matched, item)
	}
	start := (page - 1) * pageSize
	if start >= len(matched) {
		return nil, nil
	}
	end := start + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return append([]program.Program(nil), matched[start:end]...), nil
}

func (r *ProgramRepository) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.programs[programID]
	if !ok {
		return program.Program{}, therrors.New(therrors.CodeNotFound, "program not found")
	}
	return item, nil
}

func (r *ProgramRepository) FindProgramsByIDs(ctx context.Context, programIDs []int64) ([]program.Program, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]program.Program, 0, len(programIDs))
	for _, id := range programIDs {
		if item, ok := r.programs[id]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *ProgramRepository) MinPricesByProgramIDs(ctx context.Context, programIDs []int64) (map[int64]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[int64]int64, len(programIDs))
	for _, id := range programIDs {
		for _, category := range r.categories[id] {
			if category.SellStarted && (result[id] == 0 || category.PriceCent < result[id]) {
				result[id] = category.PriceCent
			}
		}
	}
	return result, nil
}

func (r *ProgramRepository) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]program.TicketCategory(nil), r.categories[programID]...), nil
}

func (r *ProgramRepository) UpdateTicketCategoryRemain(ctx context.Context, ticketCategoryID int64, remain int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for programID, categories := range r.categories {
		for index := range categories {
			if categories[index].ID != ticketCategoryID {
				continue
			}
			categories[index].Remain = remain
			r.categories[programID] = categories
			return nil
		}
	}
	return therrors.New(therrors.CodeNotFound, "ticket category not found")
}

func (r *ProgramRepository) ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]program.Seat(nil), r.seats[ticketCategoryID]...), nil
}
