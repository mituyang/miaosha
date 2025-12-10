package service

import (
	"context"
	"errors"

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
func (s *OrderService) PayOrder(orderID, userID uint64) error {
	order, err := s.orderRepo.FindByIDAndUserID(orderID, userID)
	if err != nil {
		return err
	}
	if order.Status != 0 {
		return ErrOrderStatusInvalid
	}
	return s.orderRepo.Pay(orderID)
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
	if err := s.orderRepo.UpdateStatus(orderID, 2); err != nil {
		return err
	}

	// 2. 返还 MySQL 库存
	_ = s.goodsRepo.IncrStock(order.GoodsID)

	// 3. 返还 Redis 库存
	_ = redis.IncrStock(ctx, order.GoodsID)

	// 4. 清除用户购买记录 (允许重新抢购)
	_ = redis.ClearUserBought(ctx, order.GoodsID, userID)

	return nil
}

var ErrOrderStatusInvalid = errors.New("order status invalid")
