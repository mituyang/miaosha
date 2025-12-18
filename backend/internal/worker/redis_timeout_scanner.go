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
			s.processExpiredOrders()
		}
	}
}

// processExpiredOrders 处理过期订单
func (s *RedisTimeoutScanner) processExpiredOrders() {
	ctx := context.Background()

	// 循环处理，直到没有过期订单
	for {
		// 获取过期订单（原子操作）
		items, err := redis.PopExpiredOrders(ctx, int64(s.batchSize))
		if err != nil {
			logger.Error.Printf("pop expired orders failed: %v", err)
			return
		}

		if len(items) == 0 {
			return
		}

		logger.Info.Printf("batch cancelling %d expired orders", len(items))

		// 批量取消订单
		s.batchCancelOrders(ctx, items)
	}
}

// batchCancelOrders 批量取消订单
func (s *RedisTimeoutScanner) batchCancelOrders(ctx context.Context, items []redis.OrderTimeoutItem) {
	if len(items) == 0 {
		return
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
		return
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
		_ = s.goodsRepo.IncrStockBatch(goodsID, count)
	}

	// 批量 Redis 操作
	_ = redis.BatchRestoreStock(ctx, cancelledItems)

	logger.Info.Printf("batch cancelled %d orders", len(cancelledIDs))
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
