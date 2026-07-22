package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("TICKETHUB_JWT_SECRET", "test-only-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("service:\n  name: user-service\nhttp:\n  addr: :8001\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Service.Env != "local" {
		t.Fatalf("expected local env, got %s", cfg.Service.Env)
	}
	if cfg.Service.AdapterMode != "memory" {
		t.Fatalf("expected memory adapter mode, got %s", cfg.Service.AdapterMode)
	}
	if cfg.HTTP.TimeoutDuration(time.Second) != 5*time.Second {
		t.Fatalf("expected default http timeout")
	}
	if cfg.Cache.LocalTTLDuration() != 45*time.Second || cfg.Cache.RedisTTLDuration() != 5*time.Minute {
		t.Fatalf("unexpected cache defaults: %+v", cfg.Cache)
	}
}

func TestLoadAppliesProductionLoadTestDefaults(t *testing.T) {
	cfg := Config{Service: ServiceConfig{Name: "order-service"}, HTTP: EndpointConfig{Addr: ":8003"}}
	cfg.ApplyDefaults()
	if cfg.Cache.ModeName() != "multilevel" {
		t.Fatalf("cache mode = %s", cfg.Cache.ModeName())
	}
	if got := cfg.Workers.CancelDelayDuration(); got != 15*time.Minute {
		t.Fatalf("cancel delay = %s", got)
	}
	if got := cfg.Workers.DelayVisibilityDuration(); got != 2*time.Minute {
		t.Fatalf("visibility = %s", got)
	}
	if got := cfg.Workers.PollIntervalDuration(); got != time.Second {
		t.Fatalf("poll interval = %s", got)
	}
	if cfg.Workers.CreateBatchSize != 32 || cfg.Workers.CancelBatchSize != 32 {
		t.Fatalf("batch sizes = %d/%d", cfg.Workers.CreateBatchSize, cfg.Workers.CancelBatchSize)
	}
	if cfg.PurchaseRateLimit.ProgramRate != 1000 || cfg.PurchaseRateLimit.ProgramBurst != 1500 {
		t.Fatalf("program limiter = %+v", cfg.PurchaseRateLimit)
	}
}

func TestValidateRejectsUnknownCacheMode(t *testing.T) {
	cfg := Config{Service: ServiceConfig{Name: "program-service"}, HTTP: EndpointConfig{Addr: ":8002"}}
	cfg.Cache.Mode = "unknown"
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid cache mode")
	}
}

func TestValidateRestrictsWorkerFailpointToLoadEnvironment(t *testing.T) {
	cfg := Config{Service: ServiceConfig{Name: "order-service", Env: "local"}, HTTP: EndpointConfig{Addr: ":8003"}}
	cfg.Workers.FailBeforeAckCount = 1
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected failpoint outside load environment to be rejected")
	}
	cfg.Service.Env = "load"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("load failpoint rejected: %v", err)
	}
}

func TestLoadValidatesEnabledShardDatabases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "service:\n  name: order-service\nhttp:\n  addr: :8003\nsharding:\n  enabled: true\n  database_count: 2\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected missing shard dsn error")
	}
}

func TestOrderShardingEnvironmentOverrideDoesNotAffectOtherServices(t *testing.T) {
	t.Setenv("TICKETHUB_SHARDING_ENABLED", "true")
	t.Setenv("TICKETHUB_JWT_SECRET", "test-only-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("service:\n  name: user-service\nhttp:\n  addr: :8001\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sharding.Enabled {
		t.Fatal("order sharding override must not enable sharding for user-service")
	}
}

func TestUserInfrastructurePrivacyConfigurationFromEnvironment(t *testing.T) {
	t.Setenv("TICKETHUB_ADAPTER_MODE", "infra")
	t.Setenv("TICKETHUB_JWT_SECRET", "test-only-secret")
	t.Setenv("TICKETHUB_PRIVACY_ACTIVE_KEY_VERSION", "v2")
	t.Setenv("TICKETHUB_PRIVACY_ENCRYPTION_KEYS", "v1:first,v2:second")
	t.Setenv("TICKETHUB_PRIVACY_LOOKUP_KEY", "lookup")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("service:\n  name: user-service\nhttp:\n  addr: :8001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Privacy.ActiveKeyVersion != "v2" || cfg.Privacy.EncryptionKeys["v2"] != "second" || cfg.Privacy.LookupKey != "lookup" {
		t.Fatalf("privacy config = %+v", cfg.Privacy)
	}
}

func TestUserInfrastructureRequiresPrivacyConfiguration(t *testing.T) {
	t.Setenv("TICKETHUB_ADAPTER_MODE", "infra")
	t.Setenv("TICKETHUB_JWT_SECRET", "test-only-secret")
	t.Setenv("TICKETHUB_PRIVACY_ACTIVE_KEY_VERSION", "")
	t.Setenv("TICKETHUB_PRIVACY_ENCRYPTION_KEYS", "")
	t.Setenv("TICKETHUB_PRIVACY_LOOKUP_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("service:\n  name: user-service\nhttp:\n  addr: :8001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected missing privacy configuration to fail closed")
	}
}

func TestLoadExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("TICKETHUB_JWT_SECRET", "expanded-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "service:\n  name: gateway-bff\nhttp:\n  addr: :8080\nauth:\n  jwt_secret: ${TICKETHUB_JWT_SECRET}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.JWTSecret != "expanded-secret" {
		t.Fatalf("jwt secret was not expanded")
	}
}

func TestUserInfrastructureRegistrationProtectionRequiresHMACSecret(t *testing.T) {
	t.Setenv("TICKETHUB_ADAPTER_MODE", "infra")
	t.Setenv("TICKETHUB_JWT_SECRET", "test-only-secret")
	t.Setenv("TICKETHUB_PRIVACY_ACTIVE_KEY_VERSION", "v1")
	t.Setenv("TICKETHUB_PRIVACY_ENCRYPTION_KEYS", "v1:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("TICKETHUB_PRIVACY_LOOKUP_KEY", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("TICKETHUB_REGISTER_PROTECTION_HMAC_SECRET", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "service:\n  name: user-service\nhttp:\n  addr: :8001\nregistration_protection:\n  enabled: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected missing registration hmac secret to fail")
	}
}

func TestRegistrationProtectionDefaultsAndTrustedProxyValidation(t *testing.T) {
	cfg := Config{Service: ServiceConfig{Name: "gateway-bff"}, HTTP: EndpointConfig{Addr: ":8080"}}
	cfg.ApplyDefaults()
	if cfg.Registration.CaptchaTTLDuration() != 2*time.Minute || cfg.Registration.BloomBits != 14_377_588 {
		t.Fatalf("registration defaults = %+v", cfg.Registration)
	}
	cfg.Security.TrustedProxyCIDRs = []string{"not-a-cidr"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid trusted proxy CIDR")
	}
}

func TestEndpointEnvironmentOverridesSupportLocalReplicas(t *testing.T) {
	t.Setenv("TICKETHUB_HTTP_ADDR", ":8103")
	t.Setenv("TICKETHUB_GRPC_ADDR", ":9103")
	t.Setenv("TICKETHUB_METRICS_ADDR", ":19103")
	cfg := Config{Service: ServiceConfig{Name: "order-service"}}
	cfg.ApplyDefaults()
	if cfg.HTTP.Addr != ":8103" || cfg.GRPC.Addr != ":9103" || cfg.Observability.MetricsAddr != ":19103" {
		t.Fatalf("endpoint overrides = http:%s grpc:%s metrics:%s", cfg.HTTP.Addr, cfg.GRPC.Addr, cfg.Observability.MetricsAddr)
	}
}
