package dto

// SeckillResponse 秒杀响应
type SeckillResponse struct {
	OrderID uint64 `json:"order_id,omitempty"`
	Message string `json:"message"`
}

type ActivityResponse struct {
	ID               uint64  `json:"id"`
	GoodsID          uint64  `json:"goods_id"`
	Title            string  `json:"title"`
	StartTime        string  `json:"start_time"`
	EndTime          string  `json:"end_time"`
	Status           uint8   `json:"status"`
	MaxBuyLimit      uint    `json:"max_buy_limit"`
	WarmupStatus     uint8   `json:"warmup_status"`
	IsDefault        uint8   `json:"is_default"`
	GoodsName        string  `json:"goods_name"`
	GoodsDescription string  `json:"goods_description"`
	GoodsPrice       float64 `json:"goods_price"`
	GoodsStatus      uint8   `json:"goods_status"`
	GoodsStock       uint    `json:"goods_stock"`
}
