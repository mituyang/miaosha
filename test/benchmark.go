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
	"slices"
	"sort"
	"strings"
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
	Canceled         int64 // 压测结束时未收到响应的请求（服务器可能已处理）
	CompletedLatency int64 // 已完成请求的延迟（不含取消的，纳秒）
	LastSuccessTime  int64 // 最后一次成功的时间戳（纳秒），用于计算抢购阶段耗时
	FirstSoldOutTime int64 // 第一次售罄的时间戳（纳秒），用于参考
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
	slices.Sort(sorted)

	// 计算百分位索引
	idx := int(float64(len(sorted)-1) * p / 100)
	return float64(sorted[idx]) / 1e6 // 转换为毫秒
}

// 时序数据点
type TimeSeriesPoint struct {
	Timestamp   time.Time
	QPS         float64
	SuccessQPS  float64
	AvgLatency  float64
	P95Latency  float64
	P99Latency  float64
	SuccessRate float64
}

// 时序数据收集器
type TimeSeriesCollector struct {
	mu     sync.Mutex
	points []TimeSeriesPoint
}

func (c *TimeSeriesCollector) Add(point TimeSeriesPoint) {
	c.mu.Lock()
	c.points = append(c.points, point)
	c.mu.Unlock()
}

func (c *TimeSeriesCollector) GetPoints() []TimeSeriesPoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TimeSeriesPoint{}, c.points...)
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

// 订单落库 TPS统计结果
type E2EStats struct {
	TotalOrders  int64
	FirstRequest time.Time
	LastWrite    time.Time
	DurationSec  float64
	E2ETPS       float64
}

// 查询订单落库 TPS（从MySQL统计）
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

