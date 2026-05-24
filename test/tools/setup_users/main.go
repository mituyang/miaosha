package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

// 配置 - 请根据实际情况修改
const (
	DSN        = "root:root123@tcp(localhost:13306)/seckill?charset=utf8mb4&parseTime=True"
	TotalUsers = 10_000_000 // 1000万用户
	BatchSize  = 20000      // 每批插入数量 (MySQL placeholder 限制 65535, 3字段×20000=60000)
)

func main() {
	log.Println("连接数据库...")
	db, err := sql.Open("mysql", DSN)
	if err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		log.Fatalf("Ping 失败: %v", err)
	}
	log.Println("数据库连接成功")

	// 预先生成密码哈希 (所有测试用户使用相同密码)
	password := "test123456"
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("密码哈希失败: %v", err)
	}
	pwdStr := string(hashedPwd)

	log.Printf("开始插入 %d 个用户，每批 %d 条...", TotalUsers, BatchSize)
	startTime := time.Now()

	inserted := 0
	for i := 0; i < TotalUsers; i += BatchSize {
		batchEnd := i + BatchSize
		if batchEnd > TotalUsers {
			batchEnd = TotalUsers
		}
		batchCount := batchEnd - i

		if err := insertBatch(db, i, batchCount, pwdStr); err != nil {
			log.Printf("批次 %d-%d 插入失败: %v", i, batchEnd, err)
			continue
		}

		inserted += batchCount
		if inserted%100000 == 0 {
			elapsed := time.Since(startTime)
			speed := float64(inserted) / elapsed.Seconds()
			remaining := float64(TotalUsers-inserted) / speed
			log.Printf("进度: %d/%d (%.1f%%), 速度: %.0f/s, 预计剩余: %.0fs",
				inserted, TotalUsers,
				float64(inserted)/float64(TotalUsers)*100,
				speed, remaining)
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("完成! 共插入 %d 用户, 耗时 %v, 平均 %.0f 条/秒",
		inserted, elapsed, float64(inserted)/elapsed.Seconds())
}

func insertBatch(db *sql.DB, startIdx, count int, hashedPwd string) error {
	valueStrings := make([]string, 0, count)
	valueArgs := make([]interface{}, 0, count*3)

	for j := 0; j < count; j++ {
		valueStrings = append(valueStrings, "(?, ?, ?)")
		userIndex := startIdx + j + 1
		username := fmt.Sprintf("test_user_%d", userIndex)
		email := fmt.Sprintf("test%d@yyqw.shop", userIndex)
		valueArgs = append(valueArgs, username, email, hashedPwd)
	}

	stmt := fmt.Sprintf(
		"INSERT INTO users (username, email, password) VALUES %s ON DUPLICATE KEY UPDATE email = VALUES(email)",
		strings.Join(valueStrings, ","),
	)

	_, err := db.Exec(stmt, valueArgs...)
	return err
}
