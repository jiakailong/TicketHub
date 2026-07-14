package grpcapi

import (
	"context"
	"time"

	orderv1 "tickethub/api/proto/order/v1"
	orderapp "tickethub/app/order-service/internal/application"
	"tickethub/app/order-service/internal/domain/order"
)

type Server struct {
	orderv1.UnimplementedOrderServiceServer
	orders    order.Repository
	commands  orderapp.OrderCommandService
	queries   orderapp.OrderQueryService
	reconcile orderapp.ReconciliationService
}

func NewServer(orders order.Repository, commands orderapp.OrderCommandService, queries orderapp.OrderQueryService, reconcile orderapp.ReconciliationService) Server {
	return Server{orders: orders, commands: commands, queries: queries, reconcile: reconcile}
}

func (s Server) CreateOrder(ctx context.Context, req *orderv1.CreateOrderRequest) (*orderv1.CreateOrderReply, error) {
	created := order.New(req.GetOrderNumber(), req.GetProgramId(), req.GetUserId(), req.GetAmountCent(), time.Now())
	if err := s.orders.Save(ctx, created); err != nil {
		return nil, err
	}
	return &orderv1.CreateOrderReply{OrderNumber: created.OrderNumber}, nil
}

func (s Server) GetOrder(ctx context.Context, req *orderv1.GetOrderRequest) (*orderv1.OrderReply, error) {
	current, err := s.queries.Get(ctx, req.GetOrderNumber(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	return orderReply(current), nil
}

func (s Server) ListOrders(ctx context.Context, req *orderv1.ListOrdersRequest) (*orderv1.ListOrdersReply, error) {
	page, err := s.queries.ListPage(ctx, req.GetUserId(), req.GetStatus(), req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	reply := &orderv1.ListOrdersReply{Orders: make([]*orderv1.OrderReply, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		reply.Orders = append(reply.Orders, orderReply(item))
	}
	return reply, nil
}

func (s Server) CancelOrder(ctx context.Context, req *orderv1.CancelOrderRequest) (*orderv1.OrderReply, error) {
	if err := s.commands.Cancel(ctx, req.GetOrderNumber(), req.GetUserId()); err != nil {
		return nil, err
	}
	current, err := s.queries.Get(ctx, req.GetOrderNumber(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	return orderReply(current), nil
}

func (s Server) GetOrderPayment(ctx context.Context, req *orderv1.GetOrderPaymentRequest) (*orderv1.OrderPaymentReply, error) {
	current, err := s.queries.Get(ctx, req.GetOrderNumber(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &orderv1.OrderPaymentReply{Status: string(current.Status), AmountCent: current.AmountCent}, nil
}

func (s Server) HandlePaymentCallback(ctx context.Context, req *orderv1.PaymentCallbackRequest) (*orderv1.PaymentCallbackReply, error) {
	result, err := s.commands.HandlePaymentCallback(ctx, req.GetOrderNumber(), req.GetAmountCent(), req.GetPaid())
	if err != nil {
		return nil, err
	}
	return &orderv1.PaymentCallbackReply{
		Status:         string(result.Status),
		RefundRequired: result.RefundRequired,
		AmountCent:     result.AmountCent,
	}, nil
}

func (s Server) MarkOrderRefunded(ctx context.Context, req *orderv1.MarkOrderRefundedRequest) (*orderv1.OrderReply, error) {
	current, err := s.commands.MarkRefunded(ctx, req.GetOrderNumber())
	if err != nil {
		return nil, err
	}
	return orderReply(current), nil
}

func (s Server) ReconcileProgram(ctx context.Context, req *orderv1.ReconcileProgramRequest) (*orderv1.ReconcileProgramReply, error) {
	result, err := s.reconcile.ReconcileProgram(ctx, req.GetProgramId(), req.GetRepairInventory())
	if err != nil {
		return nil, err
	}
	reply := &orderv1.ReconcileProgramReply{
		MismatchCount:          result.MismatchCount,
		RecordMismatchCount:    result.RecordMismatchCount,
		InventoryMismatchCount: result.InventoryMismatchCount,
		RepairedInventoryCount: result.RepairedInventoryCount,
		InventoryDifferences:   make([]*orderv1.InventoryDifference, 0, len(result.InventoryDifferences)),
	}
	for _, difference := range result.InventoryDifferences {
		reply.InventoryDifferences = append(reply.InventoryDifferences, &orderv1.InventoryDifference{
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

func orderReply(current order.Order) *orderv1.OrderReply {
	reply := &orderv1.OrderReply{
		OrderNumber:      current.OrderNumber,
		Status:           string(current.Status),
		AmountCent:       current.AmountCent,
		ProgramId:        current.ProgramID,
		TicketCategoryId: current.TicketCategoryID,
		SeatIds:          append([]int64(nil), current.SeatIDs...),
		TicketUserIds:    append([]int64(nil), current.TicketUserIDs...),
		CreatedAt:        current.CreatedAt.Format(time.RFC3339),
	}
	if current.PaidAt != nil {
		reply.PaidAt = current.PaidAt.Format(time.RFC3339)
	}
	if current.CanceledAt != nil {
		reply.CanceledAt = current.CanceledAt.Format(time.RFC3339)
	}
	if current.RefundedAt != nil {
		reply.RefundedAt = current.RefundedAt.Format(time.RFC3339)
	}
	return reply
}
