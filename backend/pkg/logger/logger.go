package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	Info  *log.Logger
	Warn  *log.Logger
	Error *log.Logger
)

// Init 初始化日志，按时间戳创建日志文件
// appName: api 或 worker
func Init(appName string) (*os.File, error) {
	// 创建 logs 目录
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir failed: %w", err)
	}

	// 按时间戳命名日志文件
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFile := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", appName, timestamp))

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file failed: %w", err)
	}

	// 同时输出到控制台和文件
	multiWriter := io.MultiWriter(os.Stdout, file)

	// 设置日志格式
	flags := log.Ldate | log.Ltime | log.Lshortfile

	Info = log.New(multiWriter, "[INFO] ", flags)
	Warn = log.New(multiWriter, "[WARN] ", flags)
	Error = log.New(multiWriter, "[ERROR] ", flags)

	// 替换标准库默认 logger
	log.SetOutput(multiWriter)
	log.SetFlags(flags)

	Info.Printf("Log file: %s", logFile)
	return file, nil
}
