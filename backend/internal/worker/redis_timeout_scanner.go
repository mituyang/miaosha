package worker

import (
	"context"
	"sync"
	"time"

	"seckill/internal/config"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"
)

// RedisTimeoutScanner Redis 超时扫描器
type RedisTimeoutScanner struct {
	cfg           *config.Config
	goodsRepo     *repository.GoodsRepository
	orderRepo     *repository.OrderRepository
	stopChan      chan struct{}
	wg            sync.WaitGroup
	interval      time.Duration
	batchSize     int
	maxRetryDelay time.Duration

	// 熔断机制
	consecutiveFailures int           // 连续失败次数
	maxFailures         int           // 触发熔断的最大失败次数
	backoffDuration     time.Duration // 当前退避时间
	maxBackoff          time.Duration // 最大退避时间
	baseBackoff         time.Duration // 基础退避时间
}

// NewRedisTimeoutScanner 创建 Redis 超时扫描器
func NewRedisTimeoutScanner(cfg *config.Config) *RedisTimeoutScanner {
	// 默认值
	interval := 500 * time.Millisecond
	if cfg.Timeout.RedisScanInterval != "" {
		if d, err := time.ParseDuration(cfg.Timeout.RedisScanInterval); err == nil {
			interval = d
		}
	}

	batchSize := cfg.Timeout.RedisBatchSize
	if batchSize <= 0 {
		batchSize = 2000
	}

	maxRetryDelay := time.Duration(cfg.Timeout.MaxRetryDelayMs) * time.Millisecond
	if maxRetryDelay <= 0 {
		maxRetryDelay = 5 * time.Second
	}

	return &RedisTimeoutScanner{
		cfg:           cfg,
		goodsRepo:     repository.NewGoodsRepository(database.DB),
		orderRepo:     repository.NewOrderRepository(database.DB),
		stopChan:      make(chan struct{}),
		interval:      interval,
		batchSize:     batchSize,
		maxRetryDelay: maxRetryDelay,
		// 熔断机制默认值
		maxFailures: 5,                // 连续5次失败触发熔断
		baseBackoff: 1 * time.Second,  // 基础退避1秒
		maxBackoff:  30 * time.Second, // 最大退避30秒
	}
}

// Start 启动扫描器
func (s *RedisTimeoutScanner) Start() {
	s.wg.Add(1)
	go s.scanLoop()
	logger.Info.Printf("Redis timeout scanner started, interval: %v", s.interval)
}

// scanLoop 扫描循环
func (s *RedisTimeoutScanner) scanLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			// 检查是否处于熔断状态
			if s.backoffDuration > 0 {
				logger.Info.Printf("circuit breaker active, waiting %v before retry", s.backoffDuration)
				select {
				case <-s.stopChan:
					return
				case <-time.After(s.backoffDuration):
					// 退避结束，尝试恢复
				}
			}

			success := s.processExpiredOrders()
			s.updateCircuitBreaker(success)
		}
	}
}

// updateCircuitBreaker 更新熔断状态
func (s *RedisTimeoutScanner) updateCircuitBreaker(success bool) {
	if success {
		// 成功则重置熔断状态
		if s.consecutiveFailures > 0 {
			logger.Info.Printf("circuit breaker reset after %d consecutive failures", s.consecutiveFailures)
		}
		s.consecutiveFailures = 0
		s.backoffDuration = 0
	} else {
		// 失败则增加计数
		s.consecutiveFailures++
		if s.consecutiveFailures >= s.maxFailures {
			// 触发熔断，计算指数退避时间
			s.backoffDuration = s.baseBackoff * time.Duration(1<<uint(s.consecutiveFailures-s.maxFailures))
			if s.backoffDuration > s.maxBackoff {
				s.backoffDuration = s.maxBackoff
			}
			logger.Error.Printf("circuit breaker triggered after %d failures, backoff: %v",
				s.consecutiveFailures, s.backoffDuration)
		}
	}
}

