package application

import (
	"context"
	"time"

	therrors "tickethub/pkg/errors"
)

type RegistrationAttempt struct {
	Mobile        string
	ClientIP      string
	CaptchaID     string
	CaptchaAnswer string
}

type RegisterCaptchaCommand struct {
	Mobile   string
	ClientIP string
}

type RegisterCaptcha struct {
	ID        string
	Image     string
	ExpiresIn time.Duration
}

type MobileLookupState uint8

const (
	MobileLookupUnknown MobileLookupState = iota
	MobileLookupAbsent
	MobileLookupMayExist
)

// RegistrationGuard is the application boundary for registration anti-abuse
// controls and the registered-mobile Bloom filter.
type RegistrationGuard interface {
	Check(context.Context, RegistrationAttempt) error
	IssueCaptcha(context.Context, RegisterCaptchaCommand) (RegisterCaptcha, error)
	LookupMobile(context.Context, string) (MobileLookupState, error)
	MarkRegistered(context.Context, string) error
}

type MobileLookupScanner interface {
	ScanMobileLookups(context.Context, int, func(context.Context, []byte) error) error
}

type NoopRegistrationGuard struct{}

func NewNoopRegistrationGuard() NoopRegistrationGuard {
	return NoopRegistrationGuard{}
}

func (NoopRegistrationGuard) Check(context.Context, RegistrationAttempt) error { return nil }

func (NoopRegistrationGuard) IssueCaptcha(context.Context, RegisterCaptchaCommand) (RegisterCaptcha, error) {
	return RegisterCaptcha{}, therrors.New(therrors.CodeInfrastructure, "registration protection is disabled")
}

func (NoopRegistrationGuard) LookupMobile(context.Context, string) (MobileLookupState, error) {
	return MobileLookupUnknown, nil
}

func (NoopRegistrationGuard) MarkRegistered(context.Context, string) error { return nil }
