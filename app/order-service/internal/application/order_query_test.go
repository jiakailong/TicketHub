package application

import (
	"context"
	"sort"
	"testing"
	"time"

	"tickethub/app/order-service/internal/domain/order"
)

func TestOrderQueryServiceCursorPaginationIsStable(t *testing.T) {
	repo := newFakeStatefulOrderRepo()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for _, item := range []order.Order{
		{OrderNumber: 103, UserID: 7, Status: order.StatusNoPay, CreatedAt: now},
		{OrderNumber: 102, UserID: 7, Status: order.StatusPaid, CreatedAt: now},
		{OrderNumber: 101, UserID: 7, Status: order.StatusNoPay, CreatedAt: now.Add(-time.Second)},
	} {
		if err := repo.Save(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}
	queries := NewOrderQueryService(repo)
	first, err := queries.ListPage(context.Background(), 7, "", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].OrderNumber != 103 || first.Items[1].OrderNumber != 102 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := queries.ListPage(context.Background(), 7, "", first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].OrderNumber != 101 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
}

func TestOrderQueryServiceFiltersStatus(t *testing.T) {
	repo := newFakeStatefulOrderRepo()
	now := time.Now()
	_ = repo.Save(context.Background(), order.Order{OrderNumber: 1, UserID: 7, Status: order.StatusNoPay, CreatedAt: now})
	_ = repo.Save(context.Background(), order.Order{OrderNumber: 2, UserID: 7, Status: order.StatusPaid, CreatedAt: now})
	page, err := NewOrderQueryService(repo).ListPage(context.Background(), 7, "pay", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Status != order.StatusPaid {
		t.Fatalf("page = %+v", page)
	}
}

func (r *fakeStatefulOrderRepo) ListByUserIDPage(ctx context.Context, userID int64, status order.Status, before OrderListCursor, limit int) ([]order.Order, error) {
	items := make([]order.Order, 0)
	for _, item := range r.items {
		if item.UserID != userID || (status != "" && item.Status != status) {
			continue
		}
		if !before.CreatedAt.IsZero() && !item.CreatedAt.Before(before.CreatedAt) && !(item.CreatedAt.Equal(before.CreatedAt) && item.OrderNumber < before.OrderNumber) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].OrderNumber > items[j].OrderNumber
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
