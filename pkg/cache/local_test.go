package cache

import (
	"testing"
	"time"
)

func TestRistrettoLocalStoresAndExpiresValues(t *testing.T) {
	local, err := NewRistrettoLocal(RistrettoConfig{NumCounters: 100, MaxCost: 1024, BufferItems: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()

	if !local.Set("program:1", []byte("value"), 20*time.Millisecond) {
		t.Fatal("expected cache admission")
	}
	local.Wait()
	if value, ok := local.Get("program:1"); !ok || string(value) != "value" {
		t.Fatalf("cached value = %q, found = %t", value, ok)
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := local.Get("program:1"); ok {
		t.Fatal("expected cached value to expire")
	}
}

func TestStripedRWMutexReturnsStableStripe(t *testing.T) {
	locks := NewStripedRWMutex(16)
	if locks.For("program:1") != locks.For("program:1") {
		t.Fatal("same key must map to the same stripe")
	}
}