// processExpiredOrders 处理过期订单，返回是否成功
func (s *RedisTimeoutScanner) processExpiredOrders() bool {
	ctx := context.Background()
	hasError := false

	// 循环处理，直到没有过期订单
	for {
		// 获取过期订单（原子操作）
		items, err := redis.PopExpiredOrders(ctx, int64(s.batchSize))
		if err != nil {
			logger.Error.Printf("pop expired orders failed: %v", err)
			return false
		}

		if len(items) == 0 {
			return !hasError
		}

		logger.Info.Printf("batch cancelling %d expired orders", len(items))

		// 批量取消订单
		if err := s.batchCancelOrders(ctx, items); err != nil {
			hasError = true
		}
	}
}

// batchCancelOrders 批量取消订单
func (s *RedisTimeoutScanner) batchCancelOrders(ctx context.Context, items []redis.OrderTimeoutItem) error {
	if len(items) == 0 {
		return nil
	}

	// 构建订单ID列表和映射
	orderIDs := make([]uint64, len(items))
	itemMap := make(map[uint64]redis.OrderTimeoutItem, len(items))
	for i, item := range items {
		orderIDs[i] = item.OrderID
		itemMap[item.OrderID] = item
	}

	// 批量取消订单
	cancelTime := time.Now()
	cancelledIDs, err := s.orderRepo.BatchCancelOrders(orderIDs, cancelTime)
	if err != nil {
		logger.Error.Printf("batch cancel orders failed: %v", err)
		// 失败的重新入队
		expireAt := time.Now().Add(s.maxRetryDelay)
		for _, item := range items {
			_ = redis.AddOrderTimeout(ctx, item.OrderID, item.UserID, item.GoodsID, item.SegmentID, expireAt)
		}
		return err
	}

	// 批量返还库存和清除标记
	cancelledItems := make([]redis.OrderTimeoutItem, 0, len(cancelledIDs))
	for _, orderID := range cancelledIDs {
		cancelledItems = append(cancelledItems, itemMap[orderID])
	}

	// 批量 MySQL 返还库存（按商品分组）
	goodsStockMap := make(map[uint64]int)
	for _, item := range cancelledItems {
		goodsStockMap[item.GoodsID]++
	}
	for goodsID, count := range goodsStockMap {
		if err := s.goodsRepo.IncrStockBatch(goodsID, count); err != nil {
			logger.Error.Printf("incr stock batch failed: goodsID=%d, count=%d, err=%v", goodsID, count, err)
		}
	}

	// 批量 Redis 操作
	if err := redis.BatchRestoreStock(ctx, cancelledItems); err != nil {
		logger.Error.Printf("batch restore stock failed: %v", err)
	}

	logger.Info.Printf("batch cancelled %d orders", len(cancelledIDs))
	return nil
}

// cancelOrder 取消订单
func (s *RedisTimeoutScanner) cancelOrder(ctx context.Context, item redis.OrderTimeoutItem) error {
	// 使用 CAS 更新，只有 status=0 才能取消
	affected, err := s.orderRepo.CancelOrder(item.OrderID, time.Now())
	if err != nil {
		return err
	}

	// affected=0 表示订单不存在或已经不是待支付状态
	if affected == 0 {
		return nil
	}

	// 返还库存
	_ = s.goodsRepo.IncrStock(item.GoodsID)
	_ = redis.IncrSegmentStock(ctx, item.GoodsID, item.SegmentID)

	// 清除用户标记，允许重新抢购
	_ = redis.ClearUserBought(ctx, item.GoodsID, item.UserID)
	_ = redis.ClearProcessed(ctx, item.GoodsID, item.UserID)

	logger.Info.Printf("order cancelled by redis scanner: orderID=%d", item.OrderID)
	return nil
}

// Stop 停止扫描器
func (s *RedisTimeoutScanner) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	logger.Info.Println("Redis timeout scanner stopped")
}
