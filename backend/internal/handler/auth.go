package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"seckill/internal/service"
	"seckill/pkg/util"
)

type AuthHandler struct {
	authSvc *service.AuthService
}

func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

type RegisterRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=50"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6"`
	EmailCode string `json:"email_code" binding:"required"`
}

type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=4"`
}

type SendEmailCodeRequest struct {
	Email       string `json:"email" binding:"required,email"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=4"`
}

type ResetPasswordRequest struct {
	Email     string `json:"email" binding:"required,email"`
	EmailCode string `json:"email_code" binding:"required"`
	Password  string `json:"password" binding:"required,min=6"`
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(400, "请求参数无效"))
		return
	}

	if err := h.authSvc.Register(c.Request.Context(), req.Username, req.Email, req.Password, req.EmailCode); err != nil {
		switch err {
		case service.ErrUserExists:
			c.JSON(http.StatusConflict, util.Error(409, "用户名已存在"))
		case service.ErrEmailExists:
			c.JSON(http.StatusConflict, util.Error(409, "邮箱已被注册"))
		case service.ErrEmailInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱格式无效"))
		case service.ErrEmailCodeInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱验证码错误"))
		case service.ErrEmailCodeExpired:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱验证码已过期，请重新获取"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "注册失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}

// SendEmailCode 发送注册邮箱验证码
func (h *AuthHandler) SendEmailCode(c *gin.Context) {
	var req SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(400, "请求参数无效"))
		return
	}

	if err := h.authSvc.VerifyCaptcha(c.Request.Context(), req.CaptchaID, req.CaptchaCode); err != nil {
		switch err {
		case service.ErrCaptchaInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "验证码错误"))
		case service.ErrCaptchaExpired:
			c.JSON(http.StatusBadRequest, util.Error(400, "验证码已过期，请刷新后重试"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "验证码校验失败"))
		}
		return
	}

	if err := h.authSvc.SendRegistrationEmailCode(c.Request.Context(), req.Email); err != nil {
		switch err {
		case service.ErrEmailExists:
			c.JSON(http.StatusConflict, util.Error(409, "邮箱已被注册"))
		case service.ErrEmailInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱格式无效"))
		case service.ErrEmailSendTooFrequent:
			c.JSON(http.StatusTooManyRequests, util.Error(429, "邮箱验证码发送过于频繁，请稍后再试"))
		case service.ErrEmailConfigInvalid:
			c.JSON(http.StatusInternalServerError, util.Error(500, "邮箱发送配置未完成"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "邮箱验证码发送失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{
		"cooldown_seconds":   h.authSvc.EmailSendIntervalSeconds(),
		"expires_in_seconds": h.authSvc.EmailCodeTTLSeconds(),
	}))
}

// SendPasswordResetEmailCode 发送找回密码邮箱验证码
func (h *AuthHandler) SendPasswordResetEmailCode(c *gin.Context) {
	var req SendEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(400, "请求参数无效"))
		return
	}

	if err := h.authSvc.VerifyCaptcha(c.Request.Context(), req.CaptchaID, req.CaptchaCode); err != nil {
		switch err {
		case service.ErrCaptchaInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "验证码错误"))
		case service.ErrCaptchaExpired:
			c.JSON(http.StatusBadRequest, util.Error(400, "验证码已过期，请刷新后重试"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "验证码校验失败"))
		}
		return
	}

	if err := h.authSvc.SendPasswordResetEmailCode(c.Request.Context(), req.Email); err != nil {
		switch err {
		case service.ErrEmailNotFound:
			c.JSON(http.StatusNotFound, util.Error(404, "邮箱未注册"))
		case service.ErrEmailInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱格式无效"))
		case service.ErrEmailSendTooFrequent:
			c.JSON(http.StatusTooManyRequests, util.Error(429, "邮箱验证码发送过于频繁，请稍后再试"))
		case service.ErrEmailConfigInvalid:
			c.JSON(http.StatusInternalServerError, util.Error(500, "邮箱发送配置未完成"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "邮箱验证码发送失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{
		"cooldown_seconds":   h.authSvc.EmailSendIntervalSeconds(),
		"expires_in_seconds": h.authSvc.EmailCodeTTLSeconds(),
	}))
}

// ResetPassword 重置用户密码
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(400, "请求参数无效"))
		return
	}

	if err := h.authSvc.ResetPassword(c.Request.Context(), req.Email, req.EmailCode, req.Password); err != nil {
		switch err {
		case service.ErrEmailNotFound:
			c.JSON(http.StatusNotFound, util.Error(404, "邮箱未注册"))
		case service.ErrEmailInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱格式无效"))
		case service.ErrPasswordInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "密码至少6位"))
		case service.ErrEmailCodeInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱验证码错误"))
		case service.ErrEmailCodeExpired:
			c.JSON(http.StatusBadRequest, util.Error(400, "邮箱验证码已过期，请重新获取"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "密码重置失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}

// GetCaptcha 获取登录验证码
func (h *AuthHandler) GetCaptcha(c *gin.Context) {
	captcha, err := h.authSvc.GenerateCaptcha(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(500, "获取验证码失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(captcha))
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(400, "请求参数无效"))
		return
	}

	if err := h.authSvc.VerifyCaptcha(c.Request.Context(), req.CaptchaID, req.CaptchaCode); err != nil {
		switch err {
		case service.ErrCaptchaInvalid:
			c.JSON(http.StatusBadRequest, util.Error(400, "验证码错误"))
		case service.ErrCaptchaExpired:
			c.JSON(http.StatusBadRequest, util.Error(400, "验证码已过期，请刷新后重试"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "验证码校验失败"))
		}
		return
	}

	token, err := h.authSvc.Login(req.Username, req.Password)
	if err != nil {
		switch err {
		case service.ErrUserNotFound, service.ErrPasswordWrong:
			c.JSON(http.StatusUnauthorized, util.Error(401, "用户名或密码错误"))
		case service.ErrUserDisabled:
			c.JSON(http.StatusForbidden, util.Error(403, "用户已被禁用"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(500, "登录失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"token": token}))
}
