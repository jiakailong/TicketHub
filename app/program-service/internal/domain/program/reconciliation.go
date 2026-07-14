package program

import "time"

type InventoryUsage struct {
	TicketCategoryID int64
	OccupiedCount    int64
}

type InventoryDifference struct {
	TicketCategoryID int64
	Total            int64
	OccupiedCount    int64
	ExpectedRemain   int64
	MySQLRemain      int64
	RedisRemain      int64
	RedisExists      bool
	Repaired         bool
	Reason           string
}

type ReconciliationRecord struct {
	ID               int64
	ProgramID        int64
	TicketCategoryID int64
	Difference       InventoryDifference
	HandleStatus     string
	CreatedAt        time.Time
}
