package application_test

import (
	"context"
	"testing"

	"tickethub/app/user-service/internal/application"
	"tickethub/app/user-service/internal/infrastructure/memory"
	"tickethub/app/user-service/internal/infrastructure/security"
	"tickethub/pkg/auth"
)

func TestUserCommandServiceRegisterLoginAndTicketUsers(t *testing.T) {
	ids := &fakeIDs{next: 1000}
	service := application.NewUserCommandService(
		ids,
		memory.NewUserRepository(),
		security.NewBcryptPasswordHasher(4),
	).
		WithTicketUsers(memory.NewTicketUserRepository()).
		WithAdminMobiles([]string{"13800000000"}).
		WithTokenManager(auth.NewTokenManager("test-secret"))

	ctx := context.Background()
	created, err := service.Register(ctx, application.RegisterCommand{Mobile: "13800000000", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 1001 {
		t.Fatalf("created id = %d", created.ID)
	}

	login, err := service.Login(ctx, application.LoginCommand{Mobile: "13800000000", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if login.AccessToken == "" || login.User.ID != created.ID {
		t.Fatalf("unexpected login result: %+v", login)
	}
	claims, err := auth.NewTokenManager("test-secret").Parse(login.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Role != "admin" {
		t.Fatalf("role = %s", claims.Role)
	}

	ticketUser, err := service.AddTicketUser(ctx, application.AddTicketUserCommand{
		UserID:        created.ID,
		Name:          "张三",
		CertificateNo: "310101199001010011",
		Mobile:        "13800000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.ListTicketUsers(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != ticketUser.ID {
		t.Fatalf("ticket users = %+v", items)
	}
}

func TestUserCommandServiceRejectsBadPassword(t *testing.T) {
	service := application.NewUserCommandService(
		&fakeIDs{next: 1},
		memory.NewUserRepository(),
		security.NewBcryptPasswordHasher(4),
	)
	ctx := context.Background()
	if _, err := service.Register(ctx, application.RegisterCommand{Mobile: "13800000001", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(ctx, application.LoginCommand{Mobile: "13800000001", Password: "bad"}); err == nil {
		t.Fatal("expected login to reject bad password")
	}
}

type fakeIDs struct {
	next int64
}

func (g *fakeIDs) NextID() (int64, error) {
	g.next++
	return g.next, nil
}
