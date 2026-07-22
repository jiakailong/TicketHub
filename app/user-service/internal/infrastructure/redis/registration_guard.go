package redis

import (
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	base64captcha "github.com/mojocn/base64Captcha"
	"github.com/redis/go-redis/v9"

	"tickethub/app/user-service/internal/application"
	"tickethub/pkg/cache"
	"tickethub/pkg/config"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/observability"
	"tickethub/pkg/privacy"
)

type RegistrationGuard struct {
	client        redis.Cmdable
	lua           cache.RedisLuaExecutor
	keyBase       string
	config        config.RegistrationProtectionConfig
	secret        []byte
	lookup        func(string) []byte
	captchaDriver *base64captcha.DriverDigit
}

func NewRegistrationGuard(client redis.Cmdable, keyPrefix string, cfg config.RegistrationProtectionConfig, lookup func(string) []byte) (*RegistrationGuard, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if lookup == nil {
		return nil, fmt.Errorf("mobile lookup function is required")
	}
	secret, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.HMACSecret))
	if err != nil || len(secret) < 32 {
		return nil, fmt.Errorf("registration HMAC key must be base64 with at least 32 bytes")
	}
	return &RegistrationGuard{
		client:        client,
		lua:           cache.NewRedisLuaExecutor(client),
		keyBase:       strings.TrimRight(strings.TrimSpace(keyPrefix), ":"),
		config:        cfg,
		secret:        secret,
		lookup:        lookup,
		captchaDriver: base64captcha.NewDriverDigit(80, 240, 5, 0.7, 80),
	}, nil
}

func (g *RegistrationGuard) Check(ctx context.Context, attempt application.RegistrationAttempt) error {
	ipKey, mobileKey, binding, err := g.identities(attempt.ClientIP, attempt.Mobile)
	if err != nil {
		return err
	}
	value, err := g.lua.Eval(ctx, registrationPrecheckScript, []string{g.keyBase},
		ipKey, mobileKey, g.config.CaptchaTriggerPerSecond, g.config.IPRequestsPerMinute, g.config.MobileRequestsPerMinute)
	if err != nil {
		observability.IncCounter("tickethub_registration_guard_total", map[string]string{"result": "redis_error"})
		// Preserve registration availability. APISIX remains the outer limiter and
		// the application falls back to the MySQL uniqueness check below.
		return nil
	}
	result, err := luaResult(value)
	if err != nil {
		observability.IncCounter("tickethub_registration_guard_total", map[string]string{"result": "invalid_script_result"})
		return nil
	}
	if result == 2 {
		observability.IncCounter("tickethub_registration_guard_total", map[string]string{"result": "rate_limited"})
		return therrors.New(therrors.CodeTooManyRequests, "registration request limit exceeded")
	}
	hasCaptcha := strings.TrimSpace(attempt.CaptchaID) != "" || strings.TrimSpace(attempt.CaptchaAnswer) != ""
	if result == 1 && !hasCaptcha {
		observability.IncCounter("tickethub_registration_guard_total", map[string]string{"result": "captcha_required"})
		return therrors.New(therrors.CodeCaptchaRequired, "captcha required")
	}
	if !hasCaptcha {
		observability.IncCounter("tickethub_registration_guard_total", map[string]string{"result": "allowed"})
		return nil
	}
	if strings.TrimSpace(attempt.CaptchaID) == "" || strings.TrimSpace(attempt.CaptchaAnswer) == "" {
		return therrors.New(therrors.CodeCaptchaInvalid, "captcha id and answer are both required")
	}
	verified, err := g.verifyCaptcha(ctx, attempt.CaptchaID, binding, attempt.CaptchaAnswer)
	if err != nil {
		observability.IncCounter("tickethub_registration_captcha_total", map[string]string{"result": "redis_error"})
		return nil
	}
	if !verified {
		observability.IncCounter("tickethub_registration_captcha_total", map[string]string{"result": "invalid"})
		return therrors.New(therrors.CodeCaptchaInvalid, "captcha is invalid or expired")
	}
	observability.IncCounter("tickethub_registration_captcha_total", map[string]string{"result": "verified"})
	return nil
}

