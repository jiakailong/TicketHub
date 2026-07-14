package grpcapi

import (
	"context"
	"time"

	programv1 "tickethub/api/proto/program/v1"
	"tickethub/app/program-service/internal/application"
	"tickethub/app/program-service/internal/domain/program"
	therrors "tickethub/pkg/errors"
)

type SeatStateService interface {
	RollbackCreateOrder(ctx context.Context, programID int64, ticketCategoryID int64, orderNumber int64, seatIDs []int64, ticketUserIDs []int64, count int64) error
	ConfirmSeatsSold(ctx context.Context, orderNumber int64, programID int64, seatIDs []int64, ticketUserIDs []int64) error
	ReleaseSeats(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error
}

type CreateOrderReservationService interface {
	LockSeats(ctx context.Context, cmd program.CreateOrderCommand, orderNumber int64, identifierID int64) ([]program.Seat, error)
}

func (s Server) RollbackCreateOrder(ctx context.Context, req *programv1.SeatOperationRequest) (*programv1.SeatOperationReply, error) {
	if s.seats != nil {
		count := req.GetCount()
		if count <= 0 {
			count = int64(len(req.GetSeatIds()))
		}
		if err := s.seats.RollbackCreateOrder(ctx, req.GetProgramId(), req.GetTicketCategoryId(), req.GetOrderNumber(), req.GetSeatIds(), req.GetTicketUserIds(), count); err != nil {
			return nil, err
		}
	}
	return &programv1.SeatOperationReply{Success: true}, nil
}

type Server struct {
	programv1.UnimplementedProgramServiceServer
	queries      application.ProgramQueryService
	createOrders application.CreateOrderUsecase
	seats        SeatStateService
	reservations CreateOrderReservationService
	reconcile    *application.InventoryReconciliationService
}

func (s Server) WithReservations(reservations CreateOrderReservationService) Server {
	s.reservations = reservations
	return s
}

func (s Server) ReserveCreateOrder(ctx context.Context, req *programv1.SeatOperationRequest) (*programv1.SeatOperationReply, error) {
	if s.reservations == nil {
		return nil, therrors.New(therrors.CodeInfrastructure, "create order reservation service is not configured")
	}
	if _, err := s.reservations.LockSeats(ctx, program.CreateOrderCommand{
		ProgramID:        req.GetProgramId(),
		TicketCategoryID: req.GetTicketCategoryId(),
		SeatIDs:          req.GetSeatIds(),
		TicketUserIDs:    req.GetTicketUserIds(),
	}, req.GetOrderNumber(), req.GetOrderNumber()); err != nil {
		return nil, err
	}
	return &programv1.SeatOperationReply{Success: true}, nil
}

func NewServer(queries application.ProgramQueryService, createOrders application.CreateOrderUsecase, seats SeatStateService, reconciliation ...application.InventoryReconciliationService) Server {
	server := Server{queries: queries, createOrders: createOrders, seats: seats}
	if len(reconciliation) > 0 {
		server.reconcile = &reconciliation[0]
	}
	return server
}

func (s Server) ReconcileInventory(ctx context.Context, req *programv1.ReconcileInventoryRequest) (*programv1.ReconcileInventoryReply, error) {
	if s.reconcile == nil {
		return nil, therrors.New(therrors.CodeInfrastructure, "inventory reconciliation service is not configured")
	}
	usages := make([]program.InventoryUsage, 0, len(req.GetUsages()))
	for _, usage := range req.GetUsages() {
		usages = append(usages, program.InventoryUsage{
			TicketCategoryID: usage.GetTicketCategoryId(),
			OccupiedCount:    usage.GetOccupiedCount(),
		})
	}
	result, err := s.reconcile.Reconcile(ctx, req.GetProgramId(), usages, req.GetRepair())
	if err != nil {
		return nil, err
	}
	reply := &programv1.ReconcileInventoryReply{
		MismatchCount: result.MismatchCount,
		RepairedCount: result.RepairedCount,
		Differences:   make([]*programv1.InventoryDifference, 0, len(result.Differences)),
	}
	for _, difference := range result.Differences {
		reply.Differences = append(reply.Differences, &programv1.InventoryDifference{
			TicketCategoryId: difference.TicketCategoryID,
			Total:            difference.Total,
			OccupiedCount:    difference.OccupiedCount,
			ExpectedRemain:   difference.ExpectedRemain,
			MysqlRemain:      difference.MySQLRemain,
			RedisRemain:      difference.RedisRemain,
			RedisExists:      difference.RedisExists,
			Repaired:         difference.Repaired,
			Reason:           difference.Reason,
		})
	}
	return reply, nil
}

func (s Server) SearchProgram(ctx context.Context, req *programv1.SearchProgramRequest) (*programv1.SearchProgramReply, error) {
	page, err := s.queries.SearchPage(ctx, req.GetKeyword(), req.GetCity(), req.GetCursor(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, err
	}
	reply := &programv1.SearchProgramReply{Programs: make([]*programv1.ProgramSummary, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		reply.Programs = append(reply.Programs, &programv1.ProgramSummary{
			Id:           item.Program.ID,
			Title:        item.Program.Title,
			City:         item.Program.City,
			Place:        item.Program.Place,
			ShowTime:     item.Program.ShowTime.Format(time.RFC3339),
			Status:       item.Program.Status,
			MinPriceCent: item.MinPriceCent,
		})
	}
	return reply, nil
}

func (s Server) GetProgramDetail(ctx context.Context, req *programv1.GetProgramDetailRequest) (*programv1.ProgramDetail, error) {
	detail, err := s.queries.Detail(ctx, req.GetProgramId(), req.GetTicketCategoryId())
	if err != nil {
		return nil, err
	}
	reply := &programv1.ProgramDetail{
		Id:               detail.Program.ID,
		Title:            detail.Program.Title,
		City:             detail.Program.City,
		Place:            detail.Program.Place,
		ShowTime:         detail.Program.ShowTime.Format(time.RFC3339),
		Status:           detail.Program.Status,
		TicketCategories: make([]*programv1.TicketCategory, 0, len(detail.TicketCategories)),
		Seats:            make([]*programv1.Seat, 0, len(detail.Seats)),
	}
	for _, item := range detail.TicketCategories {
		reply.TicketCategories = append(reply.TicketCategories, &programv1.TicketCategory{
			Id:        item.ID,
			Name:      item.Name,
			PriceCent: item.PriceCent,
			Remain:    item.Remain,
		})
	}
	for _, item := range detail.Seats {
		reply.Seats = append(reply.Seats, &programv1.Seat{
			Id:               item.ID,
			TicketCategoryId: item.TicketCategoryID,
			RowCode:          item.RowCode,
			ColCode:          item.ColCode,
			PriceCent:        item.PriceCent,
			Status:           string(item.Status),
		})
	}
	return reply, nil
}

func (s Server) CreateOrder(ctx context.Context, req *programv1.CreateOrderRequest) (*programv1.CreateOrderReply, error) {
	result, err := s.createOrders.CreateAsync(ctx, program.CreateOrderCommand{
		RequestID:        req.GetRequestId(),
		UserID:           req.GetUserId(),
		ProgramID:        req.GetProgramId(),
		TicketCategoryID: req.GetTicketCategoryId(),
		SeatIDs:          req.GetSeatIds(),
		TicketUserIDs:    req.GetTicketUserIds(),
	})
	if err != nil {
		return nil, err
	}
	return &programv1.CreateOrderReply{OrderNumber: result.OrderNumber}, nil
}

func (s Server) ConfirmSeatsSold(ctx context.Context, req *programv1.SeatOperationRequest) (*programv1.SeatOperationReply, error) {
	if s.seats != nil {
		if err := s.seats.ConfirmSeatsSold(ctx, req.GetOrderNumber(), req.GetProgramId(), req.GetSeatIds(), req.GetTicketUserIds()); err != nil {
			return nil, err
		}
	}
	return &programv1.SeatOperationReply{Success: true}, nil
}

func (s Server) ReleaseSeats(ctx context.Context, req *programv1.SeatOperationRequest) (*programv1.SeatOperationReply, error) {
	if s.seats != nil {
		count := req.GetCount()
		if count <= 0 {
			count = int64(len(req.GetSeatIds()))
		}
		if err := s.seats.ReleaseSeats(ctx, req.GetOrderNumber(), req.GetProgramId(), req.GetTicketCategoryId(), req.GetSeatIds(), req.GetTicketUserIds(), count); err != nil {
			return nil, err
		}
	}
	return &programv1.SeatOperationReply{Success: true}, nil
}
