package rpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	programv1 "tickethub/api/proto/program/v1"
	orderapp "tickethub/app/order-service/internal/application"
)

type ProgramClient struct {
	client programv1.ProgramServiceClient
	conn   *grpc.ClientConn
}

func (c ProgramClient) ReconcileInventory(ctx context.Context, programID int64, usages []orderapp.InventoryUsage, repair bool) ([]orderapp.InventoryDifference, int64, error) {
	if c.client == nil {
		return nil, 0, nil
	}
	request := &programv1.ReconcileInventoryRequest{
		ProgramId: programID,
		Repair:    repair,
		Usages:    make([]*programv1.TicketCategoryUsage, 0, len(usages)),
	}
	for _, usage := range usages {
		request.Usages = append(request.Usages, &programv1.TicketCategoryUsage{
			TicketCategoryId: usage.TicketCategoryID,
			OccupiedCount:    usage.OccupiedCount,
		})
	}
	reply, err := c.client.ReconcileInventory(ctx, request)
	if err != nil {
		return nil, 0, err
	}
	differences := make([]orderapp.InventoryDifference, 0, len(reply.GetDifferences()))
	for _, difference := range reply.GetDifferences() {
		differences = append(differences, orderapp.InventoryDifference{
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
	return differences, reply.GetRepairedCount(), nil
}

func NewProgramClient() ProgramClient {
	return ProgramClient{}
}

func NewProgramGRPCClient(addr string) (ProgramClient, error) {
	if strings.TrimSpace(addr) == "" {
		return ProgramClient{}, nil
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return ProgramClient{}, err
	}
	return ProgramClient{client: programv1.NewProgramServiceClient(conn), conn: conn}, nil
}

func (c ProgramClient) ConfirmSeatsSold(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64) error {
	if c.client == nil {
		return nil
	}
	_, err := c.client.ConfirmSeatsSold(ctx, &programv1.SeatOperationRequest{
		ProgramId:        programID,
		OrderNumber:      orderNumber,
		TicketCategoryId: ticketCategoryID,
		SeatIds:          seatIDs,
		TicketUserIds:    ticketUserIDs,
	})
	return err
}

func (c ProgramClient) ReleaseSeats(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	if c.client == nil {
		return nil
	}
	_, err := c.client.ReleaseSeats(ctx, &programv1.SeatOperationRequest{
		ProgramId:        programID,
		OrderNumber:      orderNumber,
		TicketCategoryId: ticketCategoryID,
		SeatIds:          seatIDs,
		TicketUserIds:    ticketUserIDs,
		Count:            count,
	})
	return err
}

func (c ProgramClient) RollbackCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	if c.client == nil {
		return nil
	}
	_, err := c.client.RollbackCreateOrder(ctx, &programv1.SeatOperationRequest{
		ProgramId:        programID,
		OrderNumber:      orderNumber,
		TicketCategoryId: ticketCategoryID,
		SeatIds:          seatIDs,
		TicketUserIds:    ticketUserIDs,
		Count:            count,
	})
	return err
}

func (c ProgramClient) ReserveCreateOrder(ctx context.Context, orderNumber int64, programID int64, ticketCategoryID int64, seatIDs []int64, ticketUserIDs []int64, count int64) error {
	if c.client == nil {
		return nil
	}
	_, err := c.client.ReserveCreateOrder(ctx, &programv1.SeatOperationRequest{
		ProgramId:        programID,
		OrderNumber:      orderNumber,
		TicketCategoryId: ticketCategoryID,
		SeatIds:          seatIDs,
		TicketUserIds:    ticketUserIDs,
		Count:            count,
	})
	return err
}

func (c ProgramClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
