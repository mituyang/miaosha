package dto

// SeckillRequest 秒杀请求参数
type SeckillRequest struct {
	ActivityID uint64 `json:"activity_id"`
	GoodsID    uint64 `json:"goods_id"`
	Quantity   int    `json:"quantity" binding:"required,min=1"` // 购买数量
}

// SeckillMessage MQ 消息体
type SeckillMessage struct {
	UserID      uint64 `json:"user_id"`
	GoodsID     uint64 `json:"goods_id"`
	ActivityID  uint64 `json:"activity_id"`
	SegmentID   int    `json:"segment_id"`   // Redis 库存分段ID
	Quantity    int    `json:"quantity"`     // 购买数量
	RequestTime int64  `json:"request_time"` // 用户请求时间戳(毫秒)
	CreateTime  int64  `json:"create_time"`  // Redis确认时间戳(毫秒)
	// BornTime 改用 kafka.Message.Time 字段，在发送时由生产者设置
}

// OrderTimeoutMessage 订单超时消息
type OrderTimeoutMessage struct {
	OrderID    uint64 `json:"order_id"`
	UserID     uint64 `json:"user_id"`
	GoodsID    uint64 `json:"goods_id"`
	ActivityID uint64 `json:"activity_id"`
	SegmentID  int    `json:"segment_id"` // Redis 库存分段ID
	Quantity   int    `json:"quantity"`   // 购买数量
}
