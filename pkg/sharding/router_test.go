package sharding

import (
	"testing"
	"time"

	"tickethub/pkg/idgen"
)

func TestGeneOrderRouterUsesEmbeddedUserGene(t *testing.T) {
	sf, _ := idgen.NewSnowflakeWithClock(1, func() time.Time {
		return time.UnixMilli(1704067200001)
	})
	gen := idgen.NewOrderNumberGenerator(sf)
	orderNumber, err := gen.NextOrderNumber(2049)
	if err != nil {
		t.Fatal(err)
	}
	router := NewGeneOrderRouter("tickethub_order", "orders", 2, 4)
	location := router.RouteOrder(orderNumber, 2049)
	if location.Database != "tickethub_order_0" || location.Table != "orders_1" {
		t.Fatalf("location = %+v", location)
	}
}

func TestGeneOrderRouterListsEveryPhysicalLocation(t *testing.T) {
	router := NewGeneOrderRouter("tickethub_order", "orders", 2, 2)
	locations := router.PrimaryLocations()

	if len(locations) != 4 {
		t.Fatalf("locations = %+v", locations)
	}
	if locations[0].Database != "tickethub_order_0" || locations[0].Table != "orders_0" {
		t.Fatalf("first location = %+v", locations[0])
	}
	if locations[3].Database != "tickethub_order_1" || locations[3].Table != "orders_1" {
		t.Fatalf("last location = %+v", locations[3])
	}
}
