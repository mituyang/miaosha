package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"seckill/internal/service"
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
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	if err := h.authSvc.Register(req.Username, req.Password); err != nil {
		if err == service.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "username already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "register failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success"})
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	token, err := h.authSvc.Login(req.Username, req.Password)
	if err != nil {
		switch err {
		case service.ErrUserNotFound, service.ErrPasswordWrong:
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "invalid username or password"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "login failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "success", "data": gin.H{"token": token}})
}
