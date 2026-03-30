package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/jwt"
	"seckill/pkg/redis"
)

var (
	ErrUserExists    = errors.New("username already exists")
	ErrUserNotFound  = errors.New("user not found")
	ErrPasswordWrong = errors.New("password incorrect")
	ErrUserDisabled  = errors.New("user disabled")
)

type AuthService struct {
	userRepo *repository.UserRepository
	jwt      *jwt.JWT
}

func NewAuthService(j *jwt.JWT) *AuthService {
	return &AuthService{
		userRepo: repository.NewUserRepository(),
		jwt:      j,
	}
}

// Register 用户注册
func (s *AuthService) Register(username, password string) error {
	// 检查用户是否存在
	_, err := s.userRepo.FindByUsername(username)
	if err == nil {
		return ErrUserExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &model.User{
		Username: username,
		Password: string(hashedPassword),
		Status:   model.UserStatusEnabled,
	}

	if err := s.userRepo.Create(user); err != nil {
		return err
	}

	ctx := context.Background()
	_ = redis.IncrementAdminUserCreated(ctx, user.Status)
	return redis.SetUserEnabled(ctx, user.ID, true)
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
