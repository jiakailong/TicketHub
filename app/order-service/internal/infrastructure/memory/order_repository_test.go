package memory

import (
	"context"
	"testing"

	"tickethub/app/order-service/internal/domain/order"
)

func TestOrderRepositoryListInventoryUsageCountsOnlyOccupiedOrders(t *testing.T) {
	repo := NewOrderRepository()
	orders := []order.Order{
		{OrderNumber: 1, ProgramID: 10001, TicketCategoryID: 10, SeatIDs: []int64{1, 2}, Status: order.StatusNoPay},
		{OrderNumber: 2, ProgramID: 10001, TicketCategoryID: 10, TicketUserIDs: []int64{1, 2, 3}, Status: order.StatusPaid},
		{OrderNumber: 3, ProgramID: 10001, TicketCategoryID: 10, SeatIDs: []int64{3, 4}, Status: order.StatusCancel},
		{OrderNumber: 4, ProgramID: 20002, TicketCategoryID: 10, SeatIDs: []int64{5}, Status: order.StatusNoPay},
		{OrderNumber: 5, ProgramID: 10001, TicketCategoryID: 20, SeatIDs: []int64{6}, Status: order.StatusPaid},
	}
	for _, item := range orders {
		if err := repo.Save(context.Background(), item); err != nil {
			t.Fatal(err)
		}
	}

	usages, err := repo.ListInventoryUsage(context.Background(), 10001)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[int64]int64, len(usages))
	for _, usage := range usages {
		counts[usage.TicketCategoryID] = usage.OccupiedCount
	}
	if counts[10] != 5 || counts[20] != 1 || len(counts) != 2 {
		t.Fatalf("counts = %+v", counts)
	}
}
