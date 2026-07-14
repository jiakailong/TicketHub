package rpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	migratev1 "tickethub/api/proto/migrate/v1"
	"tickethub/pkg/sharding"
)

type MigrationClient struct {
	client migratev1.MigrateServiceClient
	conn   *grpc.ClientConn
}

func NewMigrationGRPCClient(addr string) (MigrationClient, error) {
	if strings.TrimSpace(addr) == "" {
		return MigrationClient{}, nil
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return MigrationClient{}, err
	}
	return MigrationClient{client: migratev1.NewMigrateServiceClient(conn), conn: conn}, nil
}

func (c MigrationClient) ListShardMappings(ctx context.Context) ([]sharding.ShardMapping, error) {
	if c.client == nil {
		return nil, nil
	}
	reply, err := c.client.ListShardMappings(ctx, &migratev1.ListShardMappingsRequest{})
	if err != nil {
		return nil, err
	}
	mappings := make([]sharding.ShardMapping, 0, len(reply.GetMappings()))
	for _, mapping := range reply.GetMappings() {
		item := sharding.ShardMapping{
			VirtualShard: int(mapping.GetVirtualShard()),
			Primary: sharding.Location{
				Database: mapping.GetPhysicalDb(),
				Table:    mapping.GetPhysicalTable(),
			},
			WriteMode: sharding.WriteMode(mapping.GetWriteMode()),
			Version:   mapping.GetVersion(),
		}
		if mapping.GetShadowDb() != "" && mapping.GetShadowTable() != "" {
			item.Shadow = &sharding.Location{Database: mapping.GetShadowDb(), Table: mapping.GetShadowTable()}
		}
		mappings = append(mappings, item)
	}
	return mappings, nil
}

func (c MigrationClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
