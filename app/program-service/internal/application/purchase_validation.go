package application

import (
	"context"
	"strings"

	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

const DefaultMaxTicketsPerOrder = 6

type PurchaseValidator interface {
	Validate(ctx context.Context, cmd program.CreateOrderCommand) error
}

type PurchaseValidationPipeline struct {
	rules []PurchaseValidator
}

func NewPurchaseValidationPipeline(rules ...PurchaseValidator) PurchaseValidationPipeline {
	return PurchaseValidationPipeline{rules: append([]PurchaseValidator(nil), rules...)}
}

func (p PurchaseValidationPipeline) Validate(ctx context.Context, cmd program.CreateOrderCommand) error {
	for _, rule := range p.rules {
		if rule == nil {
			continue
		}
		if err := rule.Validate(ctx, cmd); err != nil {
			return err
		}
	}
	return nil
}

type PurchaseStructureRule struct {
	maxTickets int
}

func NewPurchaseStructureRule(maxTickets int) PurchaseStructureRule {
	if maxTickets <= 0 {
		maxTickets = DefaultMaxTicketsPerOrder
	}
	return PurchaseStructureRule{maxTickets: maxTickets}
}

func (r PurchaseStructureRule) Validate(_ context.Context, cmd program.CreateOrderCommand) error {
	if cmd.UserID <= 0 || cmd.ProgramID <= 0 || cmd.TicketCategoryID <= 0 {
		return therrors.New(therrors.CodeInvalidArgument, "user_id, program_id and ticket_category_id are required")
	}
	count := cmd.TicketCount()
	if count <= 0 {
		return therrors.New(therrors.CodeInvalidArgument, "ticket user is required")
	}
	if count > int64(r.maxTickets) {
		return therrors.New(therrors.CodeInvalidArgument, "ticket count exceeds per-order limit")
	}
	if len(cmd.TicketUserIDs) != int(count) {
		return therrors.New(therrors.CodeInvalidArgument, "ticket user count must equal ticket count")
	}
	if duplicateInt64(cmd.TicketUserIDs) {
		return therrors.New(therrors.CodeDuplicateSubmission, "ticket user is duplicated")
	}
	if duplicateInt64(cmd.SeatIDs) {
		return therrors.New(therrors.CodeDuplicateSubmission, "seat is duplicated")
	}
	return nil
}

type PurchaseCatalog interface {
	FindProgram(ctx context.Context, programID int64) (program.Program, error)
	ListTicketCategories(ctx context.Context, programID int64) ([]program.TicketCategory, error)
	ListSeats(ctx context.Context, programID int64, ticketCategoryID int64) ([]program.Seat, error)
}

type PurchaseCatalogRule struct {
	catalog PurchaseCatalog
}

func NewPurchaseCatalogRule(catalog PurchaseCatalog) PurchaseCatalogRule {
	return PurchaseCatalogRule{catalog: catalog}
}

func (r PurchaseCatalogRule) Validate(ctx context.Context, cmd program.CreateOrderCommand) error {
	current, err := r.catalog.FindProgram(ctx, cmd.ProgramID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(current.Status, "ON_SALE") {
		return therrors.New(therrors.CodeConflict, "program is not on sale")
	}
	categories, err := r.catalog.ListTicketCategories(ctx, cmd.ProgramID)
	if err != nil {
		return err
	}
	foundCategory := false
	for _, category := range categories {
		if category.ID == cmd.TicketCategoryID && category.ProgramID == cmd.ProgramID {
			foundCategory = true
			if !category.SellStarted {
				return therrors.New(therrors.CodeConflict, "ticket category is not on sale")
			}
			break
		}
	}
	if !foundCategory {
		return therrors.New(therrors.CodeNotFound, "ticket category not found")
	}
	if len(cmd.SeatIDs) == 0 {
		return nil
	}
	seats, err := r.catalog.ListSeats(ctx, cmd.ProgramID, cmd.TicketCategoryID)
	if err != nil {
		return err
	}
	available := make(map[int64]struct{}, len(seats))
	for _, seat := range seats {
		if seat.ProgramID == cmd.ProgramID && seat.TicketCategoryID == cmd.TicketCategoryID && seat.Status == program.SeatNoSold {
			available[seat.ID] = struct{}{}
		}
	}
	for _, seatID := range cmd.SeatIDs {
		if _, ok := available[seatID]; !ok {
			return therrors.New(therrors.CodeSeatUnavailable, "seat does not belong to the selected program and category or is unavailable")
		}
	}
	return nil
}

type TicketUserOwnershipSource interface {
	ListTicketUserIDs(ctx context.Context, userID int64) ([]int64, error)
}

type TicketUserOwnershipRule struct {
	source TicketUserOwnershipSource
}

func NewTicketUserOwnershipRule(source TicketUserOwnershipSource) TicketUserOwnershipRule {
	return TicketUserOwnershipRule{source: source}
}

func (r TicketUserOwnershipRule) Validate(ctx context.Context, cmd program.CreateOrderCommand) error {
	if r.source == nil {
		return therrors.New(therrors.CodeInfrastructure, "ticket user ownership source is not configured")
	}
	ownedIDs, err := r.source.ListTicketUserIDs(ctx, cmd.UserID)
	if err != nil {
		return err
	}
	owned := make(map[int64]struct{}, len(ownedIDs))
	for _, id := range ownedIDs {
		owned[id] = struct{}{}
	}
	for _, id := range cmd.TicketUserIDs {
		if _, ok := owned[id]; !ok {
			return therrors.New(therrors.CodeForbidden, "ticket user does not belong to authenticated user")
		}
	}
	return nil
}

func duplicateInt64(values []int64) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return true
		}
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
