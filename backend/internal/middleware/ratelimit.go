package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"seckill/internal/config"
	"seckill/pkg/redis"
)

// RateLimit 用户级限流中间件（令牌桶算法）
func RateLimit(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 未启用限流，直接放行
		if !cfg.RateLimit.Enabled {
			c.Next()
			return
		}

		// 获取用户 ID（由 JWT 中间件设置）
		userIDVal, exists := c.Get("user_id")
		if !exists {
			// 未登录用户不限流（或可以改为按 IP 限流）
			c.Next()
			return
		}

		userID, ok := userIDVal.(uint64)
		if !ok {
			c.Next()
			return
		}

		// 检查限流
		allowed, err := redis.CheckRateLimit(
			c.Request.Context(),
			userID,
			cfg.RateLimit.Rate,
			cfg.RateLimit.Capacity,
			cfg.RateLimit.ExpireSec,
		)

		if err != nil {
			// Redis 错误，降级放行（避免影响业务）
			c.Next()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code": 429,
				"msg":  "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
