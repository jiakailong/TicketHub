package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tickethub/app/user-service/internal/application"
	"tickethub/app/user-service/internal/infrastructure/memory"
	"tickethub/app/user-service/internal/infrastructure/security"
	"tickethub/pkg/httpx"
)

func TestCreateRegisterCaptchaUsesTrustedClientIP(t *testing.T) {
	guard := &recordingGuard{}
	users := application.NewUserCommandService(&httpFakeIDs{}, memory.NewUserRepository(), security.NewBcryptPasswordHasher(4)).WithRegistrationGuard(guard)
	resolver, err := httpx.NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(users).WithClientIPResolver(resolver)
	req := httptest.NewRequest(http.MethodPost, "/v1/users/register/captcha", bytes.NewBufferString(`{"mobile":"13800000000"}`))
	req.RemoteAddr = "10.0.0.5:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.10")
	rec := httptest.NewRecorder()

	handler.createRegisterCaptcha(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if guard.issue.ClientIP != "198.51.100.10" || guard.issue.Mobile != "13800000000" {
		t.Fatalf("captcha command = %+v", guard.issue)
	}
	var body httpx.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "OK" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

type recordingGuard struct {
	issue application.RegisterCaptchaCommand
}

func (*recordingGuard) Check(context.Context, application.RegistrationAttempt) error { return nil }
func (g *recordingGuard) IssueCaptcha(_ context.Context, cmd application.RegisterCaptchaCommand) (application.RegisterCaptcha, error) {
	g.issue = cmd
	return application.RegisterCaptcha{ID: "captcha", Image: "data:image/png;base64,test", ExpiresIn: time.Minute}, nil
}
func (*recordingGuard) LookupMobile(context.Context, string) (application.MobileLookupState, error) {
	return application.MobileLookupUnknown, nil
}
func (*recordingGuard) MarkRegistered(context.Context, string) error { return nil }

type httpFakeIDs struct{ next int64 }

func (g *httpFakeIDs) NextID() (int64, error) {
	g.next++
	return g.next, nil
}
