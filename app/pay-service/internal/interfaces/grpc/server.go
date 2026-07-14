package grpcapi

import (
	"context"

	payv1 "tickethub/api/proto/pay/v1"
	"tickethub/app/pay-service/internal/application"
)

type Server struct {
	payv1.UnimplementedPayServiceServer
	payments application.PayUsecase
}

func NewServer(payments application.PayUsecase) Server {
	return Server{payments: payments}
}

func (s Server) CommonPay(ctx context.Context, req *payv1.CommonPayRequest) (*payv1.CommonPayReply, error) {
	payURL, err := s.payments.CommonPay(ctx, req.GetOrderNumber(), req.GetUserId(), req.GetAmountCent(), req.GetChannel())
	if err != nil {
		return nil, err
	}
	return &payv1.CommonPayReply{PayUrl: payURL}, nil
}

func (s Server) TradeCheck(ctx context.Context, req *payv1.TradeCheckRequest) (*payv1.TradeCheckReply, error) {
	trade, err := s.payments.TradeCheck(ctx, req.GetOrderNumber(), req.GetUserId(), req.GetChannel())
	if err != nil {
		return nil, err
	}
	return &payv1.TradeCheckReply{Paid: trade.Paid}, nil
}

func (s Server) Refund(ctx context.Context, req *payv1.RefundRequest) (*payv1.RefundReply, error) {
	if err := s.payments.Refund(ctx, req.GetOrderNumber(), req.GetAmountCent(), req.GetReason()); err != nil {
		return nil, err
	}
	return &payv1.RefundReply{Success: true}, nil
}
