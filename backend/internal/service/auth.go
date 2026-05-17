package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"seckill/internal/config"
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/jwt"
	"seckill/pkg/redis"
)

var (
	ErrUserExists           = errors.New("username already exists")
	ErrEmailExists          = errors.New("email already exists")
	ErrEmailNotFound        = errors.New("email not found")
	ErrUserNotFound         = errors.New("user not found")
	ErrPasswordWrong        = errors.New("password incorrect")
	ErrUserDisabled         = errors.New("user disabled")
	ErrPasswordInvalid      = errors.New("password invalid")
	ErrEmailInvalid         = errors.New("email invalid")
	ErrEmailCodeInvalid     = errors.New("email code invalid")
	ErrEmailCodeExpired     = errors.New("email code expired")
	ErrEmailSendTooFrequent = errors.New("email code send too frequent")
	ErrEmailConfigInvalid   = errors.New("email config invalid")
)

type AuthService struct {
	userRepo *repository.UserRepository
	jwt      *jwt.JWT
	emailCfg config.EmailConfig
	mailer   *mailer
}

func NewAuthService(j *jwt.JWT, emailCfg config.EmailConfig) *AuthService {
	return &AuthService{
		userRepo: repository.NewUserRepository(),
		jwt:      j,
		emailCfg: emailCfg,
		mailer:   newMailer(emailCfg),
	}
}

