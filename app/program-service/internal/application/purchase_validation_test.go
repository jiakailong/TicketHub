package application_test

import (
	"context"
	"testing"

	"tickethub/app/program-service/internal/application"
	"tickethub/app/program-service/internal/domain/program"
	"tickethub/app/program-service/internal/infrastructure/memory"
	therrors "tickethub/pkg/errors"
)

func TestPurchaseValidationPipelineAcceptsOwnedSeatsAndTicketUsers(t *testing.T) {
	repo := memory.NewProgramRepository()
	pipeline := application.NewPurchaseValidationPipeline(
		application.NewPurchaseStructureRule(6),
		application.NewPurchaseCatalogRule(repo),
		application.NewTicketUserOwnershipRule(fakeTicketUserSource{3001: {20, 21}}),
	)

	err := pipeline.Validate(context.Background(), program.CreateOrderCommand{
		UserID: 3001, ProgramID: 10001, TicketCategoryID: 1,
		SeatIDs: []int64{100, 101}, TicketUserIDs: []int64{20, 21},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPurchaseStructureRuleRejectsMismatchDuplicateAndLimit(t *testing.T) {
	rule := application.NewPurchaseStructureRule(2)
	tests := []program.CreateOrderCommand{
		{UserID: 1, ProgramID: 1, TicketCategoryID: 1, SeatIDs: []int64{1, 2}, TicketUserIDs: []int64{10}},
		{UserID: 1, ProgramID: 1, TicketCategoryID: 1, TicketUserIDs: []int64{10, 10}},
		{UserID: 1, ProgramID: 1, TicketCategoryID: 1, TicketUserIDs: []int64{10, 11, 12}},
	}
	for index, cmd := range tests {
		if err := rule.Validate(context.Background(), cmd); err == nil {
			t.Fatalf("case %d should fail", index)
		}
	}
}

func TestPurchaseCatalogRuleRejectsSeatOutsideCategory(t *testing.T) {
	rule := application.NewPurchaseCatalogRule(memory.NewProgramRepository())
	err := rule.Validate(context.Background(), program.CreateOrderCommand{
		UserID: 1, ProgramID: 10001, TicketCategoryID: 1,
		SeatIDs: []int64{999}, TicketUserIDs: []int64{20},
	})
	if !therrors.IsCode(err, therrors.CodeSeatUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestTicketUserOwnershipRuleRejectsForeignTicketUser(t *testing.T) {
	rule := application.NewTicketUserOwnershipRule(fakeTicketUserSource{3001: {20}})
	err := rule.Validate(context.Background(), program.CreateOrderCommand{UserID: 3001, TicketUserIDs: []int64{21}})
	if !therrors.IsCode(err, therrors.CodeForbidden) {
		t.Fatalf("error = %v", err)
	}
}

type fakeTicketUserSource map[int64][]int64

func (s fakeTicketUserSource) ListTicketUserIDs(_ context.Context, userID int64) ([]int64, error) {
	return append([]int64(nil), s[userID]...), nil
}
