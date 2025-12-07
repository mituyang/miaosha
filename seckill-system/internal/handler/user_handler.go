package handler

import (
	"log"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"seckill-system/internal/model"
	"seckill-system/pkg/auth"
	"seckill-system/pkg/crypto"
	"seckill-system/pkg/email"
)

// UserHandler 用户相关处理器
type UserHandler struct {
	db *gorm.DB
}

// NewUserHandler 创建用户处理器
func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required"` // RSA 加密后的密码
	Email    string `json:"email" binding:"required,email"`
	Code     string `json:"code" binding:"required,len=6"`
	Nickname string `json:"nickname" binding:"required,min=1,max=50"` // 昵称必填
}

// SendCodeRequest 发送验证码请求
type SendCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// LoginRequest 登录请求（邮箱 + 密码）
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"` // RSA 加密后的密码
}

// LoginByCodeRequest 验证码登录请求（邮箱 + 验证码）
type LoginByCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6"`
}

// SendLoginCodeRequest 发送登录验证码请求
type SendLoginCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// SendResetCodeRequest 发送重置密码验证码请求
type SendResetCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Code     string `json:"code" binding:"required,len=6"`
	Password string `json:"password" binding:"required"` // RSA 加密后的新密码
}

// SendVerifyCode 发送邮箱验证码
func (h *UserHandler) SendVerifyCode(c *gin.Context) {
	var req SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "请输入有效的邮箱地址",
		})
		return
	}

	// 验证邮箱格式
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "邮箱格式不正确",
		})
		return
	}

	// 检查邮箱是否已注册
	var existUser model.User
	if err := h.db.Where("email = ?", req.Email).First(&existUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "该邮箱已被注册",
		})
		return
	}

	// 发送验证码
	ctx := c.Request.Context()
	_, err := email.SendVerifyCode(ctx, req.Email)
	if err != nil {
		log.Printf("发送验证码失败: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "发送验证码失败，请稍后重试",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "验证码已发送，请查收邮件",
	})
}

// Register 用户注册
func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	// 解密密码（前端使用 RSA 公钥加密）
	password, err := crypto.Decrypt(req.Password)
	if err != nil {
		log.Printf("密码解密失败: %v", err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码解密失败",
		})
		return
	}

	// 验证密码格式（至少6位，包含字母和数字）
	if len(password) < 6 || len(password) > 50 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码长度需要在 6-50 位之间",
		})
		return
	}
	hasLetter := regexp.MustCompile(`[a-zA-Z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasLetter || !hasNumber {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码需要同时包含字母和数字",
		})
		return
	}

	// 验证邮箱验证码
	ctx := c.Request.Context()
	if !email.VerifyCode(ctx, req.Email, req.Code) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "验证码错误或已过期",
		})
		return
	}

	// 检查用户名是否已存在
	var existUser model.User
	if err := h.db.Where("username = ?", req.Username).First(&existUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "用户名已存在",
		})
		return
	}

	// 检查邮箱是否已注册
	if err := h.db.Where("email = ?", req.Email).First(&existUser).Error; err == nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "该邮箱已被注册",
		})
		return
	}

	// 密码加密存储（使用 bcrypt，cost=4 用于测试环境，生产环境建议 10+）
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 4)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "系统错误",
		})
		return
	}

	// 创建用户（昵称已是必填，无需默认值）
	user := &model.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Email:    req.Email,
		Nickname: req.Nickname,
	}

	if err := h.db.Create(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "注册失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "注册成功",
		Data: gin.H{
			"user_id":  user.ID,
			"username": user.Username,
		},
	})
}

// Login 邮箱密码登录
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "请输入有效的邮箱和密码",
		})
		return
	}

	// 解密密码（前端使用 RSA 公钥加密）
	password, err := crypto.Decrypt(req.Password)
	if err != nil {
		log.Printf("密码解密失败: %v", err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码解密失败",
		})
		return
	}

	// 通过邮箱查找用户
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "邮箱或密码错误",
		})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "邮箱或密码错误",
		})
		return
	}

	// 生成 Token
	token, err := auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "生成 Token 失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "登录成功",
		Data: gin.H{
			"token":    token,
			"user_id":  user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
		},
	})
}

// SendLoginCode 发送登录验证码（已注册用户）
func (h *UserHandler) SendLoginCode(c *gin.Context) {
	var req SendLoginCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "请输入有效的邮箱地址",
		})
		return
	}

	// 检查邮箱是否已注册
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "该邮箱未注册",
		})
		return
	}

	// 发送登录验证码
	ctx := c.Request.Context()
	_, err := email.SendLoginCode(ctx, req.Email)
	if err != nil {
		log.Printf("发送登录验证码失败: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "发送验证码失败，请稍后重试",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "验证码已发送，请查收邮件",
	})
}

// LoginByCode 验证码登录
func (h *UserHandler) LoginByCode(c *gin.Context) {
	var req LoginByCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "请输入有效的邮箱和验证码",
		})
		return
	}

	// 验证验证码
	ctx := c.Request.Context()
	if !email.VerifyLoginCode(ctx, req.Email, req.Code) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "验证码错误或已过期",
		})
		return
	}

	// 通过邮箱查找用户
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "用户不存在",
		})
		return
	}

	// 生成 Token
	token, err := auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "生成 Token 失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "登录成功",
		Data: gin.H{
			"token":    token,
			"user_id":  user.ID,
			"username": user.Username,
			"nickname": user.Nickname,
		},
	})
}

// GetUserInfo 获取当前用户信息
func (h *UserHandler) GetUserInfo(c *gin.Context) {
	userID := c.GetInt64("user_id")
	username := c.GetString("username")

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"user_id":  userID,
			"username": username,
		},
	})
}

// GetPublicKey 获取 RSA 公钥（供前端加密密码）
func (h *UserHandler) GetPublicKey(c *gin.Context) {
	publicKey := crypto.GetPublicKey()
	if publicKey == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "获取公钥失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"public_key": publicKey,
		},
	})
}

// SendResetCode 发送重置密码验证码
func (h *UserHandler) SendResetCode(c *gin.Context) {
	var req SendResetCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "请输入有效的邮箱地址",
		})
		return
	}

	// 检查邮箱是否已注册
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "该邮箱未注册",
		})
		return
	}

	// 发送重置密码验证码
	ctx := c.Request.Context()
	_, err := email.SendResetCode(ctx, req.Email)
	if err != nil {
		log.Printf("发送重置密码验证码失败: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "发送验证码失败，请稍后重试",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "验证码已发送，请查收邮件",
	})
}

// ResetPassword 重置密码
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数错误",
		})
		return
	}

	// 验证验证码
	ctx := c.Request.Context()
	if !email.VerifyResetCode(ctx, req.Email, req.Code) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "验证码错误或已过期",
		})
		return
	}

	// 解密新密码
	password, err := crypto.Decrypt(req.Password)
	if err != nil {
		log.Printf("密码解密失败: %v", err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码解密失败",
		})
		return
	}

	// 验证密码格式
	if len(password) < 6 || len(password) > 50 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码长度需要在 6-50 位之间",
		})
		return
	}
	hasLetter := regexp.MustCompile(`[a-zA-Z]`).MatchString(password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasLetter || !hasNumber {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "密码需要同时包含字母和数字",
		})
		return
	}

	// 查找用户
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "用户不存在",
		})
		return
	}

	// 加密新密码（cost=4 用于测试环境）
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 4)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "系统错误",
		})
		return
	}

	// 更新密码
	if err := h.db.Model(&user).Update("password", string(hashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "重置密码失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "密码重置成功，请使用新密码登录",
	})
}
