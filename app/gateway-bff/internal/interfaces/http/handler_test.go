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
	"tickethub/pkg/httpx"
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
	fake := &fakeUserService{}
	userv1.RegisterUserServiceServer(server, fake)
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
	if fake.lastRegister.GetClientIp() != "192.0.2.1" {
		t.Fatalf("client ip = %q", fake.lastRegister.GetClientIp())
	}
}

func TestUserRegisterCaptchaForwardsTrustedClientIP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	fake := &fakeUserService{}
	userv1.RegisterUserServiceServer(server, fake)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	handler, err := NewHandlerWithGRPC(nil, map[string]string{"user-service": listener.Addr().String()}, auth.NewTokenManager("secret"))
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := httpx.NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	handler = handler.WithClientIPResolver(resolver)
	req := httptest.NewRequest(http.MethodPost, "/api/users/register/captcha", bytes.NewBufferString(`{"mobile":"13800000000"}`))
	req.RemoteAddr = "10.0.0.2:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.20")
	rec := httptest.NewRecorder()

	handler.userRegisterCaptcha(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if fake.lastCaptcha.GetMobile() != "13800000000" || fake.lastCaptcha.GetClientIp() != "198.51.100.20" {
		t.Fatalf("captcha request = %+v", fake.lastCaptcha)
	}
}

type fakeUserService struct {
	userv1.UnimplementedUserServiceServer
	lastRegister *userv1.RegisterRequest
	lastCaptcha  *userv1.RegisterCaptchaRequest
}

func (f *fakeUserService) Register(ctx context.Context, req *userv1.RegisterRequest) (*userv1.UserReply, error) {
	f.lastRegister = req
	return &userv1.UserReply{UserId: 99, Mobile: req.GetMobile(), RealNameStatus: "unverified"}, nil
}

func (f *fakeUserService) CreateRegisterCaptcha(ctx context.Context, req *userv1.RegisterCaptchaRequest) (*userv1.RegisterCaptchaReply, error) {
	f.lastCaptcha = req
	return &userv1.RegisterCaptchaReply{CaptchaId: "captcha", Image: "data:image/png;base64,test", ExpiresInSeconds: 120}, nil
}
