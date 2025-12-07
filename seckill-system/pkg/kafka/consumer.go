package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"

	"seckill-system/internal/model"
	"seckill-system/pkg/redis"
)

// Consumer Kafka 消费者
type Consumer struct {
	reader *kafka.Reader
	db     *gorm.DB
}

// NewConsumer 创建消费者实例
func NewConsumer(brokers []string, topic, groupID string, db *gorm.DB) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	return &Consumer{
		reader: reader,
		db:     db,
	}
}

// Start 启动消费者，持续消费消息
func (c *Consumer) Start(ctx context.Context) {
	log.Println("订单消费者启动...")

	for {
		select {
		case <-ctx.Done():
			log.Println("消费者收到停止信号，正在退出...")
			return
		default:
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				log.Printf("读取消息失败: %v", err)
				continue
			}

			if err := c.processMessage(msg.Value); err != nil {
				log.Printf("处理消息失败: %v", err)
				// 实际生产中应该有重试机制或死信队列
			}
		}
	}
}

// processMessage 处理订单消息，根据消息类型分发处理
func (c *Consumer) processMessage(data []byte) error {
	var orderMsg OrderMessage
	if err := json.Unmarshal(data, &orderMsg); err != nil {
		return fmt.Errorf("反序列化消息失败: %w", err)
	}

	// 根据消息类型分发处理
	switch orderMsg.Type {
	case MsgTypeUpdateOrder:
		return c.processUpdateOrder(&orderMsg)
	default:
		// 默认为创建订单（兼容旧消息）
		return c.processCreateOrder(&orderMsg)
	}
}

// processCreateOrder 处理创建订单消息
func (c *Consumer) processCreateOrder(orderMsg *OrderMessage) error {
	ctx := context.Background()

	// 先检查 Redis 缓存中订单状态
	orderCache, _ := redis.GetOrderCache(ctx, orderMsg.OrderID)

	return c.db.Transaction(func(tx *gorm.DB) error {
		// 1. 检查订单是否已存在
		var existingOrder model.SeckillOrder
		err := tx.Where("order_id = ?", orderMsg.OrderID).First(&existingOrder).Error
		if err == nil {
			log.Printf("订单 %s 已存在，跳过创建", orderMsg.OrderID)
			return nil
		}

		// 2. 再次检查 Redis 缓存状态（双重检查）
		orderCache, _ = redis.GetOrderCache(ctx, orderMsg.OrderID)

		// 3. 根据 Redis 缓存状态决定订单状态
		orderStatus := int8(0)
		if orderCache != nil {
			orderStatus = orderCache.Status
		}

		// 4. 只有非取消状态的订单才扣减 MySQL 库存
		if orderStatus != 2 {
			result := tx.Model(&model.SeckillGoods{}).
				Where("id = ? AND stock > 0", orderMsg.GoodsID).
				Update("stock", gorm.Expr("stock - 1"))

			if result.RowsAffected == 0 {
				return fmt.Errorf("商品 %d 库存不足或不存在", orderMsg.GoodsID)
			}
		}

		// 5. 创建订单
		createdAt := time.Now()
		if orderMsg.CreatedAt > 0 {
			createdAt = time.UnixMilli(orderMsg.CreatedAt)
		}
		order := &model.SeckillOrder{
			OrderID:   orderMsg.OrderID,
			UserID:    orderMsg.UserID,
			GoodsID:   orderMsg.GoodsID,
			Status:    orderStatus,
			CreatedAt: createdAt,
		}
		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("创建订单失败: %w", err)
		}

		log.Printf("订单 %s 创建成功（用户: %s, 商品: %d, 状态: %d）", orderMsg.OrderID, orderMsg.UserID, orderMsg.GoodsID, orderStatus)
		return nil
	})
}

// processUpdateOrder 处理订单状态更新消息
func (c *Consumer) processUpdateOrder(orderMsg *OrderMessage) error {
	return c.db.Transaction(func(tx *gorm.DB) error {
		// 1. 查询订单是否存在
		var order model.SeckillOrder
		err := tx.Where("order_id = ?", orderMsg.OrderID).First(&order).Error

		if err != nil {
			// 订单不存在，可能还没落库，直接返回（Redis 缓存已更新）
			log.Printf("订单 %s 不存在，跳过状态更新", orderMsg.OrderID)
			return nil
		}

		// 2. 检查状态是否需要更新
		if order.Status == orderMsg.Status {
			log.Printf("订单 %s 状态已是 %d，跳过更新", orderMsg.OrderID, orderMsg.Status)
			return nil
		}

		// 3. 如果是取消订单（从待支付变为已取消），需要恢复库存
		if order.Status == 0 && orderMsg.Status == 2 {
			tx.Model(&model.SeckillGoods{}).Where("id = ?", order.GoodsID).
				Update("stock", gorm.Expr("stock + 1"))
			log.Printf("订单 %s 取消，恢复商品 %d 库存", orderMsg.OrderID, order.GoodsID)
		}

		// 4. 更新订单状态
		if err := tx.Model(&order).Update("status", orderMsg.Status).Error; err != nil {
			return fmt.Errorf("更新订单状态失败: %w", err)
		}

		log.Printf("订单 %s 状态更新为 %d", orderMsg.OrderID, orderMsg.Status)
		return nil
	})
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	return c.reader.Close()
}
