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
	payTime := time.Now()
	if err := s.orderRepo.Pay(orderID, payTime); err != nil {
		return err
	}
	// 更新 Redis 状态为已支付
	_ = redis.SetUserStatus(ctx, order.GoodsID, userID, 1)
	// 从超时队列移除（支付成功，不需要超时取消）
	_ = redis.RemoveOrderTimeout(ctx, orderID)
	_ = redis.MarkAdminOrderPaid(ctx, order.GoodsID, order.Quantity, order.PayAmount, payTime)
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

	quantity := order.Quantity
	if quantity <= 0 {
		quantity = 1
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
	_ = s.goodsRepo.IncrStockBatch(order.GoodsID, quantity)

	// 3. 返还 Redis 库存
	_ = redis.IncrStockBy(ctx, order.GoodsID, quantity)

	// 4. 清除用户购买记录 (减少已购数量)
	_ = redis.ClearUserBought(ctx, order.GoodsID, userID, quantity)

	// 5. 清除已扣库存标记
	_ = redis.ClearUserDeducted(ctx, order.GoodsID, userID)

	// 6. 清除已处理标记 (减少已处理数量)
	_ = redis.ClearProcessed(ctx, order.GoodsID, userID, quantity)
	_ = redis.MarkAdminOrderCancelled(ctx, order.GoodsID, quantity)

	return nil
}

var ErrOrderStatusInvalid = errors.New("order status invalid")
