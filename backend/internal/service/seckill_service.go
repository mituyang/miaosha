package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"seckill/internal/config"
	"seckill/internal/dto"
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/mq"
	"seckill/pkg/redis"
)

type Result int

const (
	ResultSuccess     Result = 1
	ResultSoldOut     Result = 0
	ResultLimitExceed Result = -1
	ResultNotOnSale   Result = -2
	ResultError       Result = -99
)

type SeckillService struct {
	cfg          *config.Config
	goodsRepo    *repository.GoodsRepository
	activityRepo *repository.SeckillActivityRepository
}

func NewSeckillService(cfg *config.Config) *SeckillService {
	return &SeckillService{
		cfg:          cfg,
		goodsRepo:    repository.NewGoodsRepository(database.DB),
		activityRepo: repository.NewSeckillActivityRepository(database.DB),
	}
}

// GetMaxBuyLimit 获取最大限购数量
func (s *SeckillService) GetMaxBuyLimit() int {
	return s.cfg.Seckill.MaxBuyLimit
}

// DoSeckill 秒杀核心逻辑: 检查资格 -> 发MQ -> Consumer扣库存
func (s *SeckillService) DoSeckill(ctx context.Context, userID, activityID, goodsID uint64, quantity int, requestTime time.Time) (Result, error) {
	if activityID == 0 {
		if goodsID == 0 {
			return ResultError, fmt.Errorf("activity_id or goods_id is required")
		}
		defaultActivityID, ok, err := redis.GetDefaultActivityID(ctx, goodsID)
		if err != nil {
			return ResultError, err
		}
		if !ok {
			return ResultNotOnSale, nil
		}
		activityID = defaultActivityID
	}

	meta, exists, err := redis.GetActivityMeta(ctx, activityID)
	if err != nil {
		return ResultError, err
	}
	if !exists || meta.GoodsID == 0 {
		return ResultNotOnSale, nil
	}

	if quantity <= 0 {
		return ResultError, fmt.Errorf("购买数量必须大于 0")
	}
	if quantity > meta.MaxBuyLimit {
		return ResultLimitExceed, nil
	}

	result, segmentID, err := redis.CheckAndMark(ctx, activityID, userID, quantity, requestTime.UnixMilli())
	if err != nil {
		return ResultError, err
	}

	// Redis 确认时间
	createTime := time.Now()

	// 2. 根据 Lua 脚本返回值判断
	switch result {
	case redis.SeckillSoldOut:
		return ResultSoldOut, nil
	case redis.SeckillLimitExceed:
		return ResultLimitExceed, nil
	case redis.SeckillNotOnSale:
		return ResultNotOnSale, nil
	case redis.SeckillSuccess:
		// 3. 发送 Kafka 消息，异步落库
		// BornTime 改由生产者在发送时通过 kafka.Message.Time 设置
		msg := dto.SeckillMessage{
			UserID:      userID,
			GoodsID:     meta.GoodsID,
			ActivityID:  activityID,
			SegmentID:   segmentID,
			Quantity:    quantity,
			RequestTime: requestTime.UnixMilli(),
			CreateTime:  createTime.UnixMilli(),
		}
		body, _ := json.Marshal(msg)

		// 使用 userID 作为 key，让消息分散到多个分区并行消费
		key := []byte(fmt.Sprintf("%d", userID))

		// 异步发送 Kafka，不阻塞用户请求
		go func() {
			// 使用独立 context，避免 HTTP 请求取消影响发送
			sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := mq.SendKafkaMsg(sendCtx, key, body); err != nil {
				// Kafka 发送失败，返还库存并清除用户标记
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer rollbackCancel()
				_ = redis.IncrSegmentStockBy(rollbackCtx, activityID, segmentID, quantity)
				_ = redis.ClearUserMark(rollbackCtx, activityID, userID, quantity)
				// 记录错误日志，便于排查
				logger.Error.Printf("async send kafka failed: userID=%d, activityID=%d, goodsID=%d, err=%v", userID, activityID, meta.GoodsID, err)
			}
		}()

		// 立即返回成功，不等待 Kafka 发送
		return ResultSuccess, nil
	default:
		return ResultError, nil
	}
}

