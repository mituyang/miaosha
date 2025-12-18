package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Redis     RedisConfig     `yaml:"redis"`
	Kafka     KafkaConfig     `yaml:"kafka"`
	Timeout   TimeoutConfig   `yaml:"timeout"`
	JWT       JWTConfig       `yaml:"jwt"`
	Admin     AdminConfig     `yaml:"admin"`
	Snowflake SnowflakeConfig `yaml:"snowflake"`
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
	Secret string `yaml:"secret"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type MySQLConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Database     string `yaml:"database"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	SegmentCount int    `yaml:"segment_count"` // 库存分段数量，默认 32
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
	BatchSize      int `yaml:"batch_size"`       // 每批最大消息数，默认 1000
	BatchTimeoutMs int `yaml:"batch_timeout_ms"` // 批量等待超时(毫秒)，默认 5
	BufferSize     int `yaml:"buffer_size"`      // 缓冲队列大小，默认 200000
	SenderCount    int `yaml:"sender_count"`     // 发送协程数，默认 100
	MaxRetries     int `yaml:"max_retries"`      // 最大重试次数，默认 3
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

	Cfg = cfg
	return cfg, nil
}
