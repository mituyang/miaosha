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
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	CaptchaID   string `json:"captcha_id" binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=4"`
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(400, "请求参数无效"))
		return
	}

	if err := h.authSvc.Register(req.Username, req.Password); err != nil {
		if err == service.ErrUserExists {
			c.JSON(http.StatusConflict, util.Error(409, "用户名已存在"))
			return
		}
		c.JSON(http.StatusInternalServerError, util.Error(500, "注册失败"))
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
