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
	activitySvc := service.NewActivityService(cfg, seckillSvc)
	activityHandler := handler.NewActivityHandler(activitySvc, seckillSvc)
	goodsSvc := service.NewGoodsService()
	goodsHandler := handler.NewGoodsHandler(goodsSvc)

	authSvc := service.NewAuthService(jwtInstance, cfg.Email)
	authHandler := handler.NewAuthHandler(authSvc)

	orderSvc := service.NewOrderService()
	orderHandler := handler.NewOrderHandler(orderSvc)

	adminSvc := service.NewAdminService(seckillSvc, activitySvc, jwtInstance)
	adminHandler := handler.NewAdminHandler(adminSvc)

	// API 路由组
	api := r.Group("/api")
	{
		// 认证接口 (公开)
		auth := api.Group("/auth")
		{
			auth.GET("/captcha", authHandler.GetCaptcha)
			auth.POST("/email-code", authHandler.SendEmailCode)
			auth.POST("/password-reset/email-code", authHandler.SendPasswordResetEmailCode)
			auth.POST("/password-reset", authHandler.ResetPassword)
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		// 秒杀接口
		seckill := api.Group("/seckill")
		{
			// 公开接口
			seckill.GET("/stock/:goods_id", seckillHandler.GetStock)
			seckill.GET("/activity/:activity_id/stock", activityHandler.GetActivityStock)

			// 需要认证的接口（带限流）
			seckill.POST("/buy", middleware.JWTAuth(jwtInstance), middleware.RateLimit(cfg), seckillHandler.DoSeckill)
		}

		// 商品公开接口
		goods := api.Group("/goods")
		{
			goods.GET("", goodsHandler.ListOnSaleGoods)
		}

		activities := api.Group("/activities")
		{
			activities.GET("", activityHandler.ListPublicActivities)
		}

		// 订单接口 (需要认证，带限流)
		orders := api.Group("/orders", middleware.JWTAuth(jwtInstance), middleware.RateLimit(cfg))
		{
			orders.GET("", orderHandler.GetOrders)
			orders.POST("/:order_id/pay", orderHandler.PayOrder)
			orders.POST("/:order_id/cancel", orderHandler.CancelOrder)
		}

		// 管理员接口
		admin := api.Group("/admin")
		{
			admin.POST("/login", adminHandler.Login)

			adminAuth := admin.Group("", middleware.AdminJWTAuth(jwtInstance))
			{
				adminAuth.GET("/ping", adminHandler.Ping)
				adminAuth.GET("/goods", adminHandler.ListGoods)
				adminAuth.POST("/goods", adminHandler.CreateGoods)
				adminAuth.PUT("/goods/:goods_id", adminHandler.UpdateGoods)
				adminAuth.DELETE("/goods/:goods_id", adminHandler.DeleteGoods)

				adminAuth.GET("/activities", activityHandler.ListActivities)
				adminAuth.POST("/activities", activityHandler.CreateActivity)
				adminAuth.PUT("/activities/:activity_id", activityHandler.UpdateActivity)
				adminAuth.POST("/activities/:activity_id/warmup", activityHandler.WarmUpActivity)
				adminAuth.PUT("/activities/:activity_id/status", activityHandler.UpdateActivityStatus)

				adminAuth.GET("/orders", adminHandler.ListOrders)
				adminAuth.GET("/orders/:order_id", adminHandler.GetOrderDetail)

				adminAuth.GET("/users", adminHandler.ListUsers)
				adminAuth.PUT("/users/:user_id/status", adminHandler.UpdateUserStatus)

				adminAuth.POST("/warmup", adminHandler.WarmUpAll)
				adminAuth.POST("/warmup/:goods_id", adminHandler.WarmUpGoods)

				adminAuth.GET("/stats", adminHandler.GetStats)
				adminAuth.POST("/stats/rebuild", adminHandler.RebuildStats)
				adminAuth.GET("/observability", adminHandler.GetObservability)
			}
		}
	}

	return r
}
