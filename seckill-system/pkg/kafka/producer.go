package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

var Writer *kafka.Writer

// OrderMessage 订单消息结构
type OrderMessage struct {
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"` // 用户名
	GoodsID int64  `json:"goods_id"`
}

// InitProducer 初始化 Kafka 生产者
// 注意：这里使用同步模式，确保消息发送成功后才返回
func InitProducer(brokers []string, topic string) {
	Writer = &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    1, // 单条发送，保证实时性
		BatchTimeout: 10 * time.Millisecond,
		Async:        false,            // 同步模式！确保发送成功才返回
		RequiredAcks: kafka.RequireAll, // 要求所有副本确认，保证消息不丢失
	}
}

// SendOrderMessage 同步发送订单消息到 Kafka
// 返回 error 为 nil 时，表示消息已成功写入 Kafka 并被确认
func SendOrderMessage(ctx context.Context, msg *OrderMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化订单消息失败: %w", err)
	}

	// 设置发送超时，防止无限等待
	sendCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	err = Writer.WriteMessages(sendCtx, kafka.Message{
		Key:   []byte(msg.UserID), // 用户名作为 key
		Value: data,
	})
	if err != nil {
		return fmt.Errorf("发送 Kafka 消息失败: %w", err)
	}
	return nil
}

// Close 关闭生产者
func Close() error {
	if Writer != nil {
		return Writer.Close()
	}
	return nil
}
