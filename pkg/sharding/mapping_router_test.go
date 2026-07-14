package sharding

import (
	"testing"
	"time"

	"tickethub/pkg/idgen"
)

func TestMappingOrderRouterUsesRuntimePrimaryAndShadow(t *testing.T) {
	snowflake, _ := idgen.NewSnowflakeWithClock(1, func() time.Time {
		return time.UnixMilli(1704067200001)
	})
	orderNumber, err := idgen.NewOrderNumberGenerator(snowflake).NextOrderNumber(2049)
	if err != nil {
		t.Fatal(err)
	}
	fallback := NewGeneOrderRouter("tickethub_order", "orders", 2, 4)
	router := NewMappingOrderRouter(fallback)
	shadow := Location{Database: "tickethub_order_1", Table: "orders_3"}
	if err := router.ReplaceMappings([]ShardMapping{{
		VirtualShard: 1,
		Primary:      Location{Database: "tickethub_order_0", Table: "orders_1"},
		Shadow:       &shadow,
		WriteMode:    WriteDual,
		Version:      2,
	}}); err != nil {
		t.Fatal(err)
	}

	writes := router.RouteOrderWrites(orderNumber, 2049)
	if len(writes) != 2 || writes[0].Table != "orders_1" || writes[1].Table != "orders_3" {
		t.Fatalf("writes = %+v", writes)
	}
	if got := router.RouteOrder(orderNumber, 2049); got.Database != "tickethub_order_0" || got.Table != "orders_1" {
		t.Fatalf("read location = %+v", got)
	}
}

func TestMappingOrderRouterRejectsDualWriteWithoutShadow(t *testing.T) {
	router := NewMappingOrderRouter(NewGeneOrderRouter("tickethub_order", "orders", 2, 2))
	err := router.ReplaceMappings([]ShardMapping{{
		VirtualShard: 0,
		Primary:      Location{Database: "tickethub_order_0", Table: "orders_0"},
		WriteMode:    WriteDual,
	}})
	if err == nil {
		t.Fatal("expected invalid mapping error")
	}
}
