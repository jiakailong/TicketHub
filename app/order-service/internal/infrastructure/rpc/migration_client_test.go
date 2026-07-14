package rpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"

	migratev1 "tickethub/api/proto/migrate/v1"
	"tickethub/pkg/sharding"
)

func TestMigrationClientMapsRuntimeShardSnapshot(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	migratev1.RegisterMigrateServiceServer(server, fakeMigrateService{})
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	client, err := NewMigrationGRPCClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	mappings, err := client.ListShardMappings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(mappings) != 1 || mappings[0].VirtualShard != 0 || mappings[0].WriteMode != sharding.WriteDual || mappings[0].Shadow == nil {
		t.Fatalf("mappings = %+v", mappings)
	}
	if mappings[0].Shadow.Database != "tickethub_order_2" || mappings[0].Shadow.Table != "orders_0" {
		t.Fatalf("shadow = %+v", mappings[0].Shadow)
	}
}

type fakeMigrateService struct {
	migratev1.UnimplementedMigrateServiceServer
}

func (fakeMigrateService) ListShardMappings(ctx context.Context, req *migratev1.ListShardMappingsRequest) (*migratev1.ListShardMappingsReply, error) {
	return &migratev1.ListShardMappingsReply{Mappings: []*migratev1.ShardMapping{{
		VirtualShard:  0,
		PhysicalDb:    "tickethub_order_0",
		PhysicalTable: "orders_0",
		ShadowDb:      "tickethub_order_2",
		ShadowTable:   "orders_0",
		WriteMode:     "DUAL_WRITE",
		Version:       2,
	}}}, nil
}
