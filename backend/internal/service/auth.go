package service

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/jwt"
)

var (
	ErrUserExists    = errors.New("username already exists")
	ErrUserNotFound  = errors.New("user not found")
	ErrPasswordWrong = errors.New("password incorrect")
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
	}

	return s.userRepo.Create(user)
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

	// 校验密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", ErrPasswordWrong
	}

	// 生成 Token
	return s.jwt.GenerateToken(user.ID, user.Username)
}
