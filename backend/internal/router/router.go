package router

import (
	"github.com/gin-gonic/gin"

	"seckill/internal/config"
	"seckill/internal/handler"
	"seckill/internal/middleware"
	"seckill/internal/service"
	"seckill/pkg/jwt"
)

func Setup(cfg *config.Config) *gin.Engine {
	r := gin.Default()

	// 初始化 JWT
	jwtInstance := jwt.NewJWT(cfg.JWT.Secret, cfg.JWT.ExpireHours)

	// 初始化 service 和 handler
	seckillSvc := service.NewSeckillService(cfg)
	seckillHandler := handler.NewSeckillHandler(seckillSvc)

	authSvc := service.NewAuthService(jwtInstance)
	authHandler := handler.NewAuthHandler(authSvc)

	orderSvc := service.NewOrderService()
	orderHandler := handler.NewOrderHandler(orderSvc)

	// API 路由组
	api := r.Group("/api")
	{
		// 认证接口 (公开)
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		// 秒杀接口
		seckill := api.Group("/seckill")
		{
			// 公开接口
			seckill.GET("/stock/:goods_id", seckillHandler.GetStock)

			// 需要认证的接口（带限流）
			seckill.POST("/buy", middleware.JWTAuth(jwtInstance), middleware.RateLimit(cfg), seckillHandler.DoSeckill)
		}

		// 订单接口 (需要认证，带限流)
		orders := api.Group("/orders", middleware.JWTAuth(jwtInstance), middleware.RateLimit(cfg))
		{
			orders.GET("", orderHandler.GetOrders)
			orders.POST("/:order_id/pay", orderHandler.PayOrder)
			orders.POST("/:order_id/cancel", orderHandler.CancelOrder)
		}

		// 管理员接口 (Header 校验)
		admin := api.Group("/admin", middleware.AdminAuth(cfg.Admin.Secret))
		{
			admin.POST("/warmup", seckillHandler.WarmUpAll)
			admin.POST("/warmup/:goods_id", seckillHandler.WarmUp)
		}
	}

	return r
}
