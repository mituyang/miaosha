package service

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"seckill/internal/config"
)

type mailer struct {
	cfg config.EmailConfig
}

func newMailer(cfg config.EmailConfig) *mailer {
	return &mailer{cfg: cfg}
}

func (m *mailer) sendVerificationCode(to, code string, ttl time.Duration, scene string) error {
	if err := m.validate(); err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	message := m.buildMessage(to, code, ttl, scene)

	switch strings.ToLower(strings.TrimSpace(m.cfg.Encryption)) {
	case "tls":
		return m.sendTLS(addr, auth, to, message)
	case "starttls":
		return m.sendStartTLS(addr, auth, to, message)
	case "none":
		return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, message)
	default:
		return ErrEmailConfigInvalid
	}
}

func (m *mailer) validate() error {
	if strings.TrimSpace(m.cfg.Host) == "" ||
		m.cfg.Port <= 0 ||
		strings.TrimSpace(m.cfg.Username) == "" ||
		strings.TrimSpace(m.cfg.Password) == "" ||
		strings.TrimSpace(m.cfg.From) == "" ||
		strings.TrimSpace(m.cfg.Subject) == "" {
		return ErrEmailConfigInvalid
	}
	return nil
}

func (m *mailer) buildMessage(to, code string, ttl time.Duration, scene string) []byte {
	from := m.cfg.From
	if name := strings.TrimSpace(m.cfg.FromName); name != "" {
		from = fmt.Sprintf("%s <%s>", encodeRFC2047(name), m.cfg.From)
	}

	body := fmt.Sprintf("您的%s验证码是：%s\n\n该验证码%s后过期，仅用于秒杀系统账号操作，请勿泄露给他人。", scene, code, formatDurationForMessage(ttl))
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + encodeRFC2047(m.cfg.Subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

func (m *mailer) sendStartTLS(addr string, auth smtp.Auth, to string, message []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return ErrEmailConfigInvalid
	}
	if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (m *mailer) sendTLS(addr string, auth smtp.Auth, to string, message []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		_ = conn.Close()
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func encodeRFC2047(value string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func formatDurationForMessage(duration time.Duration) string {
	if duration >= time.Hour && duration%time.Hour == 0 {
		return fmt.Sprintf("%d小时", int(duration/time.Hour))
	}
	if duration >= time.Minute && duration%time.Minute == 0 {
		return fmt.Sprintf("%d分钟", int(duration/time.Minute))
	}
	if duration >= time.Second && duration%time.Second == 0 {
		return fmt.Sprintf("%d秒", int(duration/time.Second))
	}
	return duration.String()
}
