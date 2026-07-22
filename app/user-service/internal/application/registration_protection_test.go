package application_test

import (
	"context"
	"testing"
	"time"

	"tickethub/app/user-service/internal/application"
	"tickethub/app/user-service/internal/domain/user"
	"tickethub/app/user-service/internal/infrastructure/security"
	therrors "tickethub/pkg/errors"
)

func TestRegisterStopsBeforeRepositoryAndHashWhenGuardRejects(t *testing.T) {
	repository := &recordingUserRepository{}
	hasher := &recordingHasher{}
	service := application.NewUserCommandService(&fakeIDs{}, repository, hasher).
		WithRegistrationGuard(fakeRegistrationGuard{checkErr: therrors.New(therrors.CodeCaptchaRequired, "captcha required")})

	_, err := service.Register(context.Background(), application.RegisterCommand{Mobile: "13800000000", Password: "secret", ClientIP: "198.51.100.1"})
	if !therrors.IsCode(err, therrors.CodeCaptchaRequired) {
		t.Fatalf("error = %v", err)
	}
	if repository.findCalls != 0 || repository.saveCalls != 0 || hasher.hashCalls != 0 {
		t.Fatalf("guard rejection accessed repository/hash: find=%d save=%d hash=%d", repository.findCalls, repository.saveCalls, hasher.hashCalls)
	}
}

func TestRegisterSkipsPrecheckWhenBloomConfirmsMobileAbsent(t *testing.T) {
	repository := &recordingUserRepository{}
	service := application.NewUserCommandService(&fakeIDs{next: 1}, repository, security.NewBcryptPasswordHasher(4)).
		WithRegistrationGuard(fakeRegistrationGuard{lookupState: application.MobileLookupAbsent})
	if _, err := service.Register(context.Background(), application.RegisterCommand{Mobile: "13800000000", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	if repository.findCalls != 0 || repository.saveCalls != 1 {
		t.Fatalf("repository calls = find:%d save:%d", repository.findCalls, repository.saveCalls)
	}
}

type fakeRegistrationGuard struct {
	checkErr    error
	lookupState application.MobileLookupState
	markCalls   int
}

func (g fakeRegistrationGuard) Check(context.Context, application.RegistrationAttempt) error {
	return g.checkErr
}
func (g fakeRegistrationGuard) IssueCaptcha(context.Context, application.RegisterCaptchaCommand) (application.RegisterCaptcha, error) {
	return application.RegisterCaptcha{ID: "captcha", Image: "data:image/png;base64,test", ExpiresIn: time.Minute}, nil
}
func (g fakeRegistrationGuard) LookupMobile(context.Context, string) (application.MobileLookupState, error) {
	return g.lookupState, nil
}
func (g fakeRegistrationGuard) MarkRegistered(context.Context, string) error { return nil }

type recordingUserRepository struct {
	findCalls int
	saveCalls int
}

func (r *recordingUserRepository) FindByID(context.Context, int64) (user.User, error) {
	return user.User{}, therrors.New(therrors.CodeNotFound, "not found")
}
func (r *recordingUserRepository) FindByMobile(context.Context, string) (user.User, error) {
	r.findCalls++
	return user.User{}, therrors.New(therrors.CodeNotFound, "not found")
}
func (r *recordingUserRepository) Save(context.Context, user.User) error {
	r.saveCalls++
	return nil
}

type recordingHasher struct{ hashCalls int }

func (h *recordingHasher) Hash(string) (string, error) {
	h.hashCalls++
	return "hash", nil
}
func (*recordingHasher) Compare(string, string) bool { return false }
