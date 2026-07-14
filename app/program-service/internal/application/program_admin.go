package application

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/mq"
)

const ProgramChangedTopic = "ticket_hub.program_changed"

type ProgramAdminRepository interface {
	FindProgram(ctx context.Context, programID int64) (program.Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	SaveProgramWithEvent(ctx context.Context, item program.Program, categories []program.TicketCategory, event mq.Event) error
}

type ProgramAdminService struct {
	repository ProgramAdminRepository
	inventory  InventoryInitializer
	ids        ReconciliationIDGenerator
	nowFunc    func() time.Time
}

func NewProgramAdminService(repository ProgramAdminRepository, inventory InventoryInitializer, ids ReconciliationIDGenerator) ProgramAdminService {
	return ProgramAdminService{repository: repository, inventory: inventory, ids: ids, nowFunc: time.Now}
}

func (s ProgramAdminService) SaveDraft(ctx context.Context, item program.Program, categories []program.TicketCategory) error {
	if item.ID <= 0 || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.City) == "" || strings.TrimSpace(item.Place) == "" || item.ShowTime.IsZero() {
		return therrors.New(therrors.CodeInvalidArgument, "complete program information is required")
	}
	if current, err := s.repository.FindProgram(ctx, item.ID); err == nil && current.Status == "ON_SALE" {
		return therrors.New(therrors.CodeConflict, "on-sale program cannot be edited as draft")
	} else if err != nil && !therrors.IsCode(err, therrors.CodeNotFound) {
		return err
	}
	if len(categories) == 0 {
		return therrors.New(therrors.CodeInvalidArgument, "at least one ticket category is required")
	}
	for index := range categories {
		if categories[index].ID <= 0 || categories[index].PriceCent <= 0 || categories[index].Total <= 0 {
			return therrors.New(therrors.CodeInvalidArgument, "ticket category id, price and total must be positive")
		}
		categories[index].ProgramID = item.ID
		categories[index].Remain = categories[index].Total
		categories[index].SellStarted = false
	}
	item.Status = "DRAFT"
	event, err := s.changeEvent(item, "DELETE")
	if err != nil {
		return err
	}
	return s.repository.SaveProgramWithEvent(ctx, item, categories, event)
}

func (s ProgramAdminService) ChangeStatus(ctx context.Context, programID int64, target string) error {
	target = strings.ToUpper(strings.TrimSpace(target))
	current, err := s.repository.FindProgram(ctx, programID)
	if err != nil {
		return err
	}
	if current.Status == target {
		return nil
	}
	if !validProgramTransition(current.Status, target) {
		return therrors.New(therrors.CodeConflict, "invalid program status transition")
	}
	categories, err := s.repository.ListTicketCategories(ctx, programID)
	if err != nil {
		return err
	}
	if target == "ON_SALE" {
		if !current.ShowTime.After(s.nowFunc()) {
			return therrors.New(therrors.CodeConflict, "past program cannot go on sale")
		}
		for index := range categories {
			if _, err := s.inventory.InitializeRemain(ctx, programID, categories[index].ID, categories[index].Remain); err != nil {
				return err
			}
			categories[index].SellStarted = true
		}
	}
	if target == "OFF_SALE" {
		for index := range categories {
			categories[index].SellStarted = false
		}
	}
	current.Status = target
	operation := "DELETE"
	if target == "ON_SALE" {
		operation = "UPSERT"
	}
	event, err := s.changeEvent(current, operation)
	if err != nil {
		return err
	}
	return s.repository.SaveProgramWithEvent(ctx, current, categories, event)
}

func (s ProgramAdminService) changeEvent(item program.Program, operation string) (mq.Event, error) {
	eventID, err := s.ids.NextID()
	if err != nil {
		return mq.Event{}, err
	}
	now := s.nowFunc()
	payload, err := json.Marshal(mq.ProgramChangedEvent{
		ProgramID: item.ID, Title: item.Title, City: item.City, Place: item.Place,
		ShowTime: item.ShowTime, Status: item.Status, Operation: operation,
	})
	if err != nil {
		return mq.Event{}, err
	}
	return mq.Event{
		Topic: ProgramChangedTopic, Key: strconv.FormatInt(item.ID, 10), Payload: payload,
		Header: mq.Header{EventID: strconv.FormatInt(eventID, 10), SchemaVersion: "v1", OccurredAt: now},
	}, nil
}

func validProgramTransition(current string, target string) bool {
	switch strings.ToUpper(current) {
	case "DRAFT":
		return target == "READY"
	case "READY", "COMING_SOON":
		return target == "ON_SALE"
	case "ON_SALE":
		return target == "OFF_SALE"
	default:
		return false
	}
}
