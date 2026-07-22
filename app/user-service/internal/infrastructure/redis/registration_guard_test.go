package redis

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"tickethub/app/user-service/internal/application"
	"tickethub/pkg/config"
	therrors "tickethub/pkg/errors"
)

func TestRegistrationGuardRequiresCaptchaAndConsumesItOnce(t *testing.T) {
	guard := newTestRegistrationGuard(t, config.RegistrationProtectionConfig{
		CaptchaTriggerPerSecond: 2,
		IPRequestsPerMinute:     20,
		MobileRequestsPerMinute: 20,
		CaptchaIssuesPerMinute:  5,
	})
	ctx := context.Background()
	attempt := application.RegistrationAttempt{Mobile: "13800000000", ClientIP: "198.51.100.10"}
	if err := guard.Check(ctx, attempt); err != nil {
		t.Fatalf("first request rejected: %v", err)
	}
	if err := guard.Check(ctx, attempt); !therrors.IsCode(err, therrors.CodeCaptchaRequired) {
		t.Fatalf("second request error = %v", err)
	}
	challenge, err := guard.IssueCaptcha(ctx, application.RegisterCaptchaCommand{Mobile: attempt.Mobile, ClientIP: attempt.ClientIP})
	if err != nil {
		t.Fatal(err)
	}
	if challenge.ID == "" || len(challenge.Image) < len("data:image/png;base64,") || challenge.ExpiresIn != 2*time.Minute {
		t.Fatalf("unexpected challenge: %+v", challenge)
	}
	if err := guard.client.HSet(ctx, guard.captchaKey(challenge.ID), "answer_hmac", guard.answerHMAC("24680")).Err(); err != nil {
		t.Fatal(err)
	}
	attempt.CaptchaID = challenge.ID
	attempt.CaptchaAnswer = "24680"
	if err := guard.Check(ctx, attempt); err != nil {
		t.Fatalf("valid captcha rejected: %v", err)
	}
	if err := guard.Check(ctx, attempt); !therrors.IsCode(err, therrors.CodeCaptchaInvalid) {
		t.Fatalf("consumed captcha error = %v", err)
	}
}

func TestRegistrationGuardLimitsMobileAndUsesBloomReadyGate(t *testing.T) {
	guard := newTestRegistrationGuard(t, config.RegistrationProtectionConfig{
		CaptchaTriggerPerSecond: 100,
		IPRequestsPerMinute:     20,
		MobileRequestsPerMinute: 2,
		CaptchaIssuesPerMinute:  5,
	})
	ctx := context.Background()
	attempt := application.RegistrationAttempt{Mobile: "13800000000", ClientIP: "198.51.100.11"}
	for index := 0; index < 2; index++ {
		if err := guard.Check(ctx, attempt); err != nil {
			t.Fatalf("request %d error = %v", index, err)
		}
	}
	if err := guard.Check(ctx, attempt); !therrors.IsCode(err, therrors.CodeTooManyRequests) {
		t.Fatalf("mobile rate limit error = %v", err)
	}
	if state, err := guard.LookupMobile(ctx, attempt.Mobile); err != nil || state != application.MobileLookupUnknown {
		t.Fatalf("unready bloom state=%d err=%v", state, err)
	}
	if err := guard.MarkRegistered(ctx, attempt.Mobile); err != nil {
		t.Fatal(err)
	}
	if err := guard.client.Set(ctx, guard.bloomReadyKey(), "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if state, err := guard.LookupMobile(ctx, attempt.Mobile); err != nil || state != application.MobileLookupMayExist {
		t.Fatalf("marked bloom state=%d err=%v", state, err)
	}
	if state, err := guard.LookupMobile(ctx, "13900000000"); err != nil || state != application.MobileLookupAbsent {
		t.Fatalf("absent bloom state=%d err=%v", state, err)
	}
}

