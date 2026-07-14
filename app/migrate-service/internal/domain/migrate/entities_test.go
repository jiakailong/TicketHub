package migrate

import "testing"

func TestParseShardLocation(t *testing.T) {
	database, table, err := ParseShardLocation("tickethub_order_2.orders_0")
	if err != nil {
		t.Fatal(err)
	}
	if database != "tickethub_order_2" || table != "orders_0" {
		t.Fatalf("database=%s table=%s", database, table)
	}
	if _, _, err := ParseShardLocation("tickethub_order_2.orders_0;DROP TABLE orders"); err == nil {
		t.Fatal("expected unsafe shard location to be rejected")
	}
}
