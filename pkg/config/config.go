package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Service           ServiceConfig           `yaml:"service"`
	HTTP              EndpointConfig          `yaml:"http"`
	GRPC              EndpointConfig          `yaml:"grpc"`
	MySQL             MySQLConfig             `yaml:"mysql"`
	Sharding          ShardingConfig          `yaml:"sharding"`
	Redis             RedisConfig             `yaml:"redis"`
	Cache             CacheConfig             `yaml:"cache"`
	PurchaseRateLimit PurchaseRateLimitConfig `yaml:"purchase_rate_limit"`
	Workers           WorkerConfig            `yaml:"workers"`
	Kafka             KafkaConfig             `yaml:"kafka"`
	Elasticsearch     ElasticsearchConfig     `yaml:"elasticsearch"`
	Upstreams         map[string]string       `yaml:"upstreams"`
	GRPCUpstreams     map[string]string       `yaml:"grpc_upstreams"`
	Observability     ObservabilityConfig     `yaml:"observability"`
	Auth              AuthConfig              `yaml:"auth"`
	Privacy           PrivacyConfig           `yaml:"privacy"`
}

type ServiceConfig struct {
	Name        string `yaml:"name"`
	Env         string `yaml:"env"`
	AdapterMode string `yaml:"adapter_mode"`
}

type EndpointConfig struct {
	Addr    string `yaml:"addr"`
	Timeout string `yaml:"timeout"`
}

type MySQLConfig struct {
	DSN          string `yaml:"dsn"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type ShardingConfig struct {
	Enabled        bool                   `yaml:"enabled"`
	DatabasePrefix string                 `yaml:"database_prefix"`
	TablePrefix    string                 `yaml:"table_prefix"`
	DatabaseCount  int                    `yaml:"database_count"`
	TableCount     int                    `yaml:"table_count"`
	Databases      map[string]MySQLConfig `yaml:"databases"`
}

type RedisConfig struct {
	Addr      string `yaml:"addr"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	DB        int    `yaml:"db"`
	KeyPrefix string `yaml:"key_prefix"`
}

type CacheConfig struct {
	Mode              string `yaml:"mode"`
	LocalNumCounters  int64  `yaml:"local_num_counters"`
	LocalMaxCostBytes int64  `yaml:"local_max_cost_bytes"`
	LocalBufferItems  int64  `yaml:"local_buffer_items"`
	LocalLockStripes  int    `yaml:"local_lock_stripes"`
	LocalTTL          string `yaml:"local_ttl"`
	RedisTTL          string `yaml:"redis_ttl"`
	RebuildLockTTL    string `yaml:"rebuild_lock_ttl"`
	RebuildWait       string `yaml:"rebuild_wait"`
	RebuildPoll       string `yaml:"rebuild_poll"`
}

type PurchaseRateLimitConfig struct {
	UserRate     int `yaml:"user_rate"`
	UserBurst    int `yaml:"user_burst"`
	ProgramRate  int `yaml:"program_rate"`
	ProgramBurst int `yaml:"program_burst"`
}

type WorkerConfig struct {
	Disabled               []string `yaml:"disabled"`
	PollInterval           string   `yaml:"poll_interval"`
	CreateBatchSize        int      `yaml:"create_batch_size"`
	CancelBatchSize        int      `yaml:"cancel_batch_size"`
	CancelDelay            string   `yaml:"cancel_delay"`
	DelayVisibilityTimeout string   `yaml:"delay_visibility_timeout"`
	FailBeforeAckCount     int      `yaml:"fail_before_ack_count"`
}

type KafkaConfig struct {
	Brokers  []string          `yaml:"brokers"`
	ClientID string            `yaml:"client_id"`
	GroupID  string            `yaml:"group_id"`
	Topics   map[string]string `yaml:"topics"`
}

type ElasticsearchConfig struct {
	Addresses    []string `yaml:"addresses"`
	Index        string   `yaml:"index"`
	SyncInterval string   `yaml:"sync_interval"`
	BatchSize    int      `yaml:"batch_size"`
	Version      string   `yaml:"version"`
}

