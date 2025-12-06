package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"

	"seckill-system/internal/model"
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

// processMessage 处理订单消息，写入数据库
func (c *Consumer) processMessage(data []byte) error {
	var orderMsg OrderMessage
	if err := json.Unmarshal(data, &orderMsg); err != nil {
		return fmt.Errorf("反序列化消息失败: %w", err)
	}

	// 使用事务保证数据一致性
	return c.db.Transaction(func(tx *gorm.DB) error {
		// 1. 检查订单是否已存在（幂等性）
		var count int64
		tx.Model(&model.SeckillOrder{}).Where("order_id = ?", orderMsg.OrderID).Count(&count)
		if count > 0 {
			log.Printf("订单 %s 已存在，跳过处理", orderMsg.OrderID)
			return nil
		}

		// 2. 使用乐观锁扣减数据库库存
		result := tx.Model(&model.SeckillGoods{}).
			Where("id = ? AND stock > 0", orderMsg.GoodsID).
			Update("stock", gorm.Expr("stock - 1"))

		if result.RowsAffected == 0 {
			return fmt.Errorf("商品 %d 库存不足或不存在", orderMsg.GoodsID)
		}

		// 3. 创建订单
		order := &model.SeckillOrder{
			OrderID: orderMsg.OrderID,
			UserID:  orderMsg.UserID,
			GoodsID: orderMsg.GoodsID,
			Status:  0, // 待支付
		}
		if err := tx.Create(order).Error; err != nil {
			return fmt.Errorf("创建订单失败: %w", err)
		}

		log.Printf("订单 %s 创建成功", orderMsg.OrderID)
		return nil
	})
}

// Close 关闭消费者
func (c *Consumer) Close() error {
	return c.reader.Close()
}