// Register 用户注册
func (s *AuthService) Register(ctx context.Context, username, email, password, emailCode string) error {
	username = strings.TrimSpace(username)
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}

	// 检查用户是否存在
	_, err = s.userRepo.FindByUsername(username)
	if err == nil {
		return ErrUserExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	_, err = s.userRepo.FindByEmail(email)
	if err == nil {
		return ErrEmailExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if err := s.VerifyEmailCode(ctx, email, emailCode); err != nil {
		return err
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &model.User{
		Username: username,
		Email:    email,
		Password: string(hashedPassword),
		Status:   model.UserStatusEnabled,
	}

	if err := s.userRepo.Create(user); err != nil {
		return err
	}

	_ = redis.IncrementAdminUserCreated(ctx, user.Status)
	return redis.SetUserEnabled(ctx, user.ID, true)
}

// SendRegistrationEmailCode 发送注册邮箱验证码
func (s *AuthService) SendRegistrationEmailCode(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	_, err = s.userRepo.FindByEmail(email)
	if err == nil {
		return ErrEmailExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	interval, err := time.ParseDuration(s.emailCfg.SendInterval)
	if err != nil || interval <= 0 {
		return ErrEmailConfigInvalid
	}
	ttl, err := time.ParseDuration(s.emailCfg.CodeTTL)
	if err != nil || ttl <= 0 {
		return ErrEmailConfigInvalid
	}
	if s.emailCfg.CodeLength <= 0 {
		return ErrEmailConfigInvalid
	}

	locked, err := redis.SetEmailCodeSendLock(ctx, email, interval)
	if err != nil {
		return err
	}
	if !locked {
		return ErrEmailSendTooFrequent
	}

	code, err := randomDigits(s.emailCfg.CodeLength)
	if err != nil {
		_ = redis.ClearEmailCodeSendLock(ctx, email)
		return err
	}
	if err := redis.SetEmailCode(ctx, email, code, ttl); err != nil {
		_ = redis.ClearEmailCodeSendLock(ctx, email)
		return err
	}
	if err := s.mailer.sendVerificationCode(email, code, ttl, "注册"); err != nil {
		_ = redis.ClearEmailCodeSendLock(ctx, email)
		_ = redis.VerifyAndDeleteEmailCode(ctx, email, code)
		return err
	}
	return nil
}

// SendPasswordResetEmailCode 发送找回密码邮箱验证码。
func (s *AuthService) SendPasswordResetEmailCode(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	_, err = s.userRepo.FindByEmail(email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrEmailNotFound
	}
	if err != nil {
		return err
	}

	interval, err := time.ParseDuration(s.emailCfg.SendInterval)
	if err != nil || interval <= 0 {
		return ErrEmailConfigInvalid
	}
	ttl, err := time.ParseDuration(s.emailCfg.CodeTTL)
	if err != nil || ttl <= 0 {
		return ErrEmailConfigInvalid
	}
	if s.emailCfg.CodeLength <= 0 {
		return ErrEmailConfigInvalid
	}

	locked, err := redis.SetPasswordResetEmailCodeSendLock(ctx, email, interval)
	if err != nil {
		return err
	}
	if !locked {
		return ErrEmailSendTooFrequent
	}

	code, err := randomDigits(s.emailCfg.CodeLength)
	if err != nil {
		_ = redis.ClearPasswordResetEmailCodeSendLock(ctx, email)
		return err
	}
	if err := redis.SetPasswordResetEmailCode(ctx, email, code, ttl); err != nil {
		_ = redis.ClearPasswordResetEmailCodeSendLock(ctx, email)
		return err
	}
	if err := s.mailer.sendVerificationCode(email, code, ttl, "密码重置"); err != nil {
		_ = redis.ClearPasswordResetEmailCodeSendLock(ctx, email)
		_ = redis.VerifyAndDeletePasswordResetEmailCode(ctx, email, code)
		return err
	}
	return nil
}

// EmailSendIntervalSeconds 返回邮箱验证码发送间隔秒数。
func (s *AuthService) EmailSendIntervalSeconds() int {
	interval, err := time.ParseDuration(s.emailCfg.SendInterval)
	if err != nil || interval <= 0 {
		return 0
	}
	return int(interval.Seconds())
}

// EmailCodeTTLSeconds 返回邮箱验证码有效期秒数。
func (s *AuthService) EmailCodeTTLSeconds() int {
	ttl, err := time.ParseDuration(s.emailCfg.CodeTTL)
	if err != nil || ttl <= 0 {
		return 0
	}
	return int(ttl.Seconds())
}

// ResetPassword 通过邮箱验证码重置用户密码。
func (s *AuthService) ResetPassword(ctx context.Context, email, emailCode, password string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if len(password) < 6 {
		return ErrPasswordInvalid
	}

	user, err := s.userRepo.FindByEmail(email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrEmailNotFound
	}
	if err != nil {
		return err
	}
	if err := s.VerifyPasswordResetEmailCode(ctx, email, emailCode); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePassword(user.ID, string(hashedPassword))
}

// VerifyEmailCode 校验注册邮箱验证码
func (s *AuthService) VerifyEmailCode(ctx context.Context, email, code string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return ErrEmailCodeInvalid
	}
	if err := redis.VerifyAndDeleteEmailCode(ctx, email, code); err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			return ErrEmailCodeExpired
		}
		if strings.Contains(err.Error(), "email code mismatch") {
			return ErrEmailCodeInvalid
		}
		return err
	}
	return nil
}

// VerifyPasswordResetEmailCode 校验找回密码邮箱验证码。
func (s *AuthService) VerifyPasswordResetEmailCode(ctx context.Context, email, code string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return ErrEmailCodeInvalid
	}
	if err := redis.VerifyAndDeletePasswordResetEmailCode(ctx, email, code); err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			return ErrEmailCodeExpired
		}
		if strings.Contains(err.Error(), "password reset email code mismatch") {
			return ErrEmailCodeInvalid
		}
		return err
	}
	return nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", ErrEmailInvalid
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || strings.ToLower(addr.Address) != email {
		return "", ErrEmailInvalid
	}
	return email, nil
}

// Login 用户登录
func (s *AuthService) Login(username, password string) (string, error) {
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrUserNotFound
		}
		return "", err
	}

	if user.Status == model.UserStatusDisabled {
		return "", ErrUserDisabled
	}

	// 校验密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", ErrPasswordWrong
	}

	// 生成 Token
	return s.jwt.GenerateToken(user.ID, user.Username)
}