type ObservabilityConfig struct {
	MetricsAddr string `yaml:"metrics_addr"`
	TraceOTLP   string `yaml:"trace_otlp"`
	LogLevel    string `yaml:"log_level"`
}

type AuthConfig struct {
	JWTSecret    string   `yaml:"jwt_secret"`
	AdminMobiles []string `yaml:"admin_mobiles"`
}

type PrivacyConfig struct {
	ActiveKeyVersion string            `yaml:"active_key_version"`
	EncryptionKeys   map[string]string `yaml:"encryption_keys"`
	LookupKey        string            `yaml:"lookup_key"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	expanded := os.ExpandEnv(string(data))
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func MustLoad(path string) Config {
	cfg, err := Load(path)
	if err != nil {
		panic(err)
	}
	return cfg
}

func (c *Config) ApplyDefaults() {
	c.Service.Name = strings.TrimSpace(c.Service.Name)
	if c.Service.Env == "" {
		c.Service.Env = "local"
	}
	if c.Service.AdapterMode == "" {
		c.Service.AdapterMode = "memory"
	}
	if override := strings.TrimSpace(os.Getenv("TICKETHUB_ADAPTER_MODE")); override != "" {
		c.Service.AdapterMode = override
	}
	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":8080"
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_HTTP_ADDR")); value != "" {
		c.HTTP.Addr = value
	}
	if c.HTTP.Timeout == "" {
		c.HTTP.Timeout = "5s"
	}
	if c.GRPC.Timeout == "" {
		c.GRPC.Timeout = "5s"
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_GRPC_ADDR")); value != "" {
		c.GRPC.Addr = value
	}
	if c.MySQL.MaxOpenConns == 0 {
		c.MySQL.MaxOpenConns = 32
	}
	if c.MySQL.MaxIdleConns == 0 {
		c.MySQL.MaxIdleConns = 8
	}
	if c.Cache.LocalNumCounters <= 0 {
		c.Cache.LocalNumCounters = 100_000
	}
	if c.Cache.LocalMaxCostBytes <= 0 {
		c.Cache.LocalMaxCostBytes = 64 << 20
	}
	if c.Cache.LocalBufferItems <= 0 {
		c.Cache.LocalBufferItems = 64
	}
	if c.Cache.LocalLockStripes <= 0 {
		c.Cache.LocalLockStripes = 256
	}
	if c.Cache.LocalTTL == "" {
		c.Cache.LocalTTL = "45s"
	}
	if c.Cache.RedisTTL == "" {
		c.Cache.RedisTTL = "5m"
	}
	if c.Cache.RebuildLockTTL == "" {
		c.Cache.RebuildLockTTL = "5s"
	}
	if c.Cache.RebuildWait == "" {
		c.Cache.RebuildWait = "750ms"
	}
	if c.Cache.RebuildPoll == "" {
		c.Cache.RebuildPoll = "25ms"
	}
	if c.PurchaseRateLimit.UserRate <= 0 {
		c.PurchaseRateLimit.UserRate = 3
	}
	if c.PurchaseRateLimit.UserBurst <= 0 {
		c.PurchaseRateLimit.UserBurst = 6
	}
	if c.PurchaseRateLimit.ProgramRate <= 0 {
		c.PurchaseRateLimit.ProgramRate = 1000
	}
	if c.PurchaseRateLimit.ProgramBurst <= 0 {
		c.PurchaseRateLimit.ProgramBurst = 1500
	}
	if c.Workers.PollInterval == "" {
		c.Workers.PollInterval = "1s"
	}
	if c.Workers.CreateBatchSize <= 0 {
		c.Workers.CreateBatchSize = 32
	}
	if c.Workers.CancelBatchSize <= 0 {
		c.Workers.CancelBatchSize = 32
	}
	if c.Workers.CancelDelay == "" {
		c.Workers.CancelDelay = "15m"
	}
	if c.Workers.DelayVisibilityTimeout == "" {
		c.Workers.DelayVisibilityTimeout = "2m"
	}
	if c.Sharding.DatabasePrefix == "" {
		c.Sharding.DatabasePrefix = "tickethub_order"
	}
	if c.Sharding.TablePrefix == "" {
		c.Sharding.TablePrefix = "orders"
	}
	if c.Sharding.DatabaseCount <= 0 {
		c.Sharding.DatabaseCount = 2
	}
	if c.Sharding.TableCount <= 0 {
		c.Sharding.TableCount = 2
	}
	if c.Sharding.Databases == nil {
		c.Sharding.Databases = map[string]MySQLConfig{}
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_SHARDING_ENABLED")); value != "" && c.Service.Name == "order-service" {
		if enabled, err := strconv.ParseBool(value); err == nil {
			c.Sharding.Enabled = enabled
		}
	}
	for name, database := range c.Sharding.Databases {
		if database.MaxOpenConns <= 0 {
			database.MaxOpenConns = c.MySQL.MaxOpenConns
		}
		if database.MaxIdleConns <= 0 {
			database.MaxIdleConns = c.MySQL.MaxIdleConns
		}
		c.Sharding.Databases[name] = database
	}
	if c.Kafka.Topics == nil {
		c.Kafka.Topics = map[string]string{}
	}
	if c.Elasticsearch.Index == "" {
		c.Elasticsearch.Index = "tickethub_programs"
	}
	if c.Elasticsearch.SyncInterval == "" {
		c.Elasticsearch.SyncInterval = "5m"
	}
	if c.Elasticsearch.BatchSize <= 0 {
		c.Elasticsearch.BatchSize = 500
	}
	if c.Elasticsearch.Version == "" {
		c.Elasticsearch.Version = "v2"
	}
	if c.Upstreams == nil {
		c.Upstreams = map[string]string{}
	}
	if c.GRPCUpstreams == nil {
		c.GRPCUpstreams = map[string]string{}
	}
	if c.Observability.LogLevel == "" {
		c.Observability.LogLevel = "info"
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_METRICS_ADDR")); value != "" {
		c.Observability.MetricsAddr = value
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_JWT_SECRET")); value != "" {
		c.Auth.JWTSecret = value
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_ADMIN_MOBILES")); value != "" {
		c.Auth.AdminMobiles = strings.Split(value, ",")
	}
	for index := range c.Auth.AdminMobiles {
		c.Auth.AdminMobiles[index] = strings.TrimSpace(c.Auth.AdminMobiles[index])
	}
	if c.Privacy.EncryptionKeys == nil {
		c.Privacy.EncryptionKeys = map[string]string{}
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_PRIVACY_ACTIVE_KEY_VERSION")); value != "" {
		c.Privacy.ActiveKeyVersion = value
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_PRIVACY_ENCRYPTION_KEYS")); value != "" {
		c.Privacy.EncryptionKeys = parseVersionedSecrets(value)
	}
	if value := strings.TrimSpace(os.Getenv("TICKETHUB_PRIVACY_LOOKUP_KEY")); value != "" {
		c.Privacy.LookupKey = value
	}
}

func (c CacheConfig) ModeName() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return "multilevel"
	}
	return mode
}

func (c WorkerConfig) Enabled(name string) bool {
	for _, disabled := range c.Disabled {
		if strings.EqualFold(strings.TrimSpace(disabled), strings.TrimSpace(name)) {
			return false
		}
	}
	return true
}

func (c WorkerConfig) PollIntervalDuration() time.Duration {
	return parseDuration(c.PollInterval, time.Second)
}

func (c WorkerConfig) CancelDelayDuration() time.Duration {
	return parseDuration(c.CancelDelay, 15*time.Minute)
}

func (c WorkerConfig) DelayVisibilityDuration() time.Duration {
	return parseDuration(c.DelayVisibilityTimeout, 2*time.Minute)
}

func (c CacheConfig) LocalTTLDuration() time.Duration {
	return parseDuration(c.LocalTTL, 45*time.Second)
}

func (c CacheConfig) RedisTTLDuration() time.Duration {
	return parseDuration(c.RedisTTL, 5*time.Minute)
}

func (c CacheConfig) RebuildLockTTLDuration() time.Duration {
	return parseDuration(c.RebuildLockTTL, 5*time.Second)
}

func (c CacheConfig) RebuildWaitDuration() time.Duration {
	return parseDuration(c.RebuildWait, 750*time.Millisecond)
}

func (c CacheConfig) RebuildPollDuration() time.Duration {
	return parseDuration(c.RebuildPoll, 25*time.Millisecond)
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func (e ElasticsearchConfig) SyncIntervalDuration(defaultValue time.Duration) time.Duration {
	if strings.TrimSpace(e.SyncInterval) == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(e.SyncInterval)
	if err != nil {
		return defaultValue
	}
	return value
}

func (c Config) UseInfrastructure() bool {
	mode := strings.ToLower(strings.TrimSpace(c.Service.AdapterMode))
	return mode == "infra" || mode == "infrastructure" || mode == "production"
}

func (c Config) Validate() error {
	if c.Service.Name == "" {
		return fmt.Errorf("service.name is required")
	}
	if strings.TrimSpace(c.HTTP.Addr) == "" {
		return fmt.Errorf("http.addr is required")
	}
	switch c.Cache.ModeName() {
	case "mysql", "redis", "multilevel":
	default:
		return fmt.Errorf("cache.mode must be mysql, redis, or multilevel")
	}
	if c.Workers.FailBeforeAckCount > 0 && !strings.EqualFold(strings.TrimSpace(c.Service.Env), "load") {
		return fmt.Errorf("workers.fail_before_ack_count is only allowed in load environment")
	}
	if (c.Service.Name == "gateway-bff" || c.Service.Name == "user-service") && strings.TrimSpace(c.Auth.JWTSecret) == "" {
		return fmt.Errorf("auth.jwt_secret is required for %s", c.Service.Name)
	}
	if c.Sharding.Enabled {
		for index := 0; index < c.Sharding.DatabaseCount; index++ {
			name := fmt.Sprintf("%s_%d", c.Sharding.DatabasePrefix, index)
			database, ok := c.Sharding.Databases[name]
			if !ok || strings.TrimSpace(database.DSN) == "" {
				return fmt.Errorf("sharding database %s dsn is required", name)
			}
		}
	}
	if c.Service.Name == "user-service" && c.UseInfrastructure() {
		if strings.TrimSpace(c.Privacy.ActiveKeyVersion) == "" {
			return fmt.Errorf("privacy.active_key_version is required for user-service infrastructure mode")
		}
		if strings.TrimSpace(c.Privacy.EncryptionKeys[c.Privacy.ActiveKeyVersion]) == "" {
			return fmt.Errorf("privacy encryption key %s is required", c.Privacy.ActiveKeyVersion)
		}
		if strings.TrimSpace(c.Privacy.LookupKey) == "" {
			return fmt.Errorf("privacy.lookup_key is required for user-service infrastructure mode")
		}
	}
	return nil
}

func parseVersionedSecrets(value string) map[string]string {
	result := map[string]string{}
	for _, item := range strings.Split(value, ",") {
		version, secret, ok := strings.Cut(strings.TrimSpace(item), ":")
		if ok && strings.TrimSpace(version) != "" && strings.TrimSpace(secret) != "" {
			result[strings.TrimSpace(version)] = strings.TrimSpace(secret)
		}
	}
	return result
}

func (e EndpointConfig) TimeoutDuration(defaultValue time.Duration) time.Duration {
	if strings.TrimSpace(e.Timeout) == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(e.Timeout)
	if err != nil {
		return defaultValue
	}
	return value
}
