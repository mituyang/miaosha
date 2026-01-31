package redis

import (
	"context"
	"fmt"
	"time"
)

// RateLimitKey 生成限流 key
func RateLimitKey(userID uint64) string {
	return fmt.Sprintf("ratelimit:user:%d", userID)
}

// CheckRateLimit 检查用户是否被限流
// 返回: true=通过, false=限流
func CheckRateLimit(ctx context.Context, userID uint64, rate, capacity, expireSec int) (bool, error) {
	key := RateLimitKey(userID)
	now := time.Now().Unix()

	result, err := evalShaIntWithRetry(ctx, tokenBucketSHA, []string{key}, now, rate, capacity, expireSec)
	if err != nil {
		return false, err
	}

	return result == 1, nil
}
