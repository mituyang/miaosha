package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// JWT 配置 - 需要与 config.yaml 中的 jwt.secret 一致
	JWTSecret  = "W6jP5ADwpuqVCtza1ftmxyS2cll1QDaYAM6aaWEkJyI=" // 请修改为你的 secret
	TotalUsers = 10_000_000                                     // 生成 1000 万个 token
	OutputFile = "../tokens.txt"
)

type Claims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func main() {
	log.Printf("开始本地生成 %d 个 token...", TotalUsers)
	startTime := time.Now()

	file, err := os.Create(OutputFile)
	if err != nil {
		log.Fatalf("创建文件失败: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	expireTime := time.Now().AddDate(100, 0, 0) // 100年后过期

	for i := 0; i < TotalUsers; i++ {
		// user_id 从 1 开始 (与数据库自增ID对应)
		userID := uint64(i + 1)
		username := fmt.Sprintf("test_user_%d", i)

		token, err := generateToken(userID, username, expireTime)
		if err != nil {
			log.Printf("生成 token 失败: userID=%d, err=%v", userID, err)
			continue
		}

		fmt.Fprintf(writer, "%s\n", token)

		if (i+1)%100000 == 0 {
			elapsed := time.Since(startTime)
			speed := float64(i+1) / elapsed.Seconds()
			log.Printf("进度: %d/%d, 速度: %.0f/s", i+1, TotalUsers, speed)
		}
	}

	writer.Flush()

	elapsed := time.Since(startTime)
	log.Printf("完成! 共生成 %d 个 token, 耗时: %v", TotalUsers, elapsed)
	log.Printf("Token 已保存到 %s", OutputFile)
}

func generateToken(userID uint64, username string, expireTime time.Time) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(JWTSecret))
}
