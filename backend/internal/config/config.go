package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Startup   StartupConfig   `yaml:"startup"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	Kafka     KafkaConfig     `yaml:"kafka"`
	Timeout   TimeoutConfig   `yaml:"timeout"`
	JWT       JWTConfig       `yaml:"jwt"`
	Admin     AdminConfig     `yaml:"admin"`
	Email     EmailConfig     `yaml:"email"`
	Snowflake SnowflakeConfig `yaml:"snowflake"`
	Seckill   SeckillConfig   `yaml:"seckill"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

type StartupConfig struct {
	FlushRedisOnStart bool `yaml:"flush_redis_on_start"` // 是否在 API 启动时清空 Redis（默认 false）
}

type SeckillConfig struct {
	MaxBuyLimit     int                   `yaml:"max_buy_limit"` // 每用户每商品最大购买数量
	DefaultActivity DefaultActivityConfig `yaml:"default_activity"`
}

type DefaultActivityConfig struct {
	Enabled       bool   `yaml:"enabled"`
	TitleTemplate string `yaml:"title_template"`
	StartOffset   string `yaml:"start_offset"`
	EndOffset     string `yaml:"end_offset"`
}

type RateLimitConfig struct {
	Enabled   bool `yaml:"enabled"`    // 是否启用限流
	Rate      int  `yaml:"rate"`       // 令牌生成速率（每秒）
	Capacity  int  `yaml:"capacity"`   // 桶容量（最大令牌数）
	ExpireSec int  `yaml:"expire_sec"` // Redis key 过期时间（秒）
}

type SnowflakeConfig struct {
	WorkerID int64 `yaml:"worker_id"` // 工作节点ID，默认 1
	Epoch    int64 `yaml:"epoch"`     // 起始时间戳(毫秒)，默认 2024-01-01 00:00:00 UTC
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type EmailConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	From         string `yaml:"from"`
	FromName     string `yaml:"from_name"`
	Encryption   string `yaml:"encryption"`
	CodeLength   int    `yaml:"code_length"`
	CodeTTL      string `yaml:"code_ttl"`
	SendInterval string `yaml:"send_interval"`
	Subject      string `yaml:"subject"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type MySQLConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	Database           string `yaml:"database"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	MaxOpenConns       int    `yaml:"max_open_conns"`
	MaxIdleConns       int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeSec int    `yaml:"conn_max_lifetime_sec"`  // 连接最大生命周期(秒)，默认3600
	ConnMaxIdleTimeSec int    `yaml:"conn_max_idle_time_sec"` // 空闲连接超时(秒)，默认600
}

type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	SegmentCount int    `yaml:"segment_count"` // 库存分段数量
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
	Topic   string   `yaml:"topic"`
	Group   string   `yaml:"group"`

	// Producer 配置
	Producer KafkaProducerConfig `yaml:"producer"`

	// Consumer 配置
	Consumer KafkaConsumerConfig `yaml:"consumer"`
}

type KafkaProducerConfig struct {
	BatchSize        int  `yaml:"batch_size"`         // 每批最大消息数，默认 1000
	BatchTimeoutMs   int  `yaml:"batch_timeout_ms"`   // 批量等待超时(毫秒)，默认 5
	BufferSize       int  `yaml:"buffer_size"`        // 缓冲队列大小
	SenderCount      int  `yaml:"sender_count"`       // 发送协程数，默认 100
	MaxRetries       int  `yaml:"max_retries"`        // 最大重试次数，默认 3
	RateLimitEnabled bool `yaml:"rate_limit_enabled"` // 是否启用生产者限流
	RateLimitQPS     int  `yaml:"rate_limit_qps"`     // 限流速率（每秒消息数）
	RateLimitBurst   int  `yaml:"rate_limit_burst"`   // 突发容量（允许的最大突发消息数）
}

type KafkaConsumerConfig struct {
	ConsumerCount    int `yaml:"consumer_count"`     // 消费协程数，默认 64
	BatchWriterCount int `yaml:"batch_writer_count"` // 写入协程数，默认 32
	BatchSize        int `yaml:"batch_size"`         // 批量写入大小，默认 1000
	BatchQueueSize   int `yaml:"batch_queue_size"`   // 批量队列容量，默认 200000
	BatchFlushMs     int `yaml:"batch_flush_ms"`     // 批量刷新间隔(毫秒)，默认 50
	FetchBatchSize   int `yaml:"fetch_batch_size"`   // 每次拉取消息数，默认 2000
	FetchTimeoutMs   int `yaml:"fetch_timeout_ms"`   // 拉取超时(毫秒)，默认 50
	MinBytes         int `yaml:"min_bytes"`          // 最小拉取字节数，默认 1
	MaxBytes         int `yaml:"max_bytes"`          // 最大拉取字节数，默认 10MB
	CommitIntervalMs int `yaml:"commit_interval_ms"` // 自动提交间隔(毫秒)，默认 1000
}

