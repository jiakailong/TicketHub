package grpcapi

import (
	"context"
	"testing"

	userv1 "tickethub/api/proto/user/v1"
	"tickethub/app/user-service/internal/application"
	"tickethub/app/user-service/internal/infrastructure/memory"
	"tickethub/app/user-service/internal/infrastructure/security"
	"tickethub/pkg/auth"
)

func TestServerRegisterAndLogin(t *testing.T) {
	users := application.NewUserCommandService(
		&fakeIDs{next: 10},
		memory.NewUserRepository(),
		security.NewBcryptPasswordHasher(4),
	).
		WithTicketUsers(memory.NewTicketUserRepository()).
		WithTokenManager(auth.NewTokenManager("grpc-secret"))
	server := NewServer(users)

	registered, err := server.Register(context.Background(), &userv1.RegisterRequest{
		Mobile:   "13900000000",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.GetUserId() != 11 {
		t.Fatalf("registered = %+v", registered)
	}
	if registered.GetMobile() != "139****0000" {
		t.Fatalf("registered mobile was not masked: %q", registered.GetMobile())
	}

	login, err := server.Login(context.Background(), &userv1.LoginRequest{
		Mobile:   "13900000000",
		Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if login.GetAccessToken() == "" || login.GetUser().GetUserId() != 11 {
		t.Fatalf("login = %+v", login)
	}
	if login.GetUser().GetMobile() != "139****0000" {
		t.Fatalf("login mobile was not masked: %q", login.GetUser().GetMobile())
	}
	ticketUser, err := server.AddTicketUser(context.Background(), &userv1.AddTicketUserRequest{
		UserId:        11,
		Name:          "张三",
		CertificateNo: "310101199001010011",
		Mobile:        "13900000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ticketUser.GetName() != "张*" || ticketUser.GetCertificateNo() != "3101**********0011" {
		t.Fatalf("ticket user was not masked: %+v", ticketUser)
	}
	listed, err := server.ListTicketUsers(context.Background(), &userv1.ListTicketUsersRequest{UserId: 11})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.GetTicketUsers()) != 1 || listed.GetTicketUsers()[0].GetCertificateNo() != "3101**********0011" {
		t.Fatalf("listed ticket users were not masked: %+v", listed)
	}
}

type fakeIDs struct {
	next int64
}

func (g *fakeIDs) NextID() (int64, error) {
	g.next++
	return g.next, nil
}
