package grpcapi

import (
	"context"

	adminv1 "tickethub/api/proto/admin/v1"
	"tickethub/app/admin-service/internal/application"
)

type Server struct {
	adminv1.UnimplementedAdminServiceServer
	query   application.DashboardQuery
	command application.ReconciliationCommand
}

func NewServer(query application.DashboardQuery, command application.ReconciliationCommand) Server {
	return Server{query: query, command: command}
}

func (s Server) Dashboard(ctx context.Context, req *adminv1.DashboardRequest) (*adminv1.DashboardReply, error) {
	stats, err := s.query.Dashboard(ctx)
	if err != nil {
		return nil, err
	}
	return &adminv1.DashboardReply{
		DiscardOrderCount:           stats.DiscardOrderCount,
		ReconciliationMismatchCount: stats.ReconciliationMismatchCount,
	}, nil
}

func (s Server) RunReconciliation(ctx context.Context, req *adminv1.RunReconciliationRequest) (*adminv1.RunReconciliationReply, error) {
	result, err := s.command.Run(ctx, req.GetProgramId(), req.GetRepairInventory())
	if err != nil {
		return nil, err
	}
	stats, err := s.query.Dashboard(ctx)
	if err != nil {
		return nil, err
	}
	reply := &adminv1.RunReconciliationReply{
		Triggered:              true,
		ProgramId:              result.ProgramID,
		MismatchCount:          result.MismatchCount,
		DiscardOrderCount:      stats.DiscardOrderCount,
		RecordMismatchCount:    result.RecordMismatchCount,
		InventoryMismatchCount: result.InventoryMismatchCount,
		RepairedInventoryCount: result.RepairedInventoryCount,
		InventoryDifferences:   make([]*adminv1.InventoryDifference, 0, len(result.InventoryDifferences)),
	}
	for _, difference := range result.InventoryDifferences {
		reply.InventoryDifferences = append(reply.InventoryDifferences, &adminv1.InventoryDifference{
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
