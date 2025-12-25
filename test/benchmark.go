package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/time/rate"
)

// 配置
const (
	BaseURL     = "http://localhost:8080"
	AdminSecret = "tdPrNHfDnVCq+cQv8YvyW01dni0KVQ8maB0QracsWN8=" // 需要与 config.yaml 中的 admin.secret 一致
	GoodsID     = 1                                              // 测试商品ID
	Quantity    = 1                                              // 每次购买数量
	TokenFile   = "tokens.txt"
	MySQLDSN    = "root:root123@tcp(127.0.0.1:3306)/seckill?parseTime=true&loc=Local"
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
	TotalRequests    int64
	SuccessRequests  int64
	FailedRequests   int64
	SoldOut          int64
	LimitExceed      int64 // 超过限购
	Canceled         int64 // 压测结束时被取消的请求
	CompletedLatency int64 // 已完成请求的延迟（不含取消的，纳秒）
	FirstSoldOutTime int64 // 第一次售罄的时间戳（纳秒），用于计算抢购阶段耗时
}

// 延迟收集器（用于计算百分位）
type LatencyCollector struct {
	mu        sync.Mutex
	latencies []int64 // 纳秒
}

func (c *LatencyCollector) Add(latency int64) {
	c.mu.Lock()
	c.latencies = append(c.latencies, latency)
	c.mu.Unlock()
}

