package application

import (
	"context"
	"testing"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

func TestProgramAdminServiceWarmsInventoryBeforeGoingOnSale(t *testing.T) {
	repository := &fakeProgramAdminRepository{
		item:       program.Program{ID: 10, Title: "Live", City: "Shanghai", Place: "Arena", ShowTime: time.Now().Add(time.Hour), Status: "READY"},
		categories: []program.TicketCategory{{ID: 20, ProgramID: 10, Name: "A", PriceCent: 100, Total: 50, Remain: 45}},
	}
	inventory := &fakeInventoryInitializer{}
	service := NewProgramAdminService(repository, inventory, &fakeReconciliationIDs{})

	if err := service.ChangeStatus(context.Background(), 10, "ON_SALE"); err != nil {
		t.Fatal(err)
	}
	if inventory.calls != 1 || repository.saved.Status != "ON_SALE" || !repository.savedCategories[0].SellStarted {
		t.Fatalf("inventory=%+v saved=%+v categories=%+v", inventory, repository.saved, repository.savedCategories)
	}
	if repository.event.Topic != ProgramChangedTopic {
		t.Fatalf("event = %+v", repository.event)
	}
}

func TestProgramAdminServiceRejectsInvalidTransition(t *testing.T) {
	repository := &fakeProgramAdminRepository{item: program.Program{ID: 10, Status: "DRAFT"}}
	service := NewProgramAdminService(repository, &fakeInventoryInitializer{}, &fakeReconciliationIDs{})
	if err := service.ChangeStatus(context.Background(), 10, "ON_SALE"); !therrors.IsCode(err, therrors.CodeConflict) {
		t.Fatalf("error = %v", err)
	}
}

type fakeProgramAdminRepository struct {
	item            program.Program
	categories      []program.TicketCategory
	saved           program.Program
	savedCategories []program.TicketCategory
	event           mq.Event
}

func (r *fakeProgramAdminRepository) FindProgram(ctx context.Context, programID int64) (program.Program, error) {
	return r.item, nil
}

func (r *fakeProgramAdminRepository) ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error) {
	return append([]program.TicketCategory(nil), r.categories...), nil
}

func (r *fakeProgramAdminRepository) SaveProgramWithEvent(ctx context.Context, item program.Program, categories []program.TicketCategory, event mq.Event) error {
	r.saved = item
	r.savedCategories = append([]program.TicketCategory(nil), categories...)
	r.event = event
	return nil
}

type fakeInventoryInitializer struct{ calls int }

func (i *fakeInventoryInitializer) InitializeRemain(ctx context.Context, programID int64, ticketCategoryID int64, remain int64) (bool, error) {
	i.calls++
	return true, nil
}