func TestRegistrationGuardBootstrapsBloomFromBlindIndexes(t *testing.T) {
	guard := newTestRegistrationGuard(t, config.RegistrationProtectionConfig{})
	lookup := testLookup("13800000000")
	if err := guard.Bootstrap(context.Background(), scannerFunc(func(ctx context.Context, batchSize int, visit func(context.Context, []byte) error) error {
		if batchSize != 1_000 {
			t.Fatalf("batch size = %d", batchSize)
		}
		return visit(ctx, lookup)
	})); err != nil {
		t.Fatal(err)
	}
	if state, err := guard.LookupMobile(context.Background(), "13800000000"); err != nil || state != application.MobileLookupMayExist {
		t.Fatalf("bootstrapped state=%d err=%v", state, err)
	}
}

func TestRegistrationGuardConsumesOneCaptchaForOnlyOneConcurrentRequest(t *testing.T) {
	guard := newTestRegistrationGuard(t, config.RegistrationProtectionConfig{CaptchaTriggerPerSecond: 100})
	ctx := context.Background()
	attempt := application.RegistrationAttempt{Mobile: "13800000000", ClientIP: "198.51.100.12"}
	challenge, err := guard.IssueCaptcha(ctx, application.RegisterCaptchaCommand{Mobile: attempt.Mobile, ClientIP: attempt.ClientIP})
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.client.HSet(ctx, guard.captchaKey(challenge.ID), "answer_hmac", guard.answerHMAC("24680")).Err(); err != nil {
		t.Fatal(err)
	}
	attempt.CaptchaID = challenge.ID
	attempt.CaptchaAnswer = "24680"
	results := make(chan error, 2)
	var group sync.WaitGroup
	for index := 0; index < 2; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- guard.Check(context.Background(), attempt)
		}()
	}
	group.Wait()
	close(results)
	successes := 0
	invalids := 0
	for result := range results {
		if result == nil {
			successes++
		} else if therrors.IsCode(result, therrors.CodeCaptchaInvalid) {
			invalids++
		} else {
			t.Fatalf("unexpected concurrent result: %v", result)
		}
	}
	if successes != 1 || invalids != 1 {
		t.Fatalf("successes=%d invalids=%d", successes, invalids)
	}
}

func newTestRegistrationGuard(t *testing.T, cfg config.RegistrationProtectionConfig) *RegistrationGuard {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	if cfg.CaptchaTTL == "" {
		cfg.CaptchaTTL = "2m"
	}
	if cfg.CaptchaMaxAttempts == 0 {
		cfg.CaptchaMaxAttempts = 5
	}
	if cfg.CaptchaTriggerPerSecond == 0 {
		cfg.CaptchaTriggerPerSecond = 10
	}
	if cfg.IPRequestsPerMinute == 0 {
		cfg.IPRequestsPerMinute = 60
	}
	if cfg.MobileRequestsPerMinute == 0 {
		cfg.MobileRequestsPerMinute = 5
	}
	if cfg.CaptchaIssuesPerMinute == 0 {
		cfg.CaptchaIssuesPerMinute = 20
	}
	if cfg.BloomVersion == "" {
		cfg.BloomVersion = "v1"
	}
	if cfg.BloomBits == 0 {
		cfg.BloomBits = 1024
	}
	if cfg.BloomHashFunctions == 0 {
		cfg.BloomHashFunctions = 3
	}
	if cfg.BloomBootstrapBatchSize == 0 {
		cfg.BloomBootstrapBatchSize = 1_000
	}
	cfg.HMACSecret = base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	guard, err := NewRegistrationGuard(client, "tickethub:user", cfg, testLookup)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func testLookup(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

type scannerFunc func(context.Context, int, func(context.Context, []byte) error) error

func (f scannerFunc) ScanMobileLookups(ctx context.Context, batchSize int, visit func(context.Context, []byte) error) error {
	return f(ctx, batchSize, visit)
}
