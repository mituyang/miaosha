package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 基于令牌桶的限流器
type RateLimiter struct {
	rate       float64   // 每秒生成的令牌数
	capacity   float64   // 桶容量
	tokens     float64   // 当前令牌数
	lastUpdate time.Time // 上次更新时间
	mu         sync.Mutex
}

// NewRateLimiter 创建限流器
// rate: 每秒允许的请求数
// capacity: 突发容量
func NewRateLimiter(rate, capacity float64) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		capacity:   capacity,
		tokens:     capacity,
		lastUpdate: time.Now(),
	}
}

// Allow 判断是否允许请求通过
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	// 计算从上次更新到现在应该生成的令牌数
	elapsed := now.Sub(r.lastUpdate).Seconds()
	r.tokens += elapsed * r.rate

	// 令牌数不能超过桶容量
	if r.tokens > r.capacity {
		r.tokens = r.capacity
	}
	r.lastUpdate = now

	// 尝试获取一个令牌
	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后重试",
			})
			return
		}
		c.Next()
	}
}

// IPRateLimiter 基于 IP 的限流器
type IPRateLimiter struct {
	limiters map[string]*RateLimiter
	rate     float64
	capacity float64
	mu       sync.RWMutex
}

// NewIPRateLimiter 创建基于 IP 的限流器
func NewIPRateLimiter(rate, capacity float64) *IPRateLimiter {
	return &IPRateLimiter{
		limiters: make(map[string]*RateLimiter),
		rate:     rate,
		capacity: capacity,
	}
}

// GetLimiter 获取指定 IP 的限流器
func (i *IPRateLimiter) GetLimiter(ip string) *RateLimiter {
	i.mu.RLock()
	limiter, exists := i.limiters[ip]
	i.mu.RUnlock()

	if exists {
		return limiter
	}

	// 不存在则创建新的限流器
	i.mu.Lock()
	defer i.mu.Unlock()

	// 双重检查
	if limiter, exists = i.limiters[ip]; exists {
		return limiter
	}

	limiter = NewRateLimiter(i.rate, i.capacity)
	i.limiters[ip] = limiter
	return limiter
}

// IPRateLimitMiddleware 基于 IP 的限流中间件
func IPRateLimitMiddleware(ipLimiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		limiter := ipLimiter.GetLimiter(ip)

		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后重试",
			})
			return
		}
		c.Next()
	}
}
