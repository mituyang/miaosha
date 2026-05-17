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

// warmUpAdminStats 启动时预热后台统计快照
func warmUpAdminStats(cfg *config.Config) {
	seckillSvc := service.NewSeckillService(cfg)
	adminSvc := service.NewAdminService(seckillSvc, service.NewActivityService(cfg, seckillSvc), nil)
	if err := adminSvc.WarmStatsCache(context.Background()); err != nil {
		logger.Error.Printf("warmup admin stats failed: %v", err)
		return
	}
	logger.Info.Println("Admin stats snapshot warmed")
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

	if err := database.EnsureSchema(); err != nil {
		logger.Error.Fatalf("ensure schema failed: %v", err)
	}
	logger.Info.Println("MySQL schema ensured")

	if err := service.EnsureAdminAccount(cfg.Admin); err != nil {
		logger.Error.Fatalf("ensure admin account failed: %v", err)
	}
	logger.Info.Println("Admin account ensured")

	if err := service.EnsureDefaultActivities(cfg); err != nil {
		logger.Error.Fatalf("ensure default activities failed: %v", err)
	}
	logger.Info.Println("Default seckill activities ensured")

	// 3. 初始化 Redis
	if err := redis.Init(&cfg.Redis); err != nil {
		logger.Error.Fatalf("init redis failed: %v", err)
	}
	logger.Info.Println("Redis connected")

	// 4. 按配置决定是否清空 Redis 数据（默认关闭，防止误删）
	if cfg.Startup.FlushRedisOnStart {
		if err := redis.FlushAll(context.Background()); err != nil {
			logger.Error.Printf("flush redis failed: %v", err)
		} else {
			logger.Warn.Println("Redis data cleared by startup flag")
		}
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
	workerID := cfg.Snowflake.WorkerID
	if workerID <= 0 {
		workerID = 1
	}
	_ = util.InitSnowflakeWithEpoch(workerID, cfg.Snowflake.Epoch)

	// 8. 初始化 Redis 配置
	redis.SetWarmupLockExpire(cfg.Timeout.WarmupLockExpireSec)
	redis.SetSegmentCount(cfg.Redis.SegmentCount)

	// 9. 启动时预热库存
	warmUpStock(cfg)

	// 10. 启动时预热后台统计快照
	warmUpAdminStats(cfg)

	// 11. 启动 HTTP 服务
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

	// 12. 优雅关闭
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
