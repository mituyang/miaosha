package dto

// SeckillRequest 秒杀请求参数
type SeckillRequest struct {
	GoodsID uint64 `json:"goods_id" binding:"required"`
}

// SeckillMessage MQ 消息体
type SeckillMessage struct {
	UserID      uint64 `json:"user_id"`
	GoodsID     uint64 `json:"goods_id"`
	RequestTime int64  `json:"request_time"` // 用户请求时间戳(毫秒)
}

// OrderTimeoutMessage 订单超时消息
type OrderTimeoutMessage struct {
	OrderID uint64 `json:"order_id"`
	UserID  uint64 `json:"user_id"`
	GoodsID uint64 `json:"goods_id"`
}
