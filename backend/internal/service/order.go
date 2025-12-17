package service

import (
	"context"
	"errors"
	"time"

	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/redis"
)

type OrderService struct {
	orderRepo *repository.OrderRepository
	goodsRepo *repository.GoodsRepository
}

func NewOrderService() *OrderService {
	return &OrderService{
		orderRepo: repository.NewOrderRepository(database.DB),
		goodsRepo: repository.NewGoodsRepository(database.DB),
	}
}

// GetUserOrders 获取用户订单列表
func (s *OrderService) GetUserOrders(userID uint64) ([]repository.OrderWithGoods, error) {
	return s.orderRepo.FindByUserID(userID)
}

// PayOrder 支付订单
func (s *OrderService) PayOrder(ctx context.Context, orderID, userID uint64) error {
	order, err := s.orderRepo.FindByIDAndUserID(orderID, userID)
	if err != nil {
		return err
	}
	if order.Status != 0 {
		return ErrOrderStatusInvalid
	}
	if err := s.orderRepo.Pay(orderID, time.Now()); err != nil {
		return err
	}
	// 更新 Redis 状态为已支付
	_ = redis.SetUserStatus(ctx, order.GoodsID, userID, 1)
	// 从超时队列移除（支付成功，不需要超时取消）
	_ = redis.RemoveOrderTimeout(ctx, orderID)
	return nil
}

// CancelOrder 取消订单并返还库存
func (s *OrderService) CancelOrder(ctx context.Context, orderID, userID uint64) error {
	order, err := s.orderRepo.FindByIDAndUserID(orderID, userID)
	if err != nil {
		return err
	}
	if order.Status != 0 {
		return ErrOrderStatusInvalid
	}

	// 1. 取消订单
	affected, err := s.orderRepo.CancelOrder(orderID, time.Now())
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOrderStatusInvalid
	}

	// 2. 返还 MySQL 库存
	_ = s.goodsRepo.IncrStock(order.GoodsID)

	// 3. 返还 Redis 库存
	_ = redis.IncrStock(ctx, order.GoodsID)

	// 4. 清除用户购买记录 (允许重新抢购)
	_ = redis.ClearUserBought(ctx, order.GoodsID, userID)

	// 5. 清除已扣库存标记
	_ = redis.ClearUserDeducted(ctx, order.GoodsID, userID)

	// 6. 清除已处理标记 (允许重新抢购)
	_ = redis.ClearProcessed(ctx, order.GoodsID, userID)

	return nil
}

var ErrOrderStatusInvalid = errors.New("order status invalid")
