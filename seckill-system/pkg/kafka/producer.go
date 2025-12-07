package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

var Writer *kafka.Writer

// 消息类型常量
const (
	MsgTypeCreateOrder = "create" // 创建订单
	MsgTypeUpdateOrder = "update" // 更新订单状态
)

// OrderMessage 订单消息结构
type OrderMessage struct {
	Type      string `json:"type"` // 消息类型：create/update
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"` // 用户名
	GoodsID   int64  `json:"goods_id"`
	Status    int8   `json:"status"`     // 订单状态：0-待支付, 1-已支付, 2-已取消
	CreatedAt int64  `json:"created_at"` // 秒杀时间（毫秒时间戳）
}

// InitProducer 初始化 Kafka 生产者
// 高并发优化：使用批量发送 + 异步模式
func InitProducer(brokers []string, topic string) {
	Writer = &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    100,                  // 批量发送，提升吞吐量
		BatchTimeout: 5 * time.Millisecond, // 最多等待 5ms 凑批
		Async:        true,                 // 异步模式，不阻塞主流程
		RequiredAcks: kafka.RequireOne,     // 只需 Leader 确认，降低延迟
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

// SendOrderStatusUpdate 发送订单状态更新消息
func SendOrderStatusUpdate(ctx context.Context, orderID string, userID string, goodsID int64, status int8) error {
	msg := &OrderMessage{
		Type:    MsgTypeUpdateOrder,
		OrderID: orderID,
		UserID:  userID,
		GoodsID: goodsID,
		Status:  status,
	}
	return SendOrderMessage(ctx, msg)
}

// Close 关闭生产者
func Close() error {
	if Writer != nil {
		return Writer.Close()
	}
	return nil
}
