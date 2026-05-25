package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TotalUsers = 1_000_000 // 生成 100 万个 token
	OutputFile = "../tokens.txt"
)

var jwtSecret string

type Claims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func main() {
	// 兼容两种启动路径：
	// 1) 在 test/tools 目录执行：go run ./gen_tokens -> ../../.env
	// 2) 在仓库根执行：go run ./test/tools/gen_tokens -> .env
	loadDotEnvCandidates("../../.env", "../.env", ".env", "../../../.env")
	jwtSecret = strings.TrimSpace(os.Getenv("JWT_SECRET"))

	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required (load it from .env)")
	}

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

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func loadDotEnvCandidates(paths ...string) {
	for _, p := range paths {
		if loadDotEnvFile(p) {
			return
		}
	}
}

func loadDotEnvFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key == "" {
			continue
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}

	return true
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
	return token.SignedString([]byte(jwtSecret))
}
