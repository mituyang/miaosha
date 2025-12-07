package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config 全局配置结构
type Config struct {
	MySQL MySQLConfig
	Redis RedisConfig
	Kafka KafkaConfig
	GRPC  GRPCConfig
	HTTP  HTTPConfig
	Email EmailConfig
}

// EmailConfig 邮件配置
type EmailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// MySQLConfig MySQL 配置
type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

// GRPCConfig gRPC 服务配置
type GRPCConfig struct {
	Port string
}

// HTTPConfig HTTP 网关配置
type HTTPConfig struct {
	Port string
}

// DSN 生成 MySQL 连接字符串
func (m *MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database)
}

var Cfg *Config

// Load 加载配置，从 .env 文件读取
func Load() (*Config, error) {
	// 尝试加载 .env 文件（如果存在）
	// 生产环境可以直接使用环境变量，不依赖 .env 文件
	_ = godotenv.Load()             // 当前目录
	_ = godotenv.Load("../.env")    // 从 seckill-system 目录运行时，加载 miaosha/.env
	_ = godotenv.Load("../../.env") // 从 cmd/xxx 子目录运行时

	mysqlPort, _ := strconv.Atoi(getEnv("MYSQL_PORT", "3306"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	smtpPort, _ := strconv.Atoi(getEnv("EMAIL_PORT", "587"))

	Cfg = &Config{
		MySQL: MySQLConfig{
			Host:     getEnv("MYSQL_HOST", "localhost"),
			Port:     mysqlPort,
			User:     getEnv("MYSQL_USER", "root"),
			Password: getEnv("MYSQL_PASSWORD", ""),
			Database: getEnv("MYSQL_DATABASE", "seckill"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		Kafka: KafkaConfig{
			Brokers: []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
			Topic:   getEnv("KAFKA_TOPIC", "seckill-orders"),
			GroupID: getEnv("KAFKA_GROUP_ID", "seckill-consumer-group"),
		},
		GRPC: GRPCConfig{
			Port: getEnv("GRPC_PORT", "50051"),
		},
		HTTP: HTTPConfig{
			Port: getEnv("HTTP_PORT", "8080"),
		},
		Email: EmailConfig{
			Host:     getEnv("EMAIL_HOST", "smtp.qq.com"),
			Port:     smtpPort,
			Username: getEnv("EMAIL_USERNAME", ""),
			Password: getEnv("EMAIL_PASSWORD", ""),
			From:     getEnv("EMAIL_FROM", "秒杀系统"),
		},
	}

	return Cfg, nil
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
