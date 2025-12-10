package main

import (
	"os"
	"os/signal"
	"syscall"

	"seckill/internal/config"
	"seckill/internal/worker"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"
	"seckill/pkg/util"
)

func main() {
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

	// 3. 初始化 Redis
	if err := redis.Init(&cfg.Redis); err != nil {
		logger.Error.Fatalf("init redis failed: %v", err)
	}
	logger.Info.Println("Redis connected")

	// 4. 初始化雪花算法
	_ = util.InitSnowflake(2)

	// 5. 启动消费者
	c := worker.NewConsumer(cfg)
	if err := c.Start(); err != nil {
		logger.Error.Fatalf("start consumer failed: %v", err)
	}

	// 6. 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info.Println("Shutting down worker...")
	_ = c.Stop()
	_ = redis.Close()
	_ = database.Close()

	logger.Info.Println("Worker exited")
}