func (g *RegistrationGuard) IssueCaptcha(ctx context.Context, cmd application.RegisterCaptchaCommand) (application.RegisterCaptcha, error) {
	ipKey, _, binding, err := g.identities(cmd.ClientIP, cmd.Mobile)
	if err != nil {
		return application.RegisterCaptcha{}, err
	}
	id, err := randomCaptchaID()
	if err != nil {
		return application.RegisterCaptcha{}, therrors.Wrap(therrors.CodeInfrastructure, "create captcha id failed", err)
	}
	_, question, answer := g.captchaDriver.GenerateSpecificIdQuestionAnswer(id)
	item, err := g.captchaDriver.DrawCaptcha(question)
	if err != nil {
		return application.RegisterCaptcha{}, therrors.Wrap(therrors.CodeInfrastructure, "draw captcha failed", err)
	}
	value, err := g.lua.Eval(ctx, registrationIssueCaptchaScript, []string{g.keyBase},
		ipKey, id, g.answerHMAC(answer), binding, g.config.CaptchaMaxAttempts,
		int(g.config.CaptchaTTLDuration().Seconds()), g.config.CaptchaIssuesPerMinute)
	if err != nil {
		observability.IncCounter("tickethub_registration_captcha_total", map[string]string{"result": "redis_error"})
		return application.RegisterCaptcha{}, therrors.Wrap(therrors.CodeInfrastructure, "store captcha failed", err)
	}
	allowed, err := luaResult(value)
	if err != nil {
		return application.RegisterCaptcha{}, therrors.Wrap(therrors.CodeInfrastructure, "invalid captcha script response", err)
	}
	if allowed != 1 {
		observability.IncCounter("tickethub_registration_captcha_total", map[string]string{"result": "issue_rate_limited"})
		return application.RegisterCaptcha{}, therrors.New(therrors.CodeTooManyRequests, "captcha issue limit exceeded")
	}
	observability.IncCounter("tickethub_registration_captcha_total", map[string]string{"result": "issued"})
	return application.RegisterCaptcha{ID: id, Image: item.EncodeB64string(), ExpiresIn: g.config.CaptchaTTLDuration()}, nil
}

func (g *RegistrationGuard) LookupMobile(ctx context.Context, mobile string) (application.MobileLookupState, error) {
	ready, err := g.client.Get(ctx, g.bloomReadyKey()).Result()
	if err == redis.Nil || ready != "1" {
		return application.MobileLookupUnknown, nil
	}
	if err != nil {
		observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "redis_error"})
		return application.MobileLookupUnknown, nil
	}
	for _, position := range g.bloomPositions(g.lookup(privacy.NormalizeMobile(mobile))) {
		bit, err := g.client.GetBit(ctx, g.bloomKey(), position).Result()
		if err != nil {
			observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "redis_error"})
			return application.MobileLookupUnknown, nil
		}
		if bit == 0 {
			observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "absent"})
			return application.MobileLookupAbsent, nil
		}
	}
	observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "may_exist"})
	return application.MobileLookupMayExist, nil
}

func (g *RegistrationGuard) MarkRegistered(ctx context.Context, mobile string) error {
	for _, position := range g.bloomPositions(g.lookup(privacy.NormalizeMobile(mobile))) {
		if err := g.client.SetBit(ctx, g.bloomKey(), position, 1).Err(); err != nil {
			observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "mark_error"})
			return err
		}
	}
	observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "marked"})
	return nil
}

