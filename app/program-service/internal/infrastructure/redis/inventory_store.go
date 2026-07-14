package redis

import (
	"context"
	"fmt"
	"strconv"

	redislib "github.com/redis/go-redis/v9"

	"tickethub/pkg/cache"
	therrors "tickethub/pkg/errors"
)

type InventoryStore struct {
	client redislib.Cmdable
	keys   cache.KeyBuilder
}

func NewInventoryStore(client redislib.Cmdable, keys cache.KeyBuilder) InventoryStore {
	return InventoryStore{client: client, keys: keys}
}

func (s InventoryStore) GetRemain(ctx context.Context, programID int64, ticketCategoryID int64) (int64, bool, error) {
	value, err := s.client.Get(ctx, s.keys.Build("inventory", programID, ticketCategoryID)).Int64()
	if err == redislib.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, therrors.Wrap(therrors.CodeInfrastructure, "read redis inventory failed", err)
	}
	return value, true, nil
}

func (s InventoryStore) SetRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) error {
	if err := s.client.Set(ctx, s.keys.Build("inventory", programID, ticketCategoryID), remain, 0).Err(); err != nil {
		return therrors.Wrap(therrors.CodeInfrastructure, "repair redis inventory failed", err)
	}
	return nil
}

func (s InventoryStore) InitializeRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) (bool, error) {
	initialized, err := s.client.SetNX(ctx, s.keys.Build("inventory", programID, ticketCategoryID), remain, 0).Result()
	if err != nil {
		return false, therrors.Wrap(therrors.CodeInfrastructure, "initialize redis inventory failed", err)
	}
	return initialized, nil
}

func (s InventoryStore) HasActiveReservations(ctx context.Context, programID int64) (bool, error) {
	iterator := s.client.Scan(ctx, 0, s.keys.Build("order-lock", "*"), 100).Iterator()
	for iterator.Next(ctx) {
		values, err := s.client.HMGet(ctx, iterator.Val(), "program_id", "status").Result()
		if err != nil {
			return false, therrors.Wrap(therrors.CodeInfrastructure, "inspect active inventory reservation failed", err)
		}
		if len(values) < 2 || values[0] == nil {
			continue
		}
		storedProgramID, err := parseRedisInt64(values[0])
		if err != nil {
			return false, therrors.Wrap(therrors.CodeInfrastructure, "inspect active inventory reservation failed", err)
		}
		status, _ := values[1].(string)
		if storedProgramID == programID && status != "sold" {
			return true, nil
		}
	}
	if err := iterator.Err(); err != nil {
		return false, therrors.Wrap(therrors.CodeInfrastructure, "scan active inventory reservations failed", err)
	}
	return false, nil
}

func parseRedisInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected redis integer type %T", value)
	}
}
