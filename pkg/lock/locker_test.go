package lock

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLocker(t *testing.T) {
	locker := NewMemoryLocker()
	first, err := locker.Acquire(context.Background(), "order:1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := locker.Acquire(context.Background(), "order:1", time.Minute); err == nil {
		t.Fatal("expected second acquire to fail")
	}
	if err := first.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := locker.Acquire(context.Background(), "order:1", time.Minute); err != nil {
		t.Fatal(err)
	}
}