func (g *RegistrationGuard) Bootstrap(ctx context.Context, scanner application.MobileLookupScanner) error {
	if scanner == nil {
		return fmt.Errorf("mobile lookup scanner is required")
	}
	ready, err := g.client.Get(ctx, g.bloomReadyKey()).Result()
	if err == nil && ready == "1" {
		return nil
	}
	if err != nil && err != redis.Nil {
		return err
	}
	token, err := randomCaptchaID()
	if err != nil {
		return err
	}
	locked, err := g.client.SetNX(ctx, g.bootstrapLockKey(), token, 10*time.Minute).Result()
	if err != nil || !locked {
		return err
	}
	defer func() {
		_, _ = g.lua.Eval(context.Background(), registrationReleaseBootstrapLockScript, []string{g.bootstrapLockKey()}, token)
	}()
	if err := g.client.Del(ctx, g.bloomKey(), g.bloomReadyKey()).Err(); err != nil {
		return err
	}
	startedAt := time.Now()
	if err := scanner.ScanMobileLookups(ctx, g.config.BloomBootstrapBatchSize, func(ctx context.Context, lookup []byte) error {
		for _, position := range g.bloomPositions(lookup) {
			if err := g.client.SetBit(ctx, g.bloomKey(), position, 1).Err(); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "bootstrap_error"})
		return err
	}
	if err := g.client.Set(ctx, g.bloomReadyKey(), "1", 0).Err(); err != nil {
		return err
	}
	observability.IncCounter("tickethub_registration_bloom_total", map[string]string{"result": "bootstrap_ready"})
	observability.ObserveHistogram("tickethub_registration_bloom_bootstrap_duration_seconds", nil, time.Since(startedAt).Seconds())
	return nil
}

func (g *RegistrationGuard) verifyCaptcha(ctx context.Context, id string, binding string, answer string) (bool, error) {
	value, err := g.lua.Eval(ctx, registrationVerifyCaptchaScript, []string{g.captchaKey(id)}, binding, g.answerHMAC(answer))
	if err != nil {
		return false, err
	}
	result, err := luaResult(value)
	return result == 1, err
}

func (g *RegistrationGuard) identities(clientIP string, mobile string) (string, string, string, error) {
	clientIP = strings.TrimSpace(clientIP)
	mobile = privacy.NormalizeMobile(mobile)
	if clientIP == "" || mobile == "" {
		return "", "", "", therrors.New(therrors.CodeInvalidArgument, "client ip and mobile are required")
	}
	ipKey := g.hmac("ip:" + clientIP)
	mobileKey := g.hmac("mobile:" + mobile)
	return ipKey, mobileKey, g.hmac("bind:" + ipKey + ":" + mobileKey), nil
}

func (g *RegistrationGuard) answerHMAC(answer string) string {
	return g.hmac("answer:" + strings.ToLower(strings.TrimSpace(answer)))
}

func (g *RegistrationGuard) hmac(value string) string {
	mac := hmac.New(sha256.New, g.secret)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func (g *RegistrationGuard) bloomPositions(lookup []byte) []int64 {
	result := make([]int64, 0, g.config.BloomHashFunctions)
	for index := uint32(0); index < g.config.BloomHashFunctions; index++ {
		hash := sha256.New()
		_, _ = hash.Write(lookup)
		_, _ = hash.Write([]byte{byte(index >> 24), byte(index >> 16), byte(index >> 8), byte(index)})
		sum := hash.Sum(nil)
		value := uint64(0)
		for _, item := range sum[:8] {
			value = value<<8 | uint64(item)
		}
		result = append(result, int64(value%g.config.BloomBits))
	}
	return result
}

func (g *RegistrationGuard) captchaKey(id string) string {
	return g.keyBase + ":register:captcha:" + id
}
func (g *RegistrationGuard) bloomKey() string {
	return g.keyBase + ":register:bloom:mobile:" + g.config.BloomVersion
}
func (g *RegistrationGuard) bloomReadyKey() string { return g.bloomKey() + ":ready" }
func (g *RegistrationGuard) bootstrapLockKey() string {
	return g.bloomKey() + ":bootstrap-lock"
}

func randomCaptchaID() (string, error) {
	value := make([]byte, 16)
	if _, err := cryptorand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func luaResult(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected lua result %T", value)
	}
}
