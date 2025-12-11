package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"seckill/internal/config"
	"seckill/internal/dto"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/mq"
	"seckill/pkg/redis"
)

type Result int

const (
	ResultSuccess   Result = 1
	ResultSoldOut   Result = 0
	ResultRepeatBuy Result = -1
	ResultError     Result = -99
)

type SeckillService struct {
	cfg       *config.Config
	goodsRepo *repository.GoodsRepository
}

func NewSeckillService(cfg *config.Config) *SeckillService {
	return &SeckillService{
		cfg:       cfg,
		goodsRepo: repository.NewGoodsRepository(database.DB),
	}
}

// DoSeckill 秒杀核心逻辑: 检查资格 -> 发MQ -> Consumer扣库存
func (s *SeckillService) DoSeckill(ctx context.Context, userID, goodsID uint64) (Result, error) {
	// 1. 检查用户资格并标记（不扣库存）
	result, segmentID, err := redis.CheckAndMark(ctx, goodsID, userID)
	if err != nil {
		return ResultError, err
	}

	// 2. 根据 Lua 脚本返回值判断
	switch result {
	case redis.SeckillSoldOut:
		return ResultSoldOut, nil
	case redis.SeckillRepeatBuy:
		return ResultRepeatBuy, nil
	case redis.SeckillSuccess:
		// 3. 发送 MQ 消息，异步落库
		msg := dto.SeckillMessage{
			UserID:      userID,
			GoodsID:     goodsID,
			SegmentID:   segmentID,
			RequestTime: time.Now().UnixMilli(),
		}
		body, _ := json.Marshal(msg)

		if err := mq.SendSeckillMsg(ctx, s.cfg.RocketMQ.Topic, body); err != nil {
			// MQ 发送失败，清除用户标记（允许重试）
			_ = redis.ClearUserMark(ctx, goodsID, userID)
			return ResultError, err
		}

		return ResultSuccess, nil
	default:
		return ResultError, nil
	}
}

// GetStock 获取库存
func (s *SeckillService) GetStock(ctx context.Context, goodsID uint64) (int, error) {
	return redis.GetStock(ctx, goodsID)
}

// WarmUp 库存预热 - 将 MySQL 库存同步到 Redis（带分布式锁）
func (s *SeckillService) WarmUp(ctx context.Context, goodsID uint64) error {
	// 获取分布式锁
	acquired, err := redis.AcquireWarmupLock(ctx, goodsID)
	if err != nil {
		return fmt.Errorf("acquire warmup lock failed: %w", err)
	}
	if !acquired {
		return fmt.Errorf("warmup is already in progress for goods %d", goodsID)
	}
	defer redis.ReleaseWarmupLock(ctx, goodsID)

	goods, err := s.goodsRepo.GetByID(goodsID)
	if err != nil {
		return fmt.Errorf("get goods failed: %w", err)
	}

	// 清理旧数据
	if err := redis.ClearSeckillData(ctx, goodsID); err != nil {
		return fmt.Errorf("clear old data failed: %w", err)
	}

	// 初始化库存到 Redis
	if err := redis.InitStock(ctx, goodsID, int(goods.Stock)); err != nil {
		return fmt.Errorf("init stock failed: %w", err)
	}

	return nil
}

// WarmUpAll 预热所有商品库存（带分布式锁）
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

	goods, err := s.goodsRepo.GetAll()
	if err != nil {
		return 0, fmt.Errorf("get all goods failed: %w", err)
	}

	count := 0
	for _, g := range goods {
		if err := redis.ClearSeckillData(ctx, g.ID); err != nil {
			continue
		}
		if err := redis.InitStock(ctx, g.ID, int(g.Stock)); err != nil {
			continue
		}
		count++
	}

	return count, nil
}
