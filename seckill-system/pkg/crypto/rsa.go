package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"sync"
)

var (
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	publicPEM  string
	once       sync.Once
)

// Init 初始化 RSA 密钥对（启动时调用一次）
func Init() error {
	var initErr error
	once.Do(func() {
		// 生成 2048 位 RSA 密钥对
		privateKey, initErr = rsa.GenerateKey(rand.Reader, 2048)
		if initErr != nil {
			initErr = fmt.Errorf("生成 RSA 密钥失败: %w", initErr)
			return
		}
		publicKey = &privateKey.PublicKey

		// 将公钥转换为 PEM 格式，供前端使用
		pubASN1, err := x509.MarshalPKIXPublicKey(publicKey)
		if err != nil {
			initErr = fmt.Errorf("序列化公钥失败: %w", err)
			return
		}
		pubPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubASN1,
		})
		publicPEM = string(pubPEM)
	})
	return initErr
}

// GetPublicKey 获取 PEM 格式的公钥（返回给前端）
func GetPublicKey() string {
	return publicPEM
}

// Decrypt 使用私钥解密前端传来的加密数据
// 输入：Base64 编码的密文
// 输出：解密后的明文
func Decrypt(encryptedBase64 string) (string, error) {
	if privateKey == nil {
		return "", fmt.Errorf("RSA 密钥未初始化")
	}

	// Base64 解码
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedBase64)
	if err != nil {
		return "", fmt.Errorf("Base64 解码失败: %w", err)
	}

	// RSA PKCS1v15 解密
	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, ciphertext)
	if err != nil {
		return "", fmt.Errorf("RSA 解密失败: %w", err)
	}

	return string(plaintext), nil
}
