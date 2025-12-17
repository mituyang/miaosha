package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	MySQL   MySQLConfig   `yaml:"mysql"`
	Redis   RedisConfig   `yaml:"redis"`
	Kafka   KafkaConfig   `yaml:"kafka"`
	Timeout TimeoutConfig `yaml:"timeout"`
	JWT     JWTConfig     `yaml:"jwt"`
	Admin   AdminConfig   `yaml:"admin"`
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
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type KafkaConfig struct {
	Brokers []string `yaml:"brokers"`
	Topic   string   `yaml:"topic"`
	Group   string   `yaml:"group"`
}

type TimeoutConfig struct {
	OrderTimeoutSeconds int    `yaml:"order_timeout_seconds"` // 订单超时时间（秒），默认60
	RedisScanInterval   string `yaml:"redis_scan_interval"`   // Redis 扫描间隔，默认 1s
	MySQLScanInterval   string `yaml:"mysql_scan_interval"`   // MySQL 兜底扫描间隔，默认 5m
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