// GetStock 获取库存
func (s *SeckillService) GetStock(ctx context.Context, goodsID uint64) (int, error) {
	activityID, ok, err := redis.GetDefaultActivityID(ctx, goodsID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return redis.GetStock(ctx, activityID)
}

// GetActivityStock 获取活动库存
func (s *SeckillService) GetActivityStock(ctx context.Context, activityID uint64) (int, error) {
	return redis.GetStock(ctx, activityID)
}

// WarmUp 默认活动库存预热 - 将 MySQL 库存同步到 Redis（带分布式锁）
func (s *SeckillService) WarmUp(ctx context.Context, goodsID uint64) error {
	activity, err := s.activityRepo.FindDefaultByGoodsID(goodsID)
	if err != nil {
		return fmt.Errorf("get default activity failed: %w", err)
	}
	return s.WarmUpActivity(ctx, activity.ID)
}

// WarmUpActivity 活动库存预热
func (s *SeckillService) WarmUpActivity(ctx context.Context, activityID uint64) error {
	// 获取分布式锁
	acquired, err := redis.AcquireWarmupLock(ctx, activityID)
	if err != nil {
		return fmt.Errorf("acquire warmup lock failed: %w", err)
	}
	if !acquired {
		return fmt.Errorf("warmup is already in progress for activity %d", activityID)
	}
	defer redis.ReleaseWarmupLock(ctx, activityID)

	activity, err := s.activityRepo.GetWithGoods(activityID)
	if err != nil {
		return fmt.Errorf("get activity failed: %w", err)
	}

	// 清理旧数据
	if err := redis.ClearSeckillData(ctx, activityID); err != nil {
		return fmt.Errorf("clear old data failed: %w", err)
	}

	goodsOnSale := activity.GoodsStatus == model.GoodsStatusOnSale
	warmupStatus := model.SeckillActivityWarmupPending
	activityEnabled := activity.Status == model.SeckillActivityStatusPending || activity.Status == model.SeckillActivityStatusRunning

	if activityEnabled && goodsOnSale {
		if err := redis.InitStock(ctx, activityID, int(activity.GoodsStock)); err != nil {
			return fmt.Errorf("init stock failed: %w", err)
		}
		warmupStatus = model.SeckillActivityWarmupDone
	}

	if err := redis.SetActivityMeta(ctx, redis.ActivityMeta{
		ActivityID:   activity.ID,
		GoodsID:      activity.GoodsID,
		Title:        activity.Title,
		Status:       activity.Status,
		StartTimeMs:  activity.StartTime.UnixMilli(),
		EndTimeMs:    activity.EndTime.UnixMilli(),
		MaxBuyLimit:  int(activity.MaxBuyLimit),
		WarmupStatus: warmupStatus,
		GoodsOnSale:  goodsOnSale,
		IsDefault:    activity.IsDefault,
	}); err != nil {
		return fmt.Errorf("set activity meta failed: %w", err)
	}

	if err := s.activityRepo.UpdateWarmupStatus(activityID, warmupStatus); err != nil {
		return fmt.Errorf("update activity warmup status failed: %w", err)
	}

	return nil
}

// WarmUpAll 预热所有活动库存（带分布式锁）
func (s *SeckillService) WarmUpAll(ctx context.Context) (int, error) {
	// 获取全量预热分布式锁
	acquired, err := redis.AcquireWarmupAllLock(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire warmup all lock failed: %w", err)
	}
	if !acquired {
		return 0, fmt.Errorf("warmup all is already in progress")
	}
	defer redis.ReleaseWarmupAllLock(ctx)

	activities, err := s.activityRepo.ListWarmupCandidates()
	if err != nil {
		return 0, fmt.Errorf("get all activities failed: %w", err)
	}

	count := 0
	for _, activity := range activities {
		if err := s.WarmUpActivity(ctx, activity.ID); err != nil {
			logger.Error.Printf("warmup activity failed: activityID=%d, err=%v", activity.ID, err)
			continue
		}
		count++
	}

	return count, nil
}

// RefreshActivitiesByGoods 刷新商品关联活动的 Redis 运行态
func (s *SeckillService) RefreshActivitiesByGoods(ctx context.Context, goodsID uint64) error {
	activities, err := s.activityRepo.ListByGoodsID(goodsID)
	if err != nil {
		return err
	}
	for _, activity := range activities {
		if err := s.WarmUpActivity(ctx, activity.ID); err != nil {
			return err
		}
	}
	return nil
}

// ClearActivitiesByGoods 清理商品关联活动 Redis 数据
func (s *SeckillService) ClearActivitiesByGoods(ctx context.Context, goodsID uint64) error {
	activities, err := s.activityRepo.ListByGoodsID(goodsID)
	if err != nil {
		return err
	}
	for _, activity := range activities {
		if err := redis.ClearSeckillData(ctx, activity.ID); err != nil {
			return err
		}
	}
	return redis.ClearDefaultActivityID(ctx, goodsID)
}
