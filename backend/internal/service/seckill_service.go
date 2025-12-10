package service

import (
	"context"
	"encoding/json"
	"fmt"

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

// DoSeckill 秒杀核心逻辑: Redis预减 -> 发MQ
func (s *SeckillService) DoSeckill(ctx context.Context, userID, goodsID uint64) (Result, error) {
	// 1. Redis 预减库存 (原子操作)
	result, err := redis.PreDecrStock(ctx, goodsID, userID)
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
			UserID:  userID,
			GoodsID: goodsID,
		}
		body, _ := json.Marshal(msg)

		if err := mq.SendSeckillMsg(ctx, s.cfg.RocketMQ.Topic, body); err != nil {
			// MQ 发送失败，回滚 Redis 库存
			_ = redis.RollbackStock(ctx, goodsID, userID)
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

// WarmUp 库存预热 - 将 MySQL 库存同步到 Redis
func (s *SeckillService) WarmUp(ctx context.Context, goodsID uint64) error {
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

// WarmUpAll 预热所有商品库存
func (s *SeckillService) WarmUpAll(ctx context.Context) (int, error) {
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
