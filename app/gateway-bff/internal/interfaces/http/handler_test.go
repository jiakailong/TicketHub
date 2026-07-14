package httpapi

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	userv1 "tickethub/api/proto/user/v1"
	"tickethub/pkg/auth"
)

func TestAuthProxyRejectsMissingToken(t *testing.T) {
	handler := NewHandler(nil, auth.NewTokenManager("secret"))
	req := httptest.NewRequest(http.MethodGet, "/api/users/detail?user_id=1", nil)
	rec := httptest.NewRecorder()

	handler.authProxy("user-service", "/v1/users/detail")(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthProxyForwardsAuthorizedRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/users/detail" || r.URL.RawQuery != "user_id=1" {
			t.Fatalf("unexpected upstream URL: %s", r.URL.String())
		}
		if r.Header.Get(auth.UserIDHeader) != "1" || r.Header.Get(auth.RoleHeader) != "user" {
			t.Fatalf("trusted identity headers missing: %v", r.Header)
		}
		w.Header().Set("X-Upstream", "user")
		_, _ = w.Write([]byte(`{"code":"OK"}`))
	}))
	defer upstream.Close()

	tokens := auth.NewTokenManager("secret")
	token, err := tokens.Generate(auth.Claims{UserID: 1, Role: "user", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(map[string]string{"user-service": upstream.URL}, tokens)
	req := httptest.NewRequest(http.MethodGet, "/api/users/detail?user_id=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.authProxy("user-service", "/v1/users/detail")(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("X-Upstream") != "user" {
		t.Fatalf("status=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestAdminProxyRejectsUserRole(t *testing.T) {
	tokens := auth.NewTokenManager("secret")
	token, err := tokens.Generate(auth.Claims{UserID: 1, Role: "user", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(nil, tokens)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.adminProxy("admin-service", "/v1/admin/dashboard")(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserRegisterUsesGRPCWhenConfigured(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	userv1.RegisterUserServiceServer(server, fakeUserService{})
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()

	handler, err := NewHandlerWithGRPC(nil, map[string]string{"user-service": listener.Addr().String()}, auth.NewTokenManager("secret"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/users/register", bytes.NewBufferString(`{"mobile":"13800000000","password":"secret"}`))
	rec := httptest.NewRecorder()

	handler.userRegister(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "138****0000") || strings.Contains(rec.Body.String(), "13800000000") {
		t.Fatalf("expected grpc response body, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"99"`) {
		t.Fatalf("expected browser-safe string id, got %s", rec.Body.String())
	}
}

type fakeUserService struct {
	userv1.UnimplementedUserServiceServer
}

func (fakeUserService) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.UserReply, error) {
	return &userv1.UserReply{UserId: 99, Mobile: req.GetMobile(), RealNameStatus: "unverified"}, nil
}
