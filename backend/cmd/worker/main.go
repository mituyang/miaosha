package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seckill/internal/config"
	"seckill/internal/worker"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"
	"seckill/pkg/util"
)

func main() {
	// 设置时区为东八区
	loc, _ := time.LoadLocation("Asia/Shanghai")
	time.Local = loc

	// 0. 初始化日志
	logFile, err := logger.Init("worker")
	if err != nil {
		panic(err)
	}
	defer logFile.Close()

	// 1. 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		logger.Error.Fatalf("load config failed: %v", err)
	}

	// 2. 初始化 MySQL
	if err := database.Init(&cfg.MySQL); err != nil {
		logger.Error.Fatalf("init mysql failed: %v", err)
	}
	logger.Info.Println("MySQL connected")

	if err := database.EnsureSchema(); err != nil {
		logger.Error.Fatalf("ensure schema failed: %v", err)
	}
	logger.Info.Println("MySQL schema ensured")

	// 3. 初始化 Redis
	if err := redis.Init(&cfg.Redis); err != nil {
		logger.Error.Fatalf("init redis failed: %v", err)
	}
	logger.Info.Println("Redis connected")

	// 4. 加载延迟队列 Lua 脚本
	if err := redis.LoadDelayQueueScript(context.Background()); err != nil {
		logger.Error.Fatalf("load delay queue script failed: %v", err)
	}
	logger.Info.Println("Delay queue script loaded")

	// 5. 初始化 Redis 配置
	redis.SetSegmentCount(cfg.Redis.SegmentCount)

	// 6. 初始化雪花算法
	workerID := cfg.Snowflake.WorkerID
	if workerID <= 0 {
		workerID = 2 // Worker 默认使用 2
	}
	_ = util.InitSnowflakeWithEpoch(workerID, cfg.Snowflake.Epoch)

	// 7. 启动 Kafka 消费者
	kafkaConsumer := worker.NewKafkaConsumer(cfg)
	if err := kafkaConsumer.Start(); err != nil {
		logger.Error.Fatalf("start kafka consumer failed: %v", err)
	}

	// 8. 启动 Redis 超时扫描器
	redisScanner := worker.NewRedisTimeoutScanner(cfg)
	redisScanner.Start()

	// 9. 启动 MySQL 兜底扫描器
	mysqlScanner := worker.NewMySQLTimeoutScanner(cfg)
	mysqlScanner.Start()

	// 10. 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info.Println("Shutting down worker...")

	// 按顺序停止各组件
	mysqlScanner.Stop()
	redisScanner.Stop()
	_ = kafkaConsumer.Stop()
	_ = redis.Close()
	_ = database.Close()

	logger.Info.Println("Worker exited")
}
