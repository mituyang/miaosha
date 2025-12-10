package dto

// SeckillResponse 秒杀响应
type SeckillResponse struct {
	OrderID uint64 `json:"order_id,omitempty"`
	Message string `json:"message"`
}
