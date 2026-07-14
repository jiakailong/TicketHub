package rpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"

	programv1 "tickethub/api/proto/program/v1"
	orderapp "tickethub/app/order-service/internal/application"
)

func TestProgramGRPCClientConfirmSeatsSold(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeProgramService{}
	programv1.RegisterProgramServiceServer(server, fake)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	client, err := NewProgramGRPCClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.ConfirmSeatsSold(context.Background(), 1001, 2001, 3001, []int64{4001, 4002}, []int64{5001, 5002}); err != nil {
		t.Fatal(err)
	}
	if fake.confirmOrderNumber != 1001 || fake.confirmProgramID != 2001 || fake.confirmTicketCategoryID != 3001 || len(fake.confirmSeatIDs) != 2 {
		t.Fatalf("confirm got order=%d program=%d category=%d seats=%v", fake.confirmOrderNumber, fake.confirmProgramID, fake.confirmTicketCategoryID, fake.confirmSeatIDs)
	}
}

func TestProgramGRPCClientReleaseSeats(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeProgramService{}
	programv1.RegisterProgramServiceServer(server, fake)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	client, err := NewProgramGRPCClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.ReleaseSeats(context.Background(), 1001, 2001, 3001, []int64{4001, 4002}, []int64{5001, 5002}, 2); err != nil {
		t.Fatal(err)
	}
	if fake.releaseOrderNumber != 1001 || fake.releaseProgramID != 2001 || fake.releaseTicketCategoryID != 3001 || fake.releaseCount != 2 {
		t.Fatalf("release got order=%d program=%d category=%d count=%d", fake.releaseOrderNumber, fake.releaseProgramID, fake.releaseTicketCategoryID, fake.releaseCount)
	}
	if len(fake.releaseSeatIDs) != 2 {
		t.Fatalf("release seat ids = %v", fake.releaseSeatIDs)
	}
}

func TestProgramGRPCClientReconcileInventory(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeProgramService{}
	programv1.RegisterProgramServiceServer(server, fake)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	client, err := NewProgramGRPCClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	differences, repaired, err := client.ReconcileInventory(context.Background(), 10001, []orderapp.InventoryUsage{{TicketCategoryID: 10, OccupiedCount: 5}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if fake.reconcileProgramID != 10001 || !fake.reconcileRepair || len(fake.reconcileUsages) != 1 || fake.reconcileUsages[0].GetOccupiedCount() != 5 {
		t.Fatalf("request: program=%d repair=%t usages=%+v", fake.reconcileProgramID, fake.reconcileRepair, fake.reconcileUsages)
	}
	if repaired != 1 || len(differences) != 1 || differences[0].TicketCategoryID != 10 || differences[0].Reason != "REDIS_REMAIN_MISMATCH" {
		t.Fatalf("reply: repaired=%d differences=%+v", repaired, differences)
	}
}

type fakeProgramService struct {
	programv1.UnimplementedProgramServiceServer
	confirmOrderNumber      int64
	confirmProgramID        int64
	confirmTicketCategoryID int64
	confirmSeatIDs          []int64
	releaseOrderNumber      int64
	releaseProgramID        int64
	releaseTicketCategoryID int64
	releaseSeatIDs          []int64
	releaseCount            int64
	reconcileProgramID      int64
	reconcileUsages         []*programv1.TicketCategoryUsage
	reconcileRepair         bool
}

func (s *fakeProgramService) ReconcileInventory(ctx context.Context, req *programv1.ReconcileInventoryRequest) (*programv1.ReconcileInventoryReply, error) {
	s.reconcileProgramID = req.GetProgramId()
	s.reconcileUsages = append([]*programv1.TicketCategoryUsage(nil), req.GetUsages()...)
	s.reconcileRepair = req.GetRepair()
	return &programv1.ReconcileInventoryReply{
		MismatchCount: 1,
		RepairedCount: 1,
		Differences: []*programv1.InventoryDifference{{
			TicketCategoryId: 10,
			Total:            100,
			OccupiedCount:    5,
			ExpectedRemain:   95,
			MysqlRemain:      95,
			RedisRemain:      90,
			RedisExists:      true,
			Repaired:         true,
			Reason:           "REDIS_REMAIN_MISMATCH",
		}},
	}, nil
}

func (s *fakeProgramService) ConfirmSeatsSold(ctx context.Context, req *programv1.SeatOperationRequest) (*programv1.SeatOperationReply, error) {
	s.confirmOrderNumber = req.GetOrderNumber()
	s.confirmProgramID = req.GetProgramId()
	s.confirmTicketCategoryID = req.GetTicketCategoryId()
	s.confirmSeatIDs = append([]int64(nil), req.GetSeatIds()...)
	return &programv1.SeatOperationReply{Success: true}, nil
}

func (s *fakeProgramService) ReleaseSeats(ctx context.Context, req *programv1.SeatOperationRequest) (*programv1.SeatOperationReply, error) {
	s.releaseOrderNumber = req.GetOrderNumber()
	s.releaseProgramID = req.GetProgramId()
	s.releaseTicketCategoryID = req.GetTicketCategoryId()
	s.releaseSeatIDs = append([]int64(nil), req.GetSeatIds()...)
	s.releaseCount = req.GetCount()
	return &programv1.SeatOperationReply{Success: true}, nil
}