// 生成HTML报告
func generateHTMLReport(
	stats *Stats,
	latencyCollector *LatencyCollector,
	timeSeries *TimeSeriesCollector,
	actualDuration float64,
	completedRequests int64,
	e2eStats *E2EStats,
	startTime time.Time,
) error {
	points := timeSeries.GetPoints()

	// 准备图表数据
	var timestamps, qpsData, successQPSData, avgLatencyData, p95LatencyData, p99LatencyData, successRateData []string
	for _, p := range points {
		timestamps = append(timestamps, fmt.Sprintf("%.1f", p.Timestamp.Sub(startTime).Seconds()))
		qpsData = append(qpsData, fmt.Sprintf("%.2f", p.QPS))
		successQPSData = append(successQPSData, fmt.Sprintf("%.2f", p.SuccessQPS))
		avgLatencyData = append(avgLatencyData, fmt.Sprintf("%.2f", p.AvgLatency))
		p95LatencyData = append(p95LatencyData, fmt.Sprintf("%.2f", p.P95Latency))
		p99LatencyData = append(p99LatencyData, fmt.Sprintf("%.2f", p.P99Latency))
		successRateData = append(successRateData, fmt.Sprintf("%.2f", p.SuccessRate))
	}

	// 计算延迟分布（直方图）
	latencyCollector.mu.Lock()
	latencies := make([]int64, len(latencyCollector.latencies))
	copy(latencies, latencyCollector.latencies)
	latencyCollector.mu.Unlock()

	// 延迟分布区间（毫秒）
	buckets := []float64{0, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000}
	bucketCounts := make([]int, len(buckets))
	for _, lat := range latencies {
		latMs := float64(lat) / 1e6
		for i := len(buckets) - 1; i >= 0; i-- {
			if latMs >= buckets[i] {
				bucketCounts[i]++
				break
			}
		}
	}

	var bucketLabels, bucketValues []string
	for i, count := range bucketCounts {
		if i < len(buckets)-1 {
			bucketLabels = append(bucketLabels, fmt.Sprintf("%.0f-%.0fms", buckets[i], buckets[i+1]))
		} else {
			bucketLabels = append(bucketLabels, fmt.Sprintf("%.0fms+", buckets[i]))
		}
		bucketValues = append(bucketValues, fmt.Sprintf("%d", count))
	}

	// 计算核心指标
	avgLatency := float64(stats.CompletedLatency) / float64(completedRequests) / 1e6
	systemQPS := float64(completedRequests) / actualDuration
	seckillQPS := 0.0
	seckillDuration := 0.0
	if stats.LastSuccessTime > 0 && stats.SuccessRequests > 0 {
		seckillDuration = float64(stats.LastSuccessTime-startTime.UnixNano()) / 1e9
		seckillQPS = float64(stats.SuccessRequests) / seckillDuration
	}

	e2eTPS := 0.0
	e2eOrders := int64(0)
	if e2eStats != nil {
		e2eTPS = e2eStats.E2ETPS
		e2eOrders = e2eStats.TotalOrders
	}

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>秒杀压测报告</title>
    <script src="https://cdn.jsdelivr.net/npm/echarts@5.4.3/dist/echarts.min.js"></script>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1400px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        h1 { color: #333; border-bottom: 3px solid #4CAF50; padding-bottom: 10px; }
        h2 { color: #555; margin-top: 30px; border-left: 4px solid #4CAF50; padding-left: 10px; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin: 20px 0; }
        .metric-card { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 20px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
        .metric-card:nth-child(2) { background: linear-gradient(135deg, #f093fb 0%%, #f5576c 100%%); }
        .metric-card:nth-child(3) { background: linear-gradient(135deg, #4facfe 0%%, #00f2fe 100%%); }
        .metric-card:nth-child(4) { background: linear-gradient(135deg, #43e97b 0%%, #38f9d7 100%%); }
        .metric-card:nth-child(5) { background: linear-gradient(135deg, #fa709a 0%%, #fee140 100%%); }
        .metric-card:nth-child(6) { background: linear-gradient(135deg, #30cfd0 0%%, #330867 100%%); }
        .metric-label { font-size: 14px; opacity: 0.9; margin-bottom: 5px; }
        .metric-value { font-size: 32px; font-weight: bold; }
        .metric-unit { font-size: 16px; opacity: 0.8; margin-left: 5px; }
        .chart { width: 100%%; height: 400px; margin: 20px 0; }
        table { width: 100%%; border-collapse: collapse; margin: 20px 0; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #4CAF50; color: white; }
        tr:hover { background-color: #f5f5f5; }
        .timestamp { color: #888; font-size: 14px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 秒杀压测报告</h1>
        <div class="timestamp">生成时间: %s</div>
        
        <h2>核心指标</h2>
        <div class="metrics">
            <div class="metric-card">
                <div class="metric-label">秒杀QPS</div>
                <div class="metric-value">%.2f<span class="metric-unit">req/s</span></div>
            </div>
            <div class="metric-card">
                <div class="metric-label">系统QPS</div>
                <div class="metric-value">%.2f<span class="metric-unit">req/s</span></div>
            </div>
            <div class="metric-card">
                <div class="metric-label">成功请求</div>
                <div class="metric-value">%d<span class="metric-unit">次</span></div>
            </div>
            <div class="metric-card">
                <div class="metric-label">平均延迟</div>
                <div class="metric-value">%.2f<span class="metric-unit">ms</span></div>
            </div>
            <div class="metric-card">
                <div class="metric-label">P99延迟</div>
                <div class="metric-value">%.2f<span class="metric-unit">ms</span></div>
            </div>
            <div class="metric-card">
                <div class="metric-label">订单落库 TPS</div>
                <div class="metric-value">%.2f<span class="metric-unit">order/s</span></div>
            </div>
        </div>

        <h2>QPS趋势</h2>
        <div id="qpsChart" class="chart"></div>

        <h2>延迟趋势</h2>
        <div id="latencyChart" class="chart"></div>

        <h2>成功率趋势</h2>
        <div id="successRateChart" class="chart"></div>

        <h2>延迟分布</h2>
        <div id="latencyDistChart" class="chart"></div>

        <h2>请求统计</h2>
        <div id="requestPieChart" class="chart"></div>

        <h2>详细数据</h2>
        <table>
            <tr><th>指标</th><th>数值</th></tr>
            <tr><td>实际耗时</td><td>%.2f s</td></tr>
            <tr><td>总请求数</td><td>%d</td></tr>
            <tr><td>完成请求</td><td>%d</td></tr>
            <tr><td>成功请求</td><td>%d</td></tr>
            <tr><td>已售罄响应</td><td>%d</td></tr>
            <tr><td>超过限购</td><td>%d</td></tr>
            <tr><td>失败请求</td><td>%d</td></tr>
            <tr><td>被取消请求</td><td>%d (压测结束时未收到响应)</td></tr>
            <tr><td>抢购阶段耗时</td><td>%.3f s</td></tr>
            <tr><td>秒杀QPS</td><td>%.2f req/s</td></tr>
            <tr><td>系统QPS</td><td>%.2f req/s</td></tr>
            <tr><td>平均延迟</td><td>%.2f ms</td></tr>
            <tr><td>P50延迟</td><td>%.2f ms</td></tr>
            <tr><td>P95延迟</td><td>%.2f ms</td></tr>
            <tr><td>P99延迟</td><td>%.2f ms</td></tr>
            <tr><td>MySQL订单数</td><td>%d</td></tr>
            <tr><td>订单落库 TPS</td><td>%.2f order/s</td></tr>
        </table>
    </div>

    <script>
        // QPS趋势图
        var qpsChart = echarts.init(document.getElementById('qpsChart'));
        qpsChart.setOption({
            title: { text: 'QPS时间序列', left: 'center' },
            tooltip: { trigger: 'axis' },
            legend: { data: ['总QPS', '成功QPS'], bottom: 10 },
            xAxis: { type: 'category', data: [%s], name: '时间(s)' },
            yAxis: { type: 'value', name: 'QPS' },
            series: [
                { name: '总QPS', type: 'line', data: [%s], smooth: true, itemStyle: { color: '#5470c6' } },
                { name: '成功QPS', type: 'line', data: [%s], smooth: true, itemStyle: { color: '#91cc75' } }
            ]
        });

        // 延迟趋势图
        var latencyChart = echarts.init(document.getElementById('latencyChart'));
        latencyChart.setOption({
            title: { text: '延迟时间序列', left: 'center' },
            tooltip: { trigger: 'axis' },
            legend: { data: ['平均延迟', 'P95延迟', 'P99延迟'], bottom: 10 },
            xAxis: { type: 'category', data: [%s], name: '时间(s)' },
            yAxis: { type: 'value', name: '延迟(ms)' },
            series: [
                { name: '平均延迟', type: 'line', data: [%s], smooth: true, itemStyle: { color: '#fac858' } },
                { name: 'P95延迟', type: 'line', data: [%s], smooth: true, itemStyle: { color: '#ee6666' } },
                { name: 'P99延迟', type: 'line', data: [%s], smooth: true, itemStyle: { color: '#73c0de' } }
            ]
        });

        // 成功率趋势图
        var successRateChart = echarts.init(document.getElementById('successRateChart'));
        successRateChart.setOption({
            title: { text: '成功率时间序列', left: 'center' },
            tooltip: { trigger: 'axis', formatter: '{b}s<br/>{a}: {c}%%' },
            xAxis: { type: 'category', data: [%s], name: '时间(s)' },
            yAxis: { type: 'value', name: '成功率(%%)', min: 0, max: 100 },
            series: [
                { name: '成功率', type: 'line', data: [%s], smooth: true, areaStyle: {}, itemStyle: { color: '#91cc75' } }
            ]
        });

        // 延迟分布直方图
        var latencyDistChart = echarts.init(document.getElementById('latencyDistChart'));
        latencyDistChart.setOption({
            title: { text: '延迟分布直方图', left: 'center' },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: [%s], name: '延迟区间' },
            yAxis: { type: 'value', name: '请求数' },
            series: [
                { name: '请求数', type: 'bar', data: [%s], itemStyle: { color: '#5470c6' } }
            ]
        });

        // 请求统计饼图
        var requestPieChart = echarts.init(document.getElementById('requestPieChart'));
        requestPieChart.setOption({
            title: { text: '请求结果分布', left: 'center' },
            tooltip: { trigger: 'item', formatter: '{a}<br/>{b}: {c} ({d}%%)' },
            legend: { orient: 'vertical', left: 'left' },
            series: [
                {
                    name: '请求结果',
                    type: 'pie',
                    radius: '50%%',
                    data: [
                        { value: %d, name: '成功请求', itemStyle: { color: '#91cc75' } },
                        { value: %d, name: '已售罄', itemStyle: { color: '#fac858' } },
                        { value: %d, name: '超过限购', itemStyle: { color: '#ee6666' } },
                        { value: %d, name: '失败请求', itemStyle: { color: '#73c0de' } }
                    ],
                    emphasis: {
                        itemStyle: {
                            shadowBlur: 10,
                            shadowOffsetX: 0,
                            shadowColor: 'rgba(0, 0, 0, 0.5)'
                        }
                    }
                }
            ]
        });

        // 响应式调整
        window.addEventListener('resize', function() {
            qpsChart.resize();
            latencyChart.resize();
            successRateChart.resize();
            latencyDistChart.resize();
            requestPieChart.resize();
        });
    </script>
</body>
</html>`,
		time.Now().Format("2006-01-02 15:04:05"),
		seckillQPS, systemQPS, stats.SuccessRequests, avgLatency,
		latencyCollector.Percentile(99), e2eTPS,
		actualDuration, stats.TotalRequests, completedRequests,
		stats.SuccessRequests, stats.SoldOut, stats.LimitExceed,
		stats.FailedRequests, stats.Canceled,
		seckillDuration, seckillQPS, systemQPS,
		avgLatency, latencyCollector.Percentile(50),
		latencyCollector.Percentile(95), latencyCollector.Percentile(99),
		e2eOrders, e2eTPS,
		// 图表数据
		"'"+strings.Join(timestamps, "','")+"'",
		strings.Join(qpsData, ","),
		strings.Join(successQPSData, ","),
		"'"+strings.Join(timestamps, "','")+"'",
		strings.Join(avgLatencyData, ","),
		strings.Join(p95LatencyData, ","),
		strings.Join(p99LatencyData, ","),
		"'"+strings.Join(timestamps, "','")+"'",
		strings.Join(successRateData, ","),
		"'"+strings.Join(bucketLabels, "','")+"'",
		strings.Join(bucketValues, ","),
		stats.SuccessRequests, stats.SoldOut, stats.LimitExceed, stats.FailedRequests,
	)

	filename := fmt.Sprintf("benchmark_report_%s.html", time.Now().Format("20060102_150405"))
	return os.WriteFile(filename, []byte(htmlContent), 0644)
}

func main() {
	// 初始化 HTTP 客户端
	initHTTPClient()

	// 配置参数
	concurrency := 500   // 并发数
	targetQPS := 20000   // 目标QPS（每秒请求数），0表示不限制
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
	var timeSeriesCollector TimeSeriesCollector
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
							// 压测结束时未收到响应的请求不计入延迟统计
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
						// 记录最后一次成功时间
						atomic.StoreInt64(&stats.LastSuccessTime, time.Now().UnixNano())
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

	// 启动实时进度显示
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		lastTotal := int64(0)
		lastSuccess := int64(0)
		lastLatencies := []int64{}

		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime).Seconds()
				currentTotal := atomic.LoadInt64(&stats.TotalRequests)
				currentSuccess := atomic.LoadInt64(&stats.SuccessRequests)
				currentCompleted := currentTotal - atomic.LoadInt64(&stats.Canceled)

				// 计算当前秒的QPS
				currentQPS := float64(currentTotal - lastTotal)
				currentSuccessQPS := float64(currentSuccess - lastSuccess)

				// 计算当前延迟百分位
				latencyCollector.mu.Lock()
				newLatencies := latencyCollector.latencies[len(lastLatencies):]
				if len(newLatencies) > 0 {
					sorted := make([]int64, len(newLatencies))
					copy(sorted, newLatencies)
					sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

					avgLat := float64(0)
					for _, lat := range sorted {
						avgLat += float64(lat)
					}
					avgLat = avgLat / float64(len(sorted)) / 1e6

					p95Lat := float64(sorted[int(float64(len(sorted)-1)*0.95)]) / 1e6
					p99Lat := float64(sorted[int(float64(len(sorted)-1)*0.99)]) / 1e6

					successRate := 0.0
					if currentCompleted > 0 {
						successRate = float64(currentSuccess) / float64(currentCompleted) * 100
					}

					// 记录时序数据
					timeSeriesCollector.Add(TimeSeriesPoint{
						Timestamp:   time.Now(),
						QPS:         currentQPS,
						SuccessQPS:  currentSuccessQPS,
						AvgLatency:  avgLat,
						P95Latency:  p95Lat,
						P99Latency:  p99Lat,
						SuccessRate: successRate,
					})

					// 实时进度显示
					fmt.Printf("\r[%.1fs] QPS: %.0f | 成功QPS: %.0f | 总请求: %d | 成功: %d | 平均延迟: %.2fms | P95: %.2fms | 成功率: %.1f%%",
						elapsed, currentQPS, currentSuccessQPS, currentTotal, currentSuccess, avgLat, p95Lat, successRate)

					lastLatencies = append(lastLatencies, newLatencies...)
				}
				latencyCollector.mu.Unlock()

				lastTotal = currentTotal
				lastSuccess = currentSuccess

			case <-progressDone:
				return
			}
		}
	}()

	// 等待测试时间后立即停止
	time.Sleep(time.Duration(duration) * time.Second)
	actualDuration := time.Since(startTime).Seconds()
	close(progressDone)
	fmt.Println() // 换行

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
	log("被取消请求:   %d (压测结束时未收到响应，服务器可能已处理)", stats.Canceled)
	log("剩余库存:     %d", finalStock)
	log("平均延迟:     %.2f ms", avgLatency)
	log("P50延迟:      %.2f ms", latencyCollector.Percentile(50))
	log("P95延迟:      %.2f ms", latencyCollector.Percentile(95))
	log("P99延迟:      %.2f ms", latencyCollector.Percentile(99))

	// 计算秒杀QPS（有效QPS）
	if stats.LastSuccessTime > 0 && stats.SuccessRequests > 0 {
		seckillDuration := float64(stats.LastSuccessTime-startTime.UnixNano()) / 1e9 // 秒
		log("")
		log("=== 核心指标 ===")
		log("抢购阶段耗时: %.3f s (最后一次成功)", seckillDuration)
		log("秒杀QPS:      %.2f (有效秒杀能力)", float64(stats.SuccessRequests)/seckillDuration)
		log("系统QPS:      %.2f (API总吞吐)", float64(completedRequests)/actualDuration)

		// 如果有售罄时间，也显示参考信息
		if stats.FirstSoldOutTime > 0 {
			soldOutDuration := float64(stats.FirstSoldOutTime-startTime.UnixNano()) / 1e9
			log("首次售罄耗时: %.3f s (参考)", soldOutDuration)
		}
	} else if stats.SuccessRequests > 0 {
		// 没有成功时间记录，用总时间计算
		log("")
		log("=== 核心指标 ===")
		log("秒杀QPS:      %.2f (库存未售罄，基于总时间)", float64(stats.SuccessRequests)/actualDuration)
		log("系统QPS:      %.2f (API总吞吐)", float64(completedRequests)/actualDuration)
	} else {
		log("系统QPS:      %.2f (API总吞吐)", float64(completedRequests)/actualDuration)
	}

	// 等待 Worker 处理完成，查询订单落库  TPS
	var e2eStats *E2EStats
	if stats.SuccessRequests > 0 {
		log("")
		log("等待订单写入MySQL完成...")
		time.Sleep(10 * time.Second) // 等待 Worker 处理

		e2eStatsResult, err := getE2EStats(GoodsID)
		if err != nil {
			log("查询订单落库 TPS失败: %v", err)
		} else {
			e2eStats = e2eStatsResult
			log("")
			log("=== 订单落库 TPS (订单落库) ===")
			log("MySQL订单数:  %d", e2eStats.TotalOrders)
			log("首个请求时间: %s", e2eStats.FirstRequest.Format("15:04:05.000"))
			log("最后写入时间: %s", e2eStats.LastWrite.Format("15:04:05.000"))
			log("订单落库 耗时:   %.2f s", e2eStats.DurationSec)
			log("订单落库 TPS:    %.2f (订单从请求到落库)", e2eStats.E2ETPS)
		}
	}

	// 生成HTML报告
	log("")
	log("生成HTML报告...")
	if err := generateHTMLReport(&stats, &latencyCollector, &timeSeriesCollector, actualDuration, completedRequests, e2eStats, startTime); err != nil {
		log("生成报告失败: %v", err)
	} else {
		reportFile := fmt.Sprintf("benchmark_report_%s.html", time.Now().Format("20060102_150405"))
		log("报告已生成: %s", reportFile)
	}
}
