package rpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderv1 "tickethub/api/proto/order/v1"
	"tickethub/app/admin-service/internal/application"
	therrors "tickethub/pkg/errors"
)

type OrderReconciliationClient struct {
	conn   *grpc.ClientConn
	client orderv1.OrderServiceClient
}

func NewOrderReconciliationClient(addr string) (OrderReconciliationClient, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return OrderReconciliationClient{}, therrors.New(therrors.CodeInvalidArgument, "order-service grpc address is required")
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return OrderReconciliationClient{}, err
	}
	return OrderReconciliationClient{conn: conn, client: orderv1.NewOrderServiceClient(conn)}, nil
}

func (c OrderReconciliationClient) Run(ctx context.Context, programID int64, repairInventory bool) (application.ReconciliationResult, error) {
	reply, err := c.client.ReconcileProgram(ctx, &orderv1.ReconcileProgramRequest{ProgramId: programID, RepairInventory: repairInventory})
	if err != nil {
		return application.ReconciliationResult{}, err
	}
	result := application.ReconciliationResult{
		ProgramID:              programID,
		MismatchCount:          reply.GetMismatchCount(),
		RecordMismatchCount:    reply.GetRecordMismatchCount(),
		InventoryMismatchCount: reply.GetInventoryMismatchCount(),
		RepairedInventoryCount: reply.GetRepairedInventoryCount(),
		InventoryDifferences:   make([]application.InventoryDifference, 0, len(reply.GetInventoryDifferences())),
	}
	for _, difference := range reply.GetInventoryDifferences() {
		result.InventoryDifferences = append(result.InventoryDifferences, application.InventoryDifference{
			TicketCategoryID: difference.GetTicketCategoryId(),
			Total:            difference.GetTotal(),
			OccupiedCount:    difference.GetOccupiedCount(),
			ExpectedRemain:   difference.GetExpectedRemain(),
			MySQLRemain:      difference.GetMysqlRemain(),
			RedisRemain:      difference.GetRedisRemain(),
			RedisExists:      difference.GetRedisExists(),
			Repaired:         difference.GetRepaired(),
			Reason:           difference.GetReason(),
		})
	}
	return result, nil
}

func (c OrderReconciliationClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
