package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/smtp"
	"time"

	"github.com/go-redis/redis/v8"
)

// Config 邮件配置
type Config struct {
	Host     string // SMTP 服务器地址
	Port     int    // SMTP 端口
	Username string // 发件人邮箱
	Password string // 邮箱密码或授权码
	From     string // 发件人显示名称
}

var (
	cfg         *Config
	redisClient *redis.Client
)

// Init 初始化邮件模块
func Init(config *Config, rdb *redis.Client) {
	cfg = config
	redisClient = rdb
}

// GenerateCode 生成 6 位随机验证码
func GenerateCode() string {
	code := ""
	for i := 0; i < 6; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += fmt.Sprintf("%d", n.Int64())
	}
	return code
}

// SendVerifyCode 发送验证码邮件
func SendVerifyCode(ctx context.Context, toEmail string) (string, error) {
	code := GenerateCode()

	// 存储验证码到 Redis，5 分钟有效
	key := fmt.Sprintf("verify_code:%s", toEmail)
	if err := redisClient.Set(ctx, key, code, 5*time.Minute).Err(); err != nil {
		return "", fmt.Errorf("存储验证码失败: %w", err)
	}

	// 发送邮件
	subject := "【秒杀系统】注册验证码"
	body := fmt.Sprintf(`
		<h2>秒杀系统 - 邮箱验证</h2>
		<p>您的验证码是：<strong style="color: #e74c3c; font-size: 24px;">%s</strong></p>
		<p>验证码 5 分钟内有效，请尽快完成注册。</p>
		<p>如果这不是您的操作，请忽略此邮件。</p>
	`, code)

	if err := sendMailSSL(toEmail, subject, body); err != nil {
		return "", fmt.Errorf("发送邮件失败: %w", err)
	}

	return code, nil
}

// VerifyCode 验证注册验证码
func VerifyCode(ctx context.Context, email, code string) bool {
	key := fmt.Sprintf("verify_code:%s", email)
	storedCode, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return false
	}

	if storedCode == code {
		// 验证成功后删除验证码
		redisClient.Del(ctx, key)
		return true
	}
	return false
}

// SendLoginCode 发送登录验证码邮件
func SendLoginCode(ctx context.Context, toEmail string) (string, error) {
	code := GenerateCode()

	// 存储登录验证码到 Redis，5 分钟有效（使用不同的 key 前缀）
	key := fmt.Sprintf("login_code:%s", toEmail)
	if err := redisClient.Set(ctx, key, code, 5*time.Minute).Err(); err != nil {
		return "", fmt.Errorf("存储验证码失败: %w", err)
	}

	// 发送邮件
	subject := "【秒杀系统】登录验证码"
	body := fmt.Sprintf(`
		<h2>秒杀系统 - 登录验证</h2>
		<p>您的登录验证码是：<strong style="color: #3498db; font-size: 24px;">%s</strong></p>
		<p>验证码 5 分钟内有效。</p>
		<p>如果这不是您的操作，请注意账号安全。</p>
	`, code)

	if err := sendMailSSL(toEmail, subject, body); err != nil {
		return "", fmt.Errorf("发送邮件失败: %w", err)
	}

	return code, nil
}

// VerifyLoginCode 验证登录验证码
func VerifyLoginCode(ctx context.Context, email, code string) bool {
	key := fmt.Sprintf("login_code:%s", email)
	storedCode, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return false
	}

	if storedCode == code {
		// 验证成功后删除验证码
		redisClient.Del(ctx, key)
		return true
	}
	return false
}

// SendResetCode 发送重置密码验证码邮件
func SendResetCode(ctx context.Context, toEmail string) (string, error) {
	code := GenerateCode()

	// 存储重置密码验证码到 Redis，5 分钟有效
	key := fmt.Sprintf("reset_code:%s", toEmail)
	if err := redisClient.Set(ctx, key, code, 5*time.Minute).Err(); err != nil {
		return "", fmt.Errorf("存储验证码失败: %w", err)
	}

	// 发送邮件
	subject := "【秒杀系统】重置密码验证码"
	body := fmt.Sprintf(`
		<h2>秒杀系统 - 重置密码</h2>
		<p>您的验证码是：<strong style="color: #e67e22; font-size: 24px;">%s</strong></p>
		<p>验证码 5 分钟内有效。</p>
		<p>如果这不是您的操作，请忽略此邮件并确保账号安全。</p>
	`, code)

	if err := sendMailSSL(toEmail, subject, body); err != nil {
		return "", fmt.Errorf("发送邮件失败: %w", err)
	}

	return code, nil
}

// VerifyResetCode 验证重置密码验证码
func VerifyResetCode(ctx context.Context, email, code string) bool {
	key := fmt.Sprintf("reset_code:%s", email)
	storedCode, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return false
	}

	if storedCode == code {
		// 验证成功后删除验证码
		redisClient.Del(ctx, key)
		return true
	}
	return false
}

// sendMailSSL 使用 SSL/TLS 发送邮件（适用于 465 端口）
func sendMailSSL(to, subject, body string) error {
	if cfg == nil {
		return fmt.Errorf("邮件模块未初始化")
	}

	log.Printf("[邮件] 开始发送，配置: Host=%s, Port=%d, Username=%s, To=%s",
		cfg.Host, cfg.Port, cfg.Username, to)

	// 邮件内容 - 使用 CRLF 换行
	msg := "From: " + cfg.From + " <" + cfg.Username + ">\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" + body

	// TLS 配置
	tlsConfig := &tls.Config{
		ServerName:         cfg.Host,
		InsecureSkipVerify: false,
	}

	// 连接 SMTP 服务器
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("[邮件] 正在连接 %s ...", addr)

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("连接 SMTP 服务器失败: %w", err)
	}
	defer conn.Close()
	log.Printf("[邮件] TLS 连接成功")

	// 设置超时
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// 创建 SMTP 客户端
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("创建 SMTP 客户端失败: %w", err)
	}
	defer client.Close()
	log.Printf("[邮件] SMTP 客户端创建成功")

	// 发送 EHLO
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("EHLO 失败: %w", err)
	}
	log.Printf("[邮件] EHLO 成功")

	// 认证 - 使用 LOGIN 方式
	log.Printf("[邮件] 开始认证...")
	if err := client.Auth(&loginAuth{cfg.Username, cfg.Password}); err != nil {
		return fmt.Errorf("SMTP 认证失败: %w", err)
	}
	log.Printf("[邮件] 认证成功")

	// 设置发件人
	if err := client.Mail(cfg.Username); err != nil {
		return fmt.Errorf("设置发件人失败: %w", err)
	}
	log.Printf("[邮件] 发件人设置成功")

	// 设置收件人
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("设置收件人失败: %w", err)
	}
	log.Printf("[邮件] 收件人设置成功")

	// 发送邮件内容
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("获取写入器失败: %w", err)
	}

	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("写入邮件内容失败: %w", err)
	}

	if err = w.Close(); err != nil {
		return fmt.Errorf("关闭写入器失败: %w", err)
	}
	log.Printf("[邮件] 邮件内容发送成功")

	// 优雅退出
	if err := client.Quit(); err != nil {
		// Quit 错误不影响邮件发送，只记录日志
		log.Printf("[邮件] Quit 警告: %v", err)
	}

	log.Printf("[邮件] 发送完成!")
	return nil
}

// loginAuth 实现 LOGIN 认证方式（QQ 邮箱需要）
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// QQ 邮箱返回的是 base64 编码的提示
		return []byte(a.password), nil
	}
	return nil, nil
}

// dialTimeout 带超时的连接
func dialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}
