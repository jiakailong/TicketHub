package application

import (
	"context"

	"tickethub/pkg/observability"
	"tickethub/pkg/sharding"
)

type ShardMappingSource interface {
	ListShardMappings(ctx context.Context) ([]sharding.ShardMapping, error)
}

type ShardMappingRouter interface {
	ReplaceMappings(mappings []sharding.ShardMapping) error
}

type ShardMappingRefreshService struct {
	source ShardMappingSource
	router ShardMappingRouter
}

func NewShardMappingRefreshService(source ShardMappingSource, router ShardMappingRouter) ShardMappingRefreshService {
	return ShardMappingRefreshService{source: source, router: router}
}

func (s ShardMappingRefreshService) Refresh(ctx context.Context) error {
	mappings, err := s.source.ListShardMappings(ctx)
	if err != nil {
		return err
	}
	if err := s.router.ReplaceMappings(mappings); err != nil {
		return err
	}
	observability.IncCounter("ticket_hub_shard_mapping_refresh_total", map[string]string{"result": "success"})
	return nil
}
