package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"seckill-system/internal/handler"
	"seckill-system/internal/middleware"
	"seckill-system/pkg/config"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建 Gin 引擎
	r := gin.Default()

	// 创建限流器: 全局每秒 10000 请求，突发容量 20000
	globalLimiter := middleware.NewRateLimiter(10000, 20000)
	// 基于 IP 的限流: 每个 IP 每秒 10 请求，突发容量 20
	ipLimiter := middleware.NewIPRateLimiter(10, 20)

	// 应用全局限流中间件
	r.Use(middleware.RateLimitMiddleware(globalLimiter))

	// 创建秒杀 Handler，连接 gRPC 服务
	grpcAddr := "localhost:" + cfg.GRPC.Port
	seckillHandler, err := handler.NewSeckillHandler(grpcAddr)
	if err != nil {
		log.Fatalf("连接 gRPC 服务失败: %v", err)
	}

	// 秒杀相关路由组
	seckillGroup := r.Group("/api/seckill")
	{
		// 秒杀接口，应用 IP 限流
		seckillGroup.POST("/do", middleware.IPRateLimitMiddleware(ipLimiter), seckillHandler.DoSeckill)
		// 查询秒杀结果
		seckillGroup.GET("/result", seckillHandler.GetResult)
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Printf("API Gateway 启动在 :%s...", cfg.HTTP.Port)
	if err := r.Run(":" + cfg.HTTP.Port); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}
