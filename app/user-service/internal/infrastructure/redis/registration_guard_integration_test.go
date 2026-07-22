//go:build integration

package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"tickethub/app/user-service/internal/application"
	"tickethub/pkg/config"
	therrors "tickethub/pkg/errors"
)

func TestRegistrationGuardAgainstComposeRedis(t *testing.T) {
	if os.Getenv("TICKETHUB_INTEGRATION") != "1" {
		t.Skip("set TICKETHUB_INTEGRATION=1 with Compose Redis available")
	}
	addr := os.Getenv("TICKETHUB_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("connect Redis at %s: %v", addr, err)
	}
	var randomPrefix [8]byte
	if _, err := rand.Read(randomPrefix[:]); err != nil {
		t.Fatal(err)
	}
	prefix := "tickethub:integration:registration:" + hex.EncodeToString(randomPrefix[:])
	cfg := config.RegistrationProtectionConfig{
		CaptchaTTL:              "2m",
		CaptchaMaxAttempts:      2,
		CaptchaTriggerPerSecond: 2,
		IPRequestsPerMinute:     20,
		MobileRequestsPerMinute: 20,
		CaptchaIssuesPerMinute:  5,
		BloomVersion:            "v1",
		BloomBits:               1024,
		BloomHashFunctions:      3,
		BloomBootstrapBatchSize: 100,
		HMACSecret:              "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE=",
	}
	guard, err := NewRegistrationGuard(client, prefix, cfg, testLookup)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		keys, _ := client.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
	}()
	attempt := application.RegistrationAttempt{Mobile: "13800000000", ClientIP: "198.51.100.20"}
	if err := guard.Check(context.Background(), attempt); err != nil {
		t.Fatalf("first request rejected: %v", err)
	}
	if err := guard.Check(context.Background(), attempt); !therrors.IsCode(err, therrors.CodeCaptchaRequired) {
		t.Fatalf("second request error = %v", err)
	}
	challenge, err := guard.IssueCaptcha(context.Background(), application.RegisterCaptchaCommand{Mobile: attempt.Mobile, ClientIP: attempt.ClientIP})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.HSet(context.Background(), guard.captchaKey(challenge.ID), "answer_hmac", guard.answerHMAC("24680")).Err(); err != nil {
		t.Fatal(err)
	}
	attempt.CaptchaID = challenge.ID
	attempt.CaptchaAnswer = "24680"
	if err := guard.Check(context.Background(), attempt); err != nil {
		t.Fatalf("valid captcha rejected: %v", err)
	}
}
