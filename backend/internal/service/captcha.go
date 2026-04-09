package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"strings"

	redisPkg "seckill/pkg/redis"
)

var (
	ErrCaptchaInvalid = errors.New("captcha invalid")
	ErrCaptchaExpired = errors.New("captcha expired")
)

type CaptchaPayload struct {
	ID    string `json:"captcha_id"`
	Image string `json:"captcha_image"`
}

func (s *AuthService) GenerateCaptcha(ctx context.Context) (*CaptchaPayload, error) {
	captchaID, err := randomDigits(8)
	if err != nil {
		return nil, err
	}

	code, err := randomDigits(4)
	if err != nil {
		return nil, err
	}

	if err := redisPkg.SetCaptcha(ctx, captchaID, code); err != nil {
		return nil, err
	}

	return &CaptchaPayload{
		ID:    captchaID,
		Image: svgToDataURI(buildCaptchaSVG(code)),
	}, nil
}

func (s *AuthService) VerifyCaptcha(ctx context.Context, captchaID, captchaCode string) error {
	captchaID = strings.TrimSpace(captchaID)
	captchaCode = strings.TrimSpace(captchaCode)
	if captchaID == "" || captchaCode == "" {
		return ErrCaptchaInvalid
	}

	if err := redisPkg.VerifyAndDeleteCaptcha(ctx, captchaID, captchaCode); err != nil {
		if strings.Contains(err.Error(), "redis: nil") {
			return ErrCaptchaExpired
		}
		if strings.Contains(err.Error(), "captcha mismatch") {
			return ErrCaptchaInvalid
		}
		return err
	}

	return nil
}

func randomDigits(length int) (string, error) {
	var builder strings.Builder
	builder.Grow(length)
	limit := big.NewInt(10)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		builder.WriteByte(byte('0' + n.Int64()))
	}
	return builder.String(), nil
}

func buildCaptchaSVG(code string) string {
	escaped := html.EscapeString(code)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="132" height="44" viewBox="0 0 132 44">
<rect width="132" height="44" rx="8" fill="#f4efe6"/>
<path d="M8 12 C24 24, 36 4, 52 18 S84 30, 100 14 S116 8, 124 20" stroke="#d0b38a" stroke-width="2" fill="none"/>
<path d="M10 32 C26 20, 40 38, 56 24 S86 10, 102 28 S118 36, 124 24" stroke="#b88952" stroke-width="2" fill="none" opacity="0.7"/>
<text x="66" y="29" text-anchor="middle" font-size="24" font-family="Arial, sans-serif" font-weight="700" letter-spacing="6" fill="#5a3d1f">%s</text>
</svg>`, escaped)
}

func svgToDataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
