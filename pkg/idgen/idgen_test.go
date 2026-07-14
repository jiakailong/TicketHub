package idgen

import (
	"testing"
	"time"
)

func TestOrderNumberEmbedsUserGene(t *testing.T) {
	sf, err := NewSnowflakeWithClock(1, func() time.Time {
		return time.UnixMilli(defaultEpochMS + 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	gen := NewOrderNumberGenerator(sf)
	orderNumber, err := gen.NextOrderNumber(2049)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ExtractUserGene(orderNumber), UserGene(2049); got != want {
		t.Fatalf("gene = %d, want %d", got, want)
	}
	if orderNumber <= 0 {
		t.Fatalf("order number must be positive, got %d", orderNumber)
	}
}

func TestOrderNumberIsUniqueWithinSameMillisecond(t *testing.T) {
	sf, err := NewSnowflakeWithClock(2, func() time.Time {
		return time.UnixMilli(defaultEpochMS + 80_000_000_000)
	})
	if err != nil {
		t.Fatal(err)
	}
	gen := NewOrderNumberGenerator(sf)
	first, err := gen.NextOrderNumber(1001)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gen.NextOrderNumber(1001)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("order numbers must be unique: %d", first)
	}
	if first <= 0 || second <= 0 {
		t.Fatalf("order numbers must remain positive: %d, %d", first, second)
	}
}

func TestOrderNumberRejectsUnsupportedNode(t *testing.T) {
	sf, err := NewSnowflakeWithClock(orderNodeMask+1, func() time.Time {
		return time.UnixMilli(defaultEpochMS + 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewOrderNumberGenerator(sf).NextOrderNumber(1); err == nil {
		t.Fatal("expected unsupported order-number node to fail")
	}
}
