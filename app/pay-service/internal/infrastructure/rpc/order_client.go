package rpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderv1 "tickethub/api/proto/order/v1"
	"tickethub/app/pay-service/internal/application"
	therrors "tickethub/pkg/errors"
)

type OrderPaymentClient struct {
	conn   *grpc.ClientConn
	client orderv1.OrderServiceClient
}

func NewOrderPaymentClient(addr string) (OrderPaymentClient, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return OrderPaymentClient{}, therrors.New(therrors.CodeInvalidArgument, "order-service grpc address is required")
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return OrderPaymentClient{}, err
	}
	return OrderPaymentClient{conn: conn, client: orderv1.NewOrderServiceClient(conn)}, nil
}

func (c OrderPaymentClient) GetOrderPayment(ctx context.Context, orderNumber int64, userID int64) (application.OrderPaymentResult, error) {
	reply, err := c.client.GetOrderPayment(ctx, &orderv1.GetOrderPaymentRequest{OrderNumber: orderNumber, UserId: userID})
	if err != nil {
		return application.OrderPaymentResult{}, err
	}
	return application.OrderPaymentResult{Status: reply.GetStatus(), AmountCent: reply.GetAmountCent()}, nil
}

func (c OrderPaymentClient) HandlePaymentCallback(ctx context.Context, orderNumber int64, amountCent int64, paid bool) (application.OrderPaymentResult, error) {
	reply, err := c.client.HandlePaymentCallback(ctx, &orderv1.PaymentCallbackRequest{
		OrderNumber: orderNumber,
		AmountCent:  amountCent,
		Paid:        paid,
	})
	if err != nil {
		return application.OrderPaymentResult{}, err
	}
	return application.OrderPaymentResult{
		Status:         reply.GetStatus(),
		RefundRequired: reply.GetRefundRequired(),
		AmountCent:     reply.GetAmountCent(),
	}, nil
}

func (c OrderPaymentClient) MarkOrderRefunded(ctx context.Context, orderNumber int64) error {
	_, err := c.client.MarkOrderRefunded(ctx, &orderv1.MarkOrderRefundedRequest{OrderNumber: orderNumber})
	return err
}

func (c OrderPaymentClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
