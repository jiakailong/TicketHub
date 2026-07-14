package application_test

import (
	"context"
	"testing"

	"tickethub/app/program-service/internal/application"
	"tickethub/app/program-service/internal/domain/program"
	"tickethub/app/program-service/internal/infrastructure/memory"
	therrors "tickethub/pkg/errors"
)

func TestProgramPricingServiceCalculatesAuthoritativeAmount(t *testing.T) {
	service := application.NewProgramPricingService(memory.NewProgramRepository())
	amount, err := service.CalculateAmount(context.Background(), program.CreateOrderCommand{
		ProgramID:        10001,
		TicketCategoryID: 1,
		SeatIDs:          []int64{100, 101},
	})
	if err != nil {
		t.Fatal(err)
	}
	if amount != 256000 {
		t.Fatalf("amount = %d", amount)
	}
}

func TestProgramPricingServiceRejectsUnknownCategory(t *testing.T) {
	service := application.NewProgramPricingService(memory.NewProgramRepository())
	_, err := service.CalculateAmount(context.Background(), program.CreateOrderCommand{
		ProgramID:        10001,
		TicketCategoryID: 999,
		SeatIDs:          []int64{100},
	})
	if !therrors.IsCode(err, therrors.CodeNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
