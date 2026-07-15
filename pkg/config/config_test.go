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
