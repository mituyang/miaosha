package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"seckill/pkg/jwt"
	redisPkg "seckill/pkg/redis"
)

// JWTAuth JWT 认证中间件
func JWTAuth(j *jwt.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "缺少登录凭证"})
			c.Abort()
			return
		}

		// Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "登录凭证格式错误"})
			c.Abort()
			return
		}

		claims, err := j.ParseToken(parts[1])
		if err != nil {
			msg := "登录凭证无效"
			if err == jwt.ErrTokenExpired {
				msg = "登录凭证已过期"
			}
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": msg})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		enabled, err := redisPkg.IsUserEnabled(c.Request.Context(), claims.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "用户状态校验失败"})
			c.Abort()
			return
		}
		if !enabled {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "用户已被禁用"})
			c.Abort()
			return
		}

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
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权访问后台"})
			c.Abort()
			return
		}
		c.Next()
	}
}
