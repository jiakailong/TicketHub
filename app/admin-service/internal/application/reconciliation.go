package application

import "context"

type ReconciliationResult struct {
	ProgramID              int64                 `json:"program_id"`
	MismatchCount          int64                 `json:"mismatch_count"`
	RecordMismatchCount    int64                 `json:"record_mismatch_count"`
	InventoryMismatchCount int64                 `json:"inventory_mismatch_count"`
	RepairedInventoryCount int64                 `json:"repaired_inventory_count"`
	InventoryDifferences   []InventoryDifference `json:"inventory_differences"`
}

type InventoryDifference struct {
	TicketCategoryID int64  `json:"ticket_category_id"`
	Total            int64  `json:"total"`
	OccupiedCount    int64  `json:"occupied_count"`
	ExpectedRemain   int64  `json:"expected_remain"`
	MySQLRemain      int64  `json:"mysql_remain"`
	RedisRemain      int64  `json:"redis_remain"`
	RedisExists      bool   `json:"redis_exists"`
	Repaired         bool   `json:"repaired"`
	Reason           string `json:"reason"`
}

type ReconciliationCommand interface {
	Run(ctx context.Context, programID int64, repairInventory bool) (ReconciliationResult, error)
}
