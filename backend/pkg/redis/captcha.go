package redis

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const captchaKeyPrefix = "auth:captcha:"
const emailCodeKeyPrefix = "auth:email_code:"
const emailCodeSendLockPrefix = "auth:email_code_send_lock:"
const passwordResetEmailCodeKeyPrefix = "auth:password_reset_email_code:"
const passwordResetEmailCodeSendLockPrefix = "auth:password_reset_email_code_send_lock:"

func captchaKey(captchaID string) string {
	return captchaKeyPrefix + captchaID
}

func emailCodeKey(email string) string {
	return emailCodeKeyPrefix + strings.ToLower(strings.TrimSpace(email))
}

func emailCodeSendLockKey(email string) string {
	return emailCodeSendLockPrefix + strings.ToLower(strings.TrimSpace(email))
}

func passwordResetEmailCodeKey(email string) string {
	return passwordResetEmailCodeKeyPrefix + strings.ToLower(strings.TrimSpace(email))
}

func passwordResetEmailCodeSendLockKey(email string) string {
	return passwordResetEmailCodeSendLockPrefix + strings.ToLower(strings.TrimSpace(email))
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

// SetEmailCodeSendLock 设置邮箱验证码发送间隔锁。
func SetEmailCodeSendLock(ctx context.Context, email string, ttl time.Duration) (bool, error) {
	return Client.SetNX(ctx, emailCodeSendLockKey(email), "1", ttl).Result()
}

// SetEmailCode 保存邮箱验证码。
func SetEmailCode(ctx context.Context, email, code string, ttl time.Duration) error {
	return Client.Set(ctx, emailCodeKey(email), strings.TrimSpace(code), ttl).Err()
}

// ClearEmailCodeSendLock 清除邮箱验证码发送锁。
func ClearEmailCodeSendLock(ctx context.Context, email string) error {
	return Client.Del(ctx, emailCodeSendLockKey(email)).Err()
}

// VerifyAndDeleteEmailCode 校验邮箱验证码并在成功后删除。
func VerifyAndDeleteEmailCode(ctx context.Context, email, code string) error {
	key := emailCodeKey(email)
	savedCode, err := Client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("get email code failed: %w", err)
	}

	if strings.EqualFold(strings.TrimSpace(savedCode), strings.TrimSpace(code)) {
		if err := Client.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("delete email code failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("email code mismatch")
}

// SetPasswordResetEmailCodeSendLock 设置找回密码邮箱验证码发送间隔锁。
func SetPasswordResetEmailCodeSendLock(ctx context.Context, email string, ttl time.Duration) (bool, error) {
	return Client.SetNX(ctx, passwordResetEmailCodeSendLockKey(email), "1", ttl).Result()
}

// SetPasswordResetEmailCode 保存找回密码邮箱验证码。
func SetPasswordResetEmailCode(ctx context.Context, email, code string, ttl time.Duration) error {
	return Client.Set(ctx, passwordResetEmailCodeKey(email), strings.TrimSpace(code), ttl).Err()
}

// ClearPasswordResetEmailCodeSendLock 清除找回密码邮箱验证码发送锁。
func ClearPasswordResetEmailCodeSendLock(ctx context.Context, email string) error {
	return Client.Del(ctx, passwordResetEmailCodeSendLockKey(email)).Err()
}

// VerifyAndDeletePasswordResetEmailCode 校验找回密码邮箱验证码并在成功后删除。
func VerifyAndDeletePasswordResetEmailCode(ctx context.Context, email, code string) error {
	key := passwordResetEmailCodeKey(email)
	savedCode, err := Client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("get password reset email code failed: %w", err)
	}

	if strings.EqualFold(strings.TrimSpace(savedCode), strings.TrimSpace(code)) {
		if err := Client.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("delete password reset email code failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("password reset email code mismatch")
}
