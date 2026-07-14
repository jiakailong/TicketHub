package application

import (
	"context"
	"testing"

	"tickethub/pkg/sharding"
)

func TestShardMappingRefreshServiceReplacesRouterSnapshot(t *testing.T) {
	mappings := []sharding.ShardMapping{{
		VirtualShard: 0,
		Primary:      sharding.Location{Database: "tickethub_order_0", Table: "orders_0"},
		WriteMode:    sharding.WritePrimaryOnly,
		Version:      1,
	}}
	source := fakeShardMappingSource{mappings: mappings}
	router := &fakeShardMappingRouter{}
	service := NewShardMappingRefreshService(source, router)

	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(router.mappings) != 1 || router.mappings[0].Primary.Table != "orders_0" {
		t.Fatalf("mappings = %+v", router.mappings)
	}
}

type fakeShardMappingSource struct {
	mappings []sharding.ShardMapping
}

func (s fakeShardMappingSource) ListShardMappings(ctx context.Context) ([]sharding.ShardMapping, error) {
	return append([]sharding.ShardMapping(nil), s.mappings...), nil
}

type fakeShardMappingRouter struct {
	mappings []sharding.ShardMapping
}

func (r *fakeShardMappingRouter) ReplaceMappings(mappings []sharding.ShardMapping) error {
	r.mappings = append([]sharding.ShardMapping(nil), mappings...)
	return nil
}
