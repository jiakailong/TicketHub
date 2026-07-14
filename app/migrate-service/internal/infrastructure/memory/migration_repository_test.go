package memory

import (
	"context"
	"testing"
	"time"

	"tickethub/app/migrate-service/internal/domain/migrate"
)

func TestMigrationRepositoryActivatesDualWriteAndCutsOver(t *testing.T) {
	repository := NewMigrationRepository()
	task := migrate.MigrationTask{
		ID: 1, VirtualShard: 0,
		SourceShard: "tickethub_order_0.orders_0",
		TargetShard: "tickethub_order_2.orders_0",
		Status:      migrate.TaskPending, BatchSize: 100,
		CreatedAt: time.Now().Add(-time.Minute),
	}
	if err := repository.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	mappings, _ := repository.LoadShardMappings(context.Background())
	if mappings[0].WriteMode != "DUAL_WRITE" || mappings[0].ShadowDB != "tickethub_order_2" {
		t.Fatalf("dual-write mapping = %+v", mappings[0])
	}
	if err := repository.CompleteTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	mappings, _ = repository.LoadShardMappings(context.Background())
	if mappings[0].WriteMode != "PRIMARY_ONLY" || mappings[0].PhysicalDB != "tickethub_order_2" || mappings[0].ShadowDB != "" {
		t.Fatalf("cutover mapping = %+v", mappings[0])
	}
}
