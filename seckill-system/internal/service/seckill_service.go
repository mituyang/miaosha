package service

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"seckill-system/internal/model"
	"seckill-system/pkg/redis"
)

// SeckillService 秒杀业务逻辑层
type SeckillService struct {
	db *gorm.DB
}

// NewSeckillService 创建秒杀服务实例
func NewSeckillService(db *gorm.DB) *SeckillService {
	return &SeckillService{db: db}
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
		// Redis 扣减成功，同步写入数据库
		return s.createOrderSync(ctx, userID, goodsID)
	default:
		return &SeckillResult{
			Code:    3,
			Message: "系统异常",
		}, fmt.Errorf("未知的秒杀结果: %d", result)
	}
}

// createOrderSync 同步创建订单到数据库
func (s *SeckillService) createOrderSync(ctx context.Context, userID string, goodsID int64) (*SeckillResult, error) {
	orderID := generateOrderID()

	// 使用事务写入数据库
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 检查是否有有效订单（待支付或已支付）
		var count int64
		tx.Model(&model.SeckillOrder{}).Where("user_id = ? AND goods_id = ? AND status IN (0, 1)", userID, goodsID).Count(&count)
		if count > 0 {
			return fmt.Errorf("已有有效订单")
		}

		// 2. 扣减数据库库存（乐观锁）
		result := tx.Model(&model.SeckillGoods{}).
			Where("id = ? AND stock > 0", goodsID).
			Update("stock", gorm.Expr("stock - 1"))
		if result.RowsAffected == 0 {
			return fmt.Errorf("库存不足")
		}

		// 3. 创建订单
		order := &model.SeckillOrder{
			OrderID: orderID,
			UserID:  userID,
			GoodsID: goodsID,
			Status:  0, // 待支付
		}
		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("创建订单失败: %w", err)
		}

		return nil
	})

	if err != nil {
		// 数据库写入失败，回滚 Redis
		log.Printf("数据库写入失败，回滚 Redis: userID=%s, goodsID=%d, error=%v", userID, goodsID, err)
		if rollbackErr := redis.RollbackSeckill(ctx, goodsID, userID); rollbackErr != nil {
			log.Printf("严重错误：Redis 回滚失败: userID=%s, goodsID=%d, error=%v", userID, goodsID, rollbackErr)
		}
		return &SeckillResult{
			Code:    3,
			Message: "系统繁忙，请稍后重试",
		}, err
	}

	log.Printf("秒杀成功（同步）: userID=%s, goodsID=%d, orderID=%s", userID, goodsID, orderID)

	// 将订单添加到延迟队列，用于超时自动取消
	if err := redis.AddToDelayQueue(ctx, orderID); err != nil {
		log.Printf("警告：添加延迟队列失败: orderID=%s, error=%v", orderID, err)
		// 不影响主流程，继续返回成功
	}

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
