package rpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	userv1 "tickethub/api/proto/user/v1"
)

type TicketUserClient struct {
	client userv1.UserServiceClient
	conn   *grpc.ClientConn
}

func NewTicketUserClient(addr string) (TicketUserClient, error) {
	if strings.TrimSpace(addr) == "" {
		return TicketUserClient{}, nil
	}
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return TicketUserClient{}, err
	}
	return TicketUserClient{client: userv1.NewUserServiceClient(conn), conn: conn}, nil
}

func (c TicketUserClient) ListTicketUserIDs(ctx context.Context, userID int64) ([]int64, error) {
	reply, err := c.client.ListTicketUsers(ctx, &userv1.ListTicketUsersRequest{UserId: userID})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(reply.GetTicketUsers()))
	for _, item := range reply.GetTicketUsers() {
		ids = append(ids, item.GetId())
	}
	return ids, nil
}

func (c TicketUserClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}
