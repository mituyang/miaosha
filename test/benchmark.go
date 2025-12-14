package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// 配置
const (
	BaseURL     = "http://localhost:8080"
	AdminSecret = "tdPrNHfDnVCq+cQv8YvyW01dni0KVQ8maB0QracsWN8=" // 需要与 config.yaml 中的 admin.secret 一致
	GoodsID     = 1                                              // 测试商品ID
	TokenFile   = "tokens.txt"
)

// HTTP Transport（压测时动态创建，便于强制关闭）
var httpTransport *http.Transport
var httpClient *http.Client

func initHTTPClient() {
	httpTransport = &http.Transport{
		MaxIdleConns:        10000,
		MaxIdleConnsPerHost: 10000,
		MaxConnsPerHost:     10000,
		IdleConnTimeout:     30 * time.Second,
	}
	httpClient = &http.Client{
		Transport: httpTransport,
		Timeout:   30 * time.Second,
	}
}

func forceCloseHTTPClient() {
	if httpTransport != nil {
		httpTransport.CloseIdleConnections()
	}
}

// 响应结构
type Response struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// 测试结果统计
type Stats struct {
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	SoldOut         int64
	RepeatBuy       int64
	TotalLatency    int64 // 纳秒
}

// ==================== 辅助函数 ====================

// 预热库存
func warmUp() error {
	req, _ := http.NewRequest("POST", BaseURL+"/api/admin/warmup", nil)
	req.Header.Set("X-Admin-Secret", AdminSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("warmup failed: %s", result.Msg)
	}
	return nil
}

// 执行秒杀请求
func doSeckill(ctx context.Context, token string, goodsID int) (int, time.Duration, error) {
	body, _ := json.Marshal(map[string]int{"goods_id": goodsID})
	req, _ := http.NewRequestWithContext(ctx, "POST", BaseURL+"/api/seckill/buy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start)

	if err != nil {
		return -1, latency, err
	}
	defer resp.Body.Close()

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, latency, err
	}
	return result.Code, latency, nil
}

// 查询库存
func getStock(goodsID int) (int, error) {
	resp, err := httpClient.Get(fmt.Sprintf("%s/api/seckill/stock/%d", BaseURL, goodsID))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Code int `json:"code"`
		Data struct {
			Stock int `json:"stock"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return 0, err
	}
	return result.Data.Stock, nil
}

// ==================== 压测函数 ====================

// loadTokens 从文件加载 token
func loadTokens(filename string, limit int) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tokens []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() && (limit <= 0 || len(tokens) < limit) {
		token := scanner.Text()
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens, scanner.Err()
}

// log 带毫秒时间戳的日志
func log(format string, args ...any) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Printf("[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
}

func main() {
	// 初始化 HTTP 客户端
	initHTTPClient()

	// 配置参数
	concurrency := 10000 // 并发数
	maxUsers := 10000000 // 最多使用的用户数
	duration := 40       // 测试持续时间(秒)

	log("=== 秒杀压测配置 ===")
	log("并发数: %d, 最大用户数: %d, 持续时间: %ds", concurrency, maxUsers, duration)

	// 1. 从文件加载 token
	log("从 %s 加载 token...", TokenFile)
	tokens, err := loadTokens(TokenFile, maxUsers)
	if err != nil {
		log("加载 token 失败: %v", err)
		os.Exit(1)
	}
	log("加载完成，共 %d 个 token", len(tokens))

	// 2. 预热库存
	log("预热库存...")
	if err := warmUp(); err != nil {
		log("预热失败: %v", err)
		os.Exit(1)
	}

	stock, _ := getStock(GoodsID)
	log("当前库存: %d", stock)

	// 3. 开始压测
	log("创建 goroutine...")
	var stats Stats
	var wg sync.WaitGroup
	var readyWg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	startCh := make(chan struct{}) // 同时开始信号
	userIndex := int64(0)

	totalTokens := int64(len(tokens))

	// 启动并发 worker
	for range concurrency {
		wg.Add(1)
		readyWg.Add(1)
		go func() {
			defer wg.Done()
			readyWg.Done() // 标记就绪
			<-startCh      // 等待开始信号
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// 轮询获取用户
					idx := atomic.AddInt64(&userIndex, 1) % totalTokens
					token := tokens[idx]

					code, latency, err := doSeckill(ctx, token, GoodsID)
					atomic.AddInt64(&stats.TotalRequests, 1)
					atomic.AddInt64(&stats.TotalLatency, int64(latency))

					if err != nil {
						atomic.AddInt64(&stats.FailedRequests, 1)
						continue
					}

					switch code {
					case 0: // 成功
						atomic.AddInt64(&stats.SuccessRequests, 1)
					case 1001: // 已售罄
						atomic.AddInt64(&stats.SoldOut, 1)
					case 1002: // 重复购买
						atomic.AddInt64(&stats.RepeatBuy, 1)
					default:
						atomic.AddInt64(&stats.FailedRequests, 1)
					}
				}
			}
		}()
	}

	// 等待所有 goroutine 就绪
	readyWg.Wait()
	log("开始压测...")
	startTime := time.Now()
	close(startCh) // 同时开始

	// 等待测试时间后立即停止
	time.Sleep(time.Duration(duration) * time.Second)
	actualDuration := time.Since(startTime).Seconds()

	// 强制取消所有进行中的请求
	cancel()
	wg.Wait() // 等待所有 goroutine 退出
	forceCloseHTTPClient()

	// 4. 输出结果
	finalStock, _ := getStock(GoodsID)
	avgLatency := float64(stats.TotalLatency) / float64(stats.TotalRequests) / 1e6 // ms

	log("")
	log("=== 压测结果 ===")
	log("实际耗时:     %.2f s", actualDuration)
	log("总请求数:     %d", stats.TotalRequests)
	log("成功请求:     %d", stats.SuccessRequests)
	log("已售罄:       %d", stats.SoldOut)
	log("重复购买:     %d", stats.RepeatBuy)
	log("失败请求:     %d", stats.FailedRequests)
	log("剩余库存:     %d", finalStock)
	log("平均延迟:     %.2f ms", avgLatency)
	log("TPS:          %.2f", float64(stats.TotalRequests)/actualDuration)
	log("成功TPS:      %.2f", float64(stats.SuccessRequests)/actualDuration)
}
