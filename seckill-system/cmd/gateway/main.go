package main

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"seckill-system/internal/handler"
	"seckill-system/internal/middleware"
	"seckill-system/pkg/config"
	"seckill-system/pkg/crypto"
	"seckill-system/pkg/email"
	"seckill-system/pkg/kafka"
	"seckill-system/pkg/redis"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化 Redis（用于获取库存）
	if err := redis.InitRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB); err != nil {
		log.Fatalf("初始化 Redis 失败: %v", err)
	}
	log.Println("Redis 连接成功")

	// 初始化数据库（用于获取商品信息）
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 配置连接池，提升高并发性能
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("获取数据库连接池失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(100)                // 最大打开连接数
	sqlDB.SetMaxIdleConns(20)                 // 最大空闲连接数
	sqlDB.SetConnMaxLifetime(5 * time.Minute) // 连接最大存活时间

	log.Println("数据库连接成功")

	// 初始化邮件模块
	email.Init(&email.Config{
		Host:     cfg.Email.Host,
		Port:     cfg.Email.Port,
		Username: cfg.Email.Username,
		Password: cfg.Email.Password,
		From:     cfg.Email.From,
	}, redis.Client)
	log.Println("邮件模块初始化成功")

	// 初始化 RSA 加密模块（用于密码加密传输）
	if err := crypto.Init(); err != nil {
		log.Fatalf("初始化 RSA 加密模块失败: %v", err)
	}
	log.Println("RSA 加密模块初始化成功")

	// 初始化 Kafka Producer（用于发送订单状态更新消息）
	kafka.InitProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer kafka.Close()
	log.Println("Kafka Producer 初始化成功")

	// 创建 Gin 引擎
	r := gin.Default()

	// 创建限流器: 全局每秒 10000 请求，突发容量 20000
	globalLimiter := middleware.NewRateLimiter(10000, 20000)
	// 基于 IP 的限流: 每个 IP 每秒请求数，突发容量
	// 生产环境建议: (10, 20)，测试时可调大
	ipLimiter := middleware.NewIPRateLimiter(10000, 20000)

	// 应用全局限流中间件
	r.Use(middleware.RateLimitMiddleware(globalLimiter))

	// 创建 Handler
	grpcAddr := "localhost:" + cfg.GRPC.Port
	seckillHandler, err := handler.NewSeckillHandler(grpcAddr, db)
	if err != nil {
		log.Fatalf("连接 gRPC 服务失败: %v", err)
	}
	userHandler := handler.NewUserHandler(db)

	// 用户相关路由（无需登录）
	userGroup := r.Group("/api/user")
	{
		userGroup.GET("/public-key", userHandler.GetPublicKey)        // 获取 RSA 公钥
		userGroup.POST("/send-code", userHandler.SendVerifyCode)      // 发送注册验证码
		userGroup.POST("/send-login-code", userHandler.SendLoginCode) // 发送登录验证码
		userGroup.POST("/send-reset-code", userHandler.SendResetCode) // 发送重置密码验证码
		userGroup.POST("/register", userHandler.Register)             // 注册
		userGroup.POST("/login", userHandler.Login)                   // 邮箱密码登录
		userGroup.POST("/login-by-code", userHandler.LoginByCode)     // 验证码登录
		userGroup.POST("/reset-password", userHandler.ResetPassword)  // 重置密码
	}

	// 需要登录的用户接口
	authUserGroup := r.Group("/api/user")
	authUserGroup.Use(middleware.AuthMiddleware())
	{
		authUserGroup.GET("/info", userHandler.GetUserInfo)
	}

	// 秒杀相关路由组
	seckillGroup := r.Group("/api/seckill")
	{
		// 获取商品列表（无需登录）
		seckillGroup.GET("/goods", seckillHandler.GetGoodsList)
		// 获取秒杀记录（无需登录，公开展示）
		seckillGroup.GET("/records", seckillHandler.GetSeckillRecords)
		// 重置库存（管理接口，生产环境应加管理员鉴权）
		seckillGroup.POST("/reset", seckillHandler.ResetStock)
	}

	// 需要登录的秒杀接口
	authSeckillGroup := r.Group("/api/seckill")
	authSeckillGroup.Use(middleware.AuthMiddleware())
	{
		// 秒杀接口，需要登录 + IP 限流
		authSeckillGroup.POST("/do", middleware.IPRateLimitMiddleware(ipLimiter), seckillHandler.DoSeckill)
		// 查询秒杀结果
		authSeckillGroup.GET("/result", seckillHandler.GetResult)
		// 获取我的订单列表
		authSeckillGroup.GET("/my-orders", seckillHandler.GetMyOrders)
		// 获取订单详情
		authSeckillGroup.GET("/order-detail", seckillHandler.GetOrderDetail)
		// 支付订单
		authSeckillGroup.POST("/pay", seckillHandler.PayOrder)
		// 取消订单
		authSeckillGroup.POST("/cancel", seckillHandler.CancelOrder)
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
