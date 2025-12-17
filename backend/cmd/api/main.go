package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"seckill/internal/config"
	"seckill/internal/router"
	"seckill/internal/service"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/mq"
	"seckill/pkg/redis"
	"seckill/pkg/util"
)

// warmUpStock 启动时预热库存
func warmUpStock(cfg *config.Config) {
	svc := service.NewSeckillService(cfg)
	count, err := svc.WarmUpAll(context.Background())
	if err != nil {
		logger.Error.Printf("warmup stock failed: %v", err)
		return
	}
	logger.Info.Printf("Stock warmup completed: %d goods", count)
}

func main() {
	// 设置时区为东八区
	loc, _ := time.LoadLocation("Asia/Shanghai")
	time.Local = loc

	// 0. 初始化日志
	logFile, err := logger.Init("api")
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

	// 4. 清空 Redis 数据
	if err := redis.FlushAll(context.Background()); err != nil {
		logger.Error.Printf("flush redis failed: %v", err)
	} else {
		logger.Info.Println("Redis data cleared")
	}

	// 5. 加载 Lua 脚本
	if err := redis.LoadScript(context.Background()); err != nil {
		logger.Error.Fatalf("load lua script failed: %v", err)
	}
	logger.Info.Println("Lua script loaded")

	// 6. 初始化 Kafka Producer
	if err := mq.InitKafkaProducer(&cfg.Kafka); err != nil {
		logger.Error.Fatalf("init kafka producer failed: %v", err)
	}
	logger.Info.Println("Kafka producer started")

	// 7. 初始化雪花算法
	_ = util.InitSnowflake(1)

	// 8. 启动时预热库存
	warmUpStock(cfg)

	// 9. 启动 HTTP 服务
	r := router.Setup(cfg)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		logger.Info.Printf("API server starting on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error.Fatalf("listen failed: %v", err)
		}
	}()

	// 10. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error.Printf("Server shutdown error: %v", err)
	}

	_ = mq.CloseKafkaProducer()
	_ = redis.Close()
	_ = database.Close()

	logger.Info.Println("Server exited")
}
