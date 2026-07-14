package application_test

import (
	"context"
	"testing"

	"tickethub/app/program-service/internal/application"
	"tickethub/app/program-service/internal/infrastructure/memory"
)

func TestProgramQueryServiceSearchAndDetail(t *testing.T) {
	service := application.NewProgramQueryService(memory.NewProgramRepository())

	programs, err := service.Search(context.Background(), "Live", "Shanghai", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(programs) != 1 || programs[0].ID != 10001 {
		t.Fatalf("programs = %+v", programs)
	}

	detail, err := service.Detail(context.Background(), 10001, 1)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Program.Title == "" || len(detail.TicketCategories) == 0 || len(detail.Seats) == 0 {
		t.Fatalf("detail = %+v", detail)
	}
}
