package redis

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const captchaKeyPrefix = "auth:captcha:"

func captchaKey(captchaID string) string {
	return captchaKeyPrefix + captchaID
}

// SetCaptcha 保存验证码，默认 5 分钟过期。
func SetCaptcha(ctx context.Context, captchaID, code string) error {
	return Client.Set(ctx, captchaKey(captchaID), strings.TrimSpace(code), 5*time.Minute).Err()
}

// VerifyAndDeleteCaptcha 校验验证码并在成功后删除。
func VerifyAndDeleteCaptcha(ctx context.Context, captchaID, code string) error {
	key := captchaKey(captchaID)
	savedCode, err := Client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("get captcha failed: %w", err)
	}

	if strings.EqualFold(strings.TrimSpace(savedCode), strings.TrimSpace(code)) {
		if err := Client.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("delete captcha failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("captcha mismatch")
}
