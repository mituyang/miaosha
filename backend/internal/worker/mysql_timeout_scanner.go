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

// MySQLTimeoutScanner MySQL 兜底超时扫描器
type MySQLTimeoutScanner struct {
	cfg            *config.Config
	goodsRepo      *repository.GoodsRepository
	orderRepo      *repository.OrderRepository
	stopChan       chan struct{}
	wg             sync.WaitGroup
	interval       time.Duration
	timeoutSeconds int
	batchSize      int
}

// NewMySQLTimeoutScanner 创建 MySQL 兜底扫描器
func NewMySQLTimeoutScanner(cfg *config.Config) *MySQLTimeoutScanner {
	// 默认值
	interval := 5 * time.Minute
	if cfg.Timeout.MySQLScanInterval != "" {
		if d, err := time.ParseDuration(cfg.Timeout.MySQLScanInterval); err == nil {
			interval = d
		}
	}

	timeoutSeconds := cfg.Timeout.OrderTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}

	batchSize := cfg.Timeout.MySQLBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	return &MySQLTimeoutScanner{
		cfg:            cfg,
		goodsRepo:      repository.NewGoodsRepository(database.DB),
		orderRepo:      repository.NewOrderRepository(database.DB),
		stopChan:       make(chan struct{}),
		interval:       interval,
		timeoutSeconds: timeoutSeconds,
		batchSize:      batchSize,
	}
}

// Start 启动扫描器
func (s *MySQLTimeoutScanner) Start() {
	s.wg.Add(1)
	go s.scanLoop()
	logger.Info.Printf("MySQL timeout scanner started, interval: %v, timeout: %ds",
		s.interval, s.timeoutSeconds)
}

// scanLoop 扫描循环
func (s *MySQLTimeoutScanner) scanLoop() {
	defer s.wg.Done()

	// 启动后立即执行一次
	s.processExpiredOrders()

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
func (s *MySQLTimeoutScanner) processExpiredOrders() {
	ctx := context.Background()

	// 查询超时的待支付订单
	// status=0 AND write_time < NOW() - timeout - 1分钟（比 Redis 扫描多等1分钟，作为兜底）
	threshold := time.Now().Add(-time.Duration(s.timeoutSeconds)*time.Second - time.Minute)
	orders, err := s.orderRepo.FindExpiredUnpaidOrders(threshold, s.batchSize)
	if err != nil {
		logger.Error.Printf("find expired orders failed: %v", err)
		return
	}

	if len(orders) == 0 {
		return
	}

	logger.Info.Printf("MySQL fallback: processing %d expired orders", len(orders))

	for _, order := range orders {
		if err := s.cancelOrder(ctx, order.ID, order.UserID, order.GoodsID, order.Quantity); err != nil {
			logger.Error.Printf("MySQL fallback cancel order %d failed: %v", order.ID, err)
		}
	}
}

// cancelOrder 取消订单
func (s *MySQLTimeoutScanner) cancelOrder(ctx context.Context, orderID, userID, goodsID uint64, quantity int) error {
	// 使用 CAS 更新，只有 status=0 才能取消
	affected, err := s.orderRepo.CancelOrder(orderID, time.Now())
	if err != nil {
		return err
	}

	// affected=0 表示订单不存在或已经不是待支付状态
	if affected == 0 {
		return nil
	}

	if quantity <= 0 {
		quantity = 1
	}

	// 返还库存（MySQL 兜底扫描没有 segmentID，返还到分段 0）
	_ = s.goodsRepo.IncrStockBatch(goodsID, quantity)
	_ = redis.IncrSegmentStockBy(ctx, goodsID, 0, quantity)

	// 清除用户标记，允许重新抢购
	_ = redis.ClearUserBought(ctx, goodsID, userID, quantity)
	_ = redis.ClearProcessed(ctx, goodsID, userID, quantity)
	_ = redis.MarkAdminOrderCancelled(ctx, goodsID, quantity)

	logger.Info.Printf("order cancelled by MySQL fallback: orderID=%d", orderID)
	return nil
}

// Stop 停止扫描器
func (s *MySQLTimeoutScanner) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	logger.Info.Println("MySQL timeout scanner stopped")
}
