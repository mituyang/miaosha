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

const (
	defaultRedisScanInterval = time.Second
	defaultBatchSize         = 100
	maxRetryDelay            = 5 * time.Second
)

// RedisTimeoutScanner Redis 超时扫描器
type RedisTimeoutScanner struct {
	cfg       *config.Config
	goodsRepo *repository.GoodsRepository
	orderRepo *repository.OrderRepository
	stopChan  chan struct{}
	wg        sync.WaitGroup
	interval  time.Duration
}

// NewRedisTimeoutScanner 创建 Redis 超时扫描器
func NewRedisTimeoutScanner(cfg *config.Config) *RedisTimeoutScanner {
	interval := defaultRedisScanInterval
	if cfg.Timeout.RedisScanInterval != "" {
		if d, err := time.ParseDuration(cfg.Timeout.RedisScanInterval); err == nil {
			interval = d
		}
	}

	return &RedisTimeoutScanner{
		cfg:       cfg,
		goodsRepo: repository.NewGoodsRepository(database.DB),
		orderRepo: repository.NewOrderRepository(database.DB),
		stopChan:  make(chan struct{}),
		interval:  interval,
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

	// 获取过期订单（原子操作）
	items, err := redis.PopExpiredOrders(ctx, defaultBatchSize)
	if err != nil {
		logger.Error.Printf("pop expired orders failed: %v", err)
		return
	}

	if len(items) == 0 {
		return
	}

	logger.Info.Printf("processing %d expired orders", len(items))

	for _, item := range items {
		if err := s.cancelOrder(ctx, item); err != nil {
			logger.Error.Printf("cancel order %d failed: %v", item.OrderID, err)
			// 失败的重新入队，延迟重试
			expireAt := time.Now().Add(maxRetryDelay)
			_ = redis.AddOrderTimeout(ctx, item.OrderID, item.UserID, item.GoodsID, item.SegmentID, expireAt)
		}
	}
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
