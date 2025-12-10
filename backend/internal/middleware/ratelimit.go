package middleware

import "github.com/gin-gonic/gin"

// RateLimit 限流中间件 (简单实现，生产环境建议用令牌桶)
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 实现限流逻辑 (如: 令牌桶、滑动窗口)
		c.Next()
	}
}
