package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"seckill/pkg/jwt"
)

// JWTAuth JWT 认证中间件
func JWTAuth(j *jwt.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "missing token"})
			c.Abort()
			return
		}

		// Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid token format"})
			c.Abort()
			return
		}

		claims, err := j.ParseToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": err.Error()})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

// AdminAuth 管理员认证中间件 (Header 校验)
func AdminAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminSecret := c.GetHeader("X-Admin-Secret")
		if adminSecret != secret {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "forbidden"})
			c.Abort()
			return
		}
		c.Next()
	}
}
