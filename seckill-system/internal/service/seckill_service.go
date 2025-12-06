package service

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"seckill-system/pkg/kafka"
	"seckill-system/pkg/redis"
)

// SeckillService 秒杀业务逻辑层
type SeckillService struct{}

// NewSeckillService 创建秒杀服务实例
func NewSeckillService() *SeckillService {
	return &SeckillService{}
}

// SeckillResult 秒杀结果
type SeckillResult struct {
	Code    int32 // 0-成功, 1-库存不足, 2-重复秒杀, 3-系统错误
	Message string
	OrderID string
}

// DoSeckill 执行秒杀核心逻辑
// 保证原子性：Redis 扣减成功 + Kafka 发送成功 才算秒杀成功
// 如果 Kafka 发送失败，自动回滚 Redis 库存
func (s *SeckillService) DoSeckill(ctx context.Context, userID, goodsID int64) (*SeckillResult, error) {
	// 1. 执行 Redis Lua 脚本，原子性扣减库存并检查重复
	result, err := redis.DoSeckill(ctx, goodsID, userID)
	if err != nil {
		return &SeckillResult{
			Code:    3,
			Message: "系统繁忙，请稍后重试",
		}, fmt.Errorf("redis 秒杀失败: %w", err)
	}

	// 2. 根据 Lua 脚本返回值判断结果
	switch result {
	case -1:
		return &SeckillResult{
			Code:    2,
			Message: "您已经参与过此商品的秒杀",
		}, nil
	case 0:
		return &SeckillResult{
			Code:    1,
			Message: "商品已售罄",
		}, nil
	case 1:
		// Redis 扣减成功，继续发送 Kafka
		return s.sendToKafkaWithRollback(ctx, userID, goodsID)
	default:
		return &SeckillResult{
			Code:    3,
			Message: "系统异常",
		}, fmt.Errorf("未知的秒杀结果: %d", result)
	}
}

// sendToKafkaWithRollback 发送 Kafka 消息，失败时回滚 Redis
func (s *SeckillService) sendToKafkaWithRollback(ctx context.Context, userID, goodsID int64) (*SeckillResult, error) {
	orderID := generateOrderID()

	orderMsg := &kafka.OrderMessage{
		OrderID: orderID,
		UserID:  userID,
		GoodsID: goodsID,
	}

	// 同步发送到 Kafka，等待确认
	if err := kafka.SendOrderMessage(ctx, orderMsg); err != nil {
		log.Printf("Kafka 发送失败，开始回滚 Redis: userID=%d, goodsID=%d, error=%v", userID, goodsID, err)

		// Kafka 发送失败，回滚 Redis 库存
		if rollbackErr := redis.RollbackSeckill(ctx, goodsID, userID); rollbackErr != nil {
			// 回滚也失败了，记录日志，需要人工介入或定时任务修复
			log.Printf("严重错误：Redis 回滚失败: userID=%d, goodsID=%d, error=%v", userID, goodsID, rollbackErr)
		}

		return &SeckillResult{
			Code:    3,
			Message: "系统繁忙，请稍后重试",
		}, fmt.Errorf("发送订单消息失败并已回滚: %w", err)
	}

	// Kafka 发送成功，秒杀完成
	log.Printf("秒杀成功: userID=%d, goodsID=%d, orderID=%s", userID, goodsID, orderID)

	return &SeckillResult{
		Code:    0,
		Message: "秒杀成功，请尽快完成支付",
		OrderID: orderID,
	}, nil
}

// generateOrderID 生成唯一订单号
func generateOrderID() string {
	return uuid.New().String()
}