type TimeoutConfig struct {
	OrderTimeoutSeconds int    `yaml:"order_timeout_seconds"`  // 订单超时时间（秒），默认60
	RedisScanInterval   string `yaml:"redis_scan_interval"`    // Redis 扫描间隔，默认 500ms
	MySQLScanInterval   string `yaml:"mysql_scan_interval"`    // MySQL 兜底扫描间隔，默认 5m
	RedisBatchSize      int    `yaml:"redis_batch_size"`       // Redis 扫描批量大小，默认 2000
	MySQLBatchSize      int    `yaml:"mysql_batch_size"`       // MySQL 扫描批量大小，默认 100
	MaxRetryDelayMs     int    `yaml:"max_retry_delay_ms"`     // 最大重试延迟(毫秒)，默认 5000
	WarmupLockExpireSec int    `yaml:"warmup_lock_expire_sec"` // 预热锁过期时间(秒)，默认 30
}

var Cfg *Config

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := applyEnvOverrides(cfg); err != nil {
		return nil, err
	}

	Cfg = cfg
	return cfg, nil
}

func applyEnvOverrides(cfg *Config) error {
	if err := setIntFromEnv("SERVER_PORT", &cfg.Server.Port); err != nil {
		return err
	}
	if err := setBoolFromEnv("STARTUP_FLUSH_REDIS_ON_START", &cfg.Startup.FlushRedisOnStart); err != nil {
		return err
	}

	if v, ok := os.LookupEnv("MYSQL_HOST"); ok {
		cfg.MySQL.Host = v
	}
	if err := setIntFromEnv("MYSQL_PORT", &cfg.MySQL.Port); err != nil {
		return err
	}
	if v, ok := os.LookupEnv("MYSQL_DATABASE"); ok {
		cfg.MySQL.Database = v
	}
	if v, ok := os.LookupEnv("MYSQL_USER"); ok {
		cfg.MySQL.Username = v
	}
	if v, ok := os.LookupEnv("MYSQL_PASSWORD"); ok {
		cfg.MySQL.Password = v
	}

	if v, ok := os.LookupEnv("REDIS_ADDR"); ok {
		cfg.Redis.Addr = v
	}
	if v, ok := os.LookupEnv("REDIS_PASSWORD"); ok {
		cfg.Redis.Password = v
	}
	if err := setIntFromEnv("REDIS_DB", &cfg.Redis.DB); err != nil {
		return err
	}

	if v, ok := os.LookupEnv("KAFKA_BROKERS"); ok {
		brokers := splitAndTrim(v, ",")
		if len(brokers) == 0 {
			return fmt.Errorf("invalid KAFKA_BROKERS: empty brokers")
		}
		cfg.Kafka.Brokers = brokers
	}
	if v, ok := os.LookupEnv("KAFKA_TOPIC"); ok {
		cfg.Kafka.Topic = v
	}
	if v, ok := os.LookupEnv("KAFKA_GROUP"); ok {
		cfg.Kafka.Group = v
	}

	jwtSecret, err := requireNonEmptyEnv("JWT_SECRET")
	if err != nil {
		return err
	}
	cfg.JWT.Secret = jwtSecret

	if err := setIntFromEnv("JWT_EXPIRE_HOURS", &cfg.JWT.ExpireHours); err != nil {
		return err
	}

	adminUsername, err := requireNonEmptyEnv("ADMIN_USERNAME")
	if err != nil {
		return err
	}
	cfg.Admin.Username = adminUsername

	adminPassword, err := requireNonEmptyEnv("ADMIN_PASSWORD")
	if err != nil {
		return err
	}
	cfg.Admin.Password = adminPassword

	if v, ok := os.LookupEnv("EMAIL_SMTP_HOST"); ok {
		cfg.Email.Host = v
	}
	if err := setIntFromEnv("EMAIL_SMTP_PORT", &cfg.Email.Port); err != nil {
		return err
	}
	if v, ok := os.LookupEnv("EMAIL_USERNAME"); ok {
		cfg.Email.Username = v
	}
	if v, ok := os.LookupEnv("EMAIL_PASSWORD"); ok {
		cfg.Email.Password = v
	}
	if v, ok := os.LookupEnv("EMAIL_FROM"); ok {
		cfg.Email.From = v
	}
	if v, ok := os.LookupEnv("EMAIL_FROM_NAME"); ok {
		cfg.Email.FromName = v
	}
	if v, ok := os.LookupEnv("EMAIL_ENCRYPTION"); ok {
		cfg.Email.Encryption = v
	}
	if err := setIntFromEnv("EMAIL_CODE_LENGTH", &cfg.Email.CodeLength); err != nil {
		return err
	}
	if v, ok := os.LookupEnv("EMAIL_CODE_TTL"); ok {
		cfg.Email.CodeTTL = v
	}
	if v, ok := os.LookupEnv("EMAIL_SEND_INTERVAL"); ok {
		cfg.Email.SendInterval = v
	}
	if v, ok := os.LookupEnv("EMAIL_SUBJECT"); ok {
		cfg.Email.Subject = v
	}

	return nil
}

func requireNonEmptyEnv(name string) (string, error) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("%s is required and must be provided via environment (e.g. .env)", name)
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%s is required and cannot be empty", name)
	}
	return v, nil
}

func setIntFromEnv(name string, target *int) error {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}
	*target = n
	return nil
}

func setBoolFromEnv(name string, target *bool) error {
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}
	*target = b
	return nil
}

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
