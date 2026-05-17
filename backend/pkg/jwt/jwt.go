package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenExpired = errors.New("登录凭证已过期")
	ErrTokenInvalid = errors.New("登录凭证无效")
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type Claims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

type JWT struct {
	Secret []byte
	Expire time.Duration
}

func NewJWT(secret string, expireHours int) *JWT {
	return &JWT{
		Secret: []byte(secret),
		Expire: time.Duration(expireHours) * time.Hour,
	}
}

// GenerateToken 生成 JWT Token
func (j *JWT) GenerateToken(userID uint64, username string) (string, error) {
	return j.generateToken(userID, username, RoleUser)
}

// GenerateAdminToken 生成管理员 JWT Token
func (j *JWT) GenerateAdminToken(adminID uint64, username string) (string, error) {
	return j.generateToken(adminID, username, RoleAdmin)
}

func (j *JWT) generateToken(userID uint64, username, role string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.Expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.Secret)
}

// ParseToken 解析 JWT Token
func (j *JWT) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return j.Secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}
