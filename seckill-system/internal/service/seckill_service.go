package service

import (
	"context"
	"fmt"
	"log"
	"time"

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
// 流程：Redis 扣减库存 -> 同步写入数据库
// userID 为用户名（字符串）
func (s *SeckillService) DoSeckill(ctx context.Context, userID string, goodsID int64) (*SeckillResult, error) {
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
		log.Printf("秒杀失败（重复秒杀）: userID=%s, goodsID=%d", userID, goodsID)
		return &SeckillResult{
			Code:    2,
			Message: "您已经参与过此商品的秒杀",
		}, nil
	case 0:
		log.Printf("秒杀失败（库存不足）: userID=%s, goodsID=%d", userID, goodsID)
		return &SeckillResult{
			Code:    1,
			Message: "商品已售罄",
		}, nil
	case 1:
		// Redis 扣减成功，异步发送到 Kafka
		return s.createOrderAsync(ctx, userID, goodsID)
	default:
		return &SeckillResult{
			Code:    3,
			Message: "系统异常",
		}, fmt.Errorf("未知的秒杀结果: %d", result)
	}
}

// createOrderAsync 异步创建订单（发送到 Kafka）
func (s *SeckillService) createOrderAsync(ctx context.Context, userID string, goodsID int64) (*SeckillResult, error) {
	orderID := generateOrderID()

	// 发送订单消息到 Kafka（带上秒杀时间，毫秒精度）
	msg := &kafka.OrderMessage{
		Type:      kafka.MsgTypeCreateOrder,
		OrderID:   orderID,
		UserID:    userID,
		GoodsID:   goodsID,
		CreatedAt: time.Now().UnixMilli(), // 秒杀时间（毫秒）
	}

	if err := kafka.SendOrderMessage(ctx, msg); err != nil {
		// Kafka 发送失败，回滚 Redis
		log.Printf("Kafka 发送失败，回滚 Redis: userID=%s, goodsID=%d, error=%v", userID, goodsID, err)
		if rollbackErr := redis.RollbackSeckill(ctx, goodsID, userID); rollbackErr != nil {
			log.Printf("严重错误：Redis 回滚失败: userID=%s, goodsID=%d, error=%v", userID, goodsID, rollbackErr)
		}
		return &SeckillResult{
			Code:    3,
			Message: "系统繁忙，请稍后重试",
		}, err
	}

	log.Printf("秒杀成功（异步）: userID=%s, goodsID=%d, orderID=%s", userID, goodsID, orderID)

	// 将订单缓存到 Redis（立即可查）
	orderCache := &redis.OrderCache{
		OrderID:   orderID,
		UserID:    userID,
		GoodsID:   goodsID,
		Status:    0, // 待支付
		CreatedAt: msg.CreatedAt,
	}
	if err := redis.SetOrderCache(ctx, orderCache); err != nil {
		log.Printf("警告：缓存订单失败: orderID=%s, error=%v", orderID, err)
	}

	// 将订单添加到延迟队列，用于超时自动取消
	if err := redis.AddToDelayQueue(ctx, orderID); err != nil {
		log.Printf("警告：添加延迟队列失败: orderID=%s, error=%v", orderID, err)
	}

	return &SeckillResult{
		Code:    0,
		Message: "秒杀成功，订单处理中",
		OrderID: orderID,
	}, nil
}

// generateOrderID 生成唯一订单号
func generateOrderID() string {
	return uuid.New().String()
}