func (c *LatencyCollector) Percentile(p float64) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.latencies) == 0 {
		return 0
	}

	// 排序
	sorted := make([]int64, len(c.latencies))
	copy(sorted, c.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// 计算百分位索引
	idx := int(float64(len(sorted)-1) * p / 100)
	return float64(sorted[idx]) / 1e6 // 转换为毫秒
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
func doSeckill(ctx context.Context, token string, goodsID int, quantity int) (int, time.Duration, error) {
	body, _ := json.Marshal(map[string]int{"goods_id": goodsID, "quantity": quantity})
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

// 端到端TPS统计结果
type E2EStats struct {
	TotalOrders  int64
	FirstRequest time.Time
	LastWrite    time.Time
	DurationSec  float64
	E2ETPS       float64
}

// 查询端到端TPS（从MySQL统计）
func getE2EStats(goodsID int) (*E2EStats, error) {
	db, err := sql.Open("mysql", MySQLDSN)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var stats E2EStats
	err = db.QueryRow(`
		SELECT 
			COUNT(*) as total_orders,
			MIN(request_time) as first_request,
			MAX(write_time) as last_write
		FROM orders 
		WHERE goods_id = ?
	`, goodsID).Scan(&stats.TotalOrders, &stats.FirstRequest, &stats.LastWrite)
	if err != nil {
		return nil, err
	}

	if stats.TotalOrders > 0 {
		stats.DurationSec = stats.LastWrite.Sub(stats.FirstRequest).Seconds()
		if stats.DurationSec > 0 {
			stats.E2ETPS = float64(stats.TotalOrders) / stats.DurationSec
		}
	}
	return &stats, nil
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
	concurrency := 300   // 并发数
	targetQPS := 10000   // 目标QPS（每秒请求数），0表示不限制
	maxUsers := 10000000 // 最多使用的用户数
	duration := 30       // 测试持续时间(秒)

	log("=== 秒杀压测配置 ===")
	if targetQPS > 0 {
		log("并发数: %d, 目标QPS: %d, 最大用户数: %d, 持续时间: %ds", concurrency, targetQPS, maxUsers, duration)
	} else {
		log("并发数: %d, 目标QPS: 不限制, 最大用户数: %d, 持续时间: %ds", concurrency, maxUsers, duration)
	}

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
	var latencyCollector LatencyCollector
	var wg sync.WaitGroup
	var readyWg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	startCh := make(chan struct{}) // 同时开始信号
	userIndex := int64(0)

	totalTokens := int64(len(tokens))

	// 创建限流器（如果设置了目标QPS）
	var limiter *rate.Limiter
	if targetQPS > 0 {
		limiter = rate.NewLimiter(rate.Limit(targetQPS), targetQPS) // 令牌桶：每秒targetQPS个令牌
	}

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
					// 限流等待
					if limiter != nil {
						if err := limiter.Wait(ctx); err != nil {
							return // context canceled
						}
					}

					// 轮询获取用户
					idx := atomic.AddInt64(&userIndex, 1) % totalTokens
					token := tokens[idx]

					code, latency, err := doSeckill(ctx, token, GoodsID, Quantity)
					atomic.AddInt64(&stats.TotalRequests, 1)

					if err != nil {
						// 区分是主动取消还是真正的网络错误
						if errors.Is(err, context.Canceled) {
							atomic.AddInt64(&stats.Canceled, 1)
							// 被取消的请求不计入延迟统计
						} else {
							atomic.AddInt64(&stats.FailedRequests, 1)
							atomic.AddInt64(&stats.CompletedLatency, int64(latency))
							latencyCollector.Add(int64(latency))
							log("请求失败(网络错误): %v", err)
						}
						continue
					}

					// 只有完成的请求才计入延迟
					atomic.AddInt64(&stats.CompletedLatency, int64(latency))
					latencyCollector.Add(int64(latency))

					switch code {
					case 0: // 成功
						atomic.AddInt64(&stats.SuccessRequests, 1)
					case 1001: // 已售罄
						// 记录第一次售罄时间
						if atomic.LoadInt64(&stats.FirstSoldOutTime) == 0 {
							atomic.CompareAndSwapInt64(&stats.FirstSoldOutTime, 0, time.Now().UnixNano())
						}
						atomic.AddInt64(&stats.SoldOut, 1)
					case 1002: // 超过限购
						atomic.AddInt64(&stats.LimitExceed, 1)
					default:
						atomic.AddInt64(&stats.FailedRequests, 1)
						log("请求失败(业务错误): code=%d", code)
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
	completedRequests := stats.TotalRequests - stats.Canceled
	avgLatency := float64(stats.CompletedLatency) / float64(completedRequests) / 1e6 // ms

	log("")
	log("=== 压测结果 ===")
	log("实际耗时:     %.2f s", actualDuration)
	log("总请求数:     %d", stats.TotalRequests)
	log("完成请求:     %d", completedRequests)
	log("成功请求:     %d", stats.SuccessRequests)
	log("已售罄:       %d", stats.SoldOut)
	log("超过限购:     %d", stats.LimitExceed)
	log("失败请求:     %d", stats.FailedRequests)
	log("被取消请求:   %d (压测结束时进行中的请求，不计入统计)", stats.Canceled)
	log("剩余库存:     %d", finalStock)
	log("平均延迟:     %.2f ms", avgLatency)
	log("P50延迟:      %.2f ms", latencyCollector.Percentile(50))
	log("P95延迟:      %.2f ms", latencyCollector.Percentile(95))
	log("P99延迟:      %.2f ms", latencyCollector.Percentile(99))
	log("QPS:          %.2f (API响应速度)", float64(completedRequests)/actualDuration)

	// 计算抢购阶段QPS
	if stats.FirstSoldOutTime > 0 && stats.SuccessRequests > 0 {
		seckillDuration := float64(stats.FirstSoldOutTime-startTime.UnixNano()) / 1e9 // 秒
		log("抢购阶段耗时: %.2f s", seckillDuration)
		log("抢购阶段QPS:  %.2f (成功请求/抢购耗时)", float64(stats.SuccessRequests)/seckillDuration)
	} else if stats.SuccessRequests > 0 {
		// 没有售罄，说明库存没卖完，用总时间计算
		log("抢购阶段QPS:  %.2f (库存未售罄，基于总时间)", float64(stats.SuccessRequests)/actualDuration)
	}

	// 等待 Worker 处理完成，查询端到端 TPS
	if stats.SuccessRequests > 0 {
		log("")
		log("等待订单写入MySQL完成...")
		time.Sleep(10 * time.Second) // 等待 Worker 处理

		e2eStats, err := getE2EStats(GoodsID)
		if err != nil {
			log("查询端到端TPS失败: %v", err)
		} else {
			log("")
			log("=== 端到端TPS (订单落库) ===")
			log("MySQL订单数:  %d", e2eStats.TotalOrders)
			log("首个请求时间: %s", e2eStats.FirstRequest.Format("15:04:05.000"))
			log("最后写入时间: %s", e2eStats.LastWrite.Format("15:04:05.000"))
			log("端到端耗时:   %.2f s", e2eStats.DurationSec)
			log("端到端TPS:    %.2f (订单从请求到落库)", e2eStats.E2ETPS)
		}
	}
}
