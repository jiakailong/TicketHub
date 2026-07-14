package redis

import (
	"context"
	"encoding/json"
	"math/rand"
	"strings"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/app/program-service/internal/domain/program"
	"tickethub/pkg/cache"
)

type programQuerySource interface {
	SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error)
	FindProgram(ctx context.Context, programID int64) (program.Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error)
}

type ProgramQueryCache struct {
	source programQuerySource
	client redislib.Cmdable
	keys   cache.KeyBuilder
	ttl    time.Duration
}

func NewProgramQueryCache(source programQuerySource, client redislib.Cmdable, keys cache.KeyBuilder, ttl time.Duration) ProgramQueryCache {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return ProgramQueryCache{source: source, client: client, keys: keys, ttl: ttl}
}

func (c ProgramQueryCache) SearchPrograms(ctx context.Context, keyword string, city string, page int, pageSize int) ([]program.Program, error) {
	return c.source.SearchPrograms(ctx, keyword, city, page, pageSize)
}

func (c ProgramQueryCache) SearchProgramsAfter(ctx context.Context, keyword string, city string, cursor string, pageSize int) ([]program.Program, string, error) {
	if source, ok := c.source.(interface {
		SearchProgramsAfter(context.Context, string, string, string, int) ([]program.Program, string, error)
	}); ok {
		return source.SearchProgramsAfter(ctx, keyword, city, cursor, pageSize)
	}
	items, err := c.source.SearchPrograms(ctx, keyword, city, 1, pageSize)
	return items, "", err
}

func (c ProgramQueryCache) SuggestPrograms(ctx context.Context, prefix string, limit int) ([]string, error) {
	if source, ok := c.source.(interface {
		SuggestPrograms(context.Context, string, int) ([]string, error)
	}); ok {
		return source.SuggestPrograms(ctx, prefix, limit)
	}
	items, err := c.source.SearchPrograms(ctx, prefix, "", 1, limit)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Title)
	}
	return result, nil
}

func (c ProgramQueryCache) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	key := c.keys.Build("cache", "program", programID)
	var item program.Program
	if c.loadJSON(ctx, key, &item) {
		return item, nil
	}
	item, err := c.source.FindProgram(ctx, programID)
	if err != nil {
		return program.Program{}, err
	}
	c.storeJSON(ctx, key, item)
	return item, nil
}

func (c ProgramQueryCache) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	key := c.keys.Build("cache", "program-categories", programID)
	var items []program.TicketCategory
	if c.loadJSON(ctx, key, &items) {
		return items, nil
	}
	items, err := c.source.ListTicketCategories(ctx, programID)
	if err != nil {
		return nil, err
	}
	c.storeJSON(ctx, key, items)
	return items, nil
}

func (c ProgramQueryCache) ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error) {
	items, err := c.source.ListSeats(ctx, programID, ticketCategoryID)
	if err != nil || len(items) == 0 {
		return items, err
	}
	keys := make([]string, len(items))
	for index, item := range items {
		keys[index] = c.keys.Build("seat", programID, item.ID)
	}
	states, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		// Real-time state is an enhancement for display. The Redis Lua script
		// remains the authoritative check when the user submits an order.
		return items, nil
	}
	for index, raw := range states {
		value, _ := raw.(string)
		switch {
		case strings.HasPrefix(value, "locked:"):
			items[index].Status = program.SeatLocked
		case strings.HasPrefix(value, "sold:"):
			items[index].Status = program.SeatSold
		}
	}
	return items, nil
}

func (c ProgramQueryCache) MinPricesByProgramIDs(ctx context.Context, programIDs []int64) (map[int64]int64, error) {
	if source, ok := c.source.(interface {
		MinPricesByProgramIDs(context.Context, []int64) (map[int64]int64, error)
	}); ok {
		return source.MinPricesByProgramIDs(ctx, programIDs)
	}
	result := make(map[int64]int64, len(programIDs))
	for _, programID := range programIDs {
		categories, err := c.ListTicketCategories(ctx, programID)
		if err != nil {
			return nil, err
		}
		for _, category := range categories {
			if result[programID] == 0 || category.PriceCent < result[programID] {
				result[programID] = category.PriceCent
			}
		}
	}
	return result, nil
}

func (c ProgramQueryCache) Invalidate(ctx context.Context, programID int64) error {
	return c.client.Del(ctx,
		c.keys.Build("cache", "program", programID),
		c.keys.Build("cache", "program-categories", programID),
	).Err()
}

func (c ProgramQueryCache) loadJSON(ctx context.Context, key string, target any) bool {
	data, err := c.client.Get(ctx, key).Bytes()
	return err == nil && json.Unmarshal(data, target) == nil
}

func (c ProgramQueryCache) storeJSON(ctx context.Context, key string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	jitter := time.Duration(rand.Int63n(int64(c.ttl/5 + 1)))
	_ = c.client.Set(ctx, key, data, c.ttl+jitter).Err()
}
