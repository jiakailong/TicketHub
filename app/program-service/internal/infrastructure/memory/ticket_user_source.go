package memory

import "context"

// TicketUserSource keeps memory-mode development self-contained while still
// enforcing positive, distinct ticket-user IDs in PurchaseStructureRule.
type TicketUserSource struct{}

func (TicketUserSource) ListTicketUserIDs(_ context.Context, _ int64) ([]int64, error) {
	// The memory adapter uses deterministic IDs in examples and tests.
	return []int64{20, 21, 1001, 1002, 1003, 1004, 1005, 1006}, nil
}
