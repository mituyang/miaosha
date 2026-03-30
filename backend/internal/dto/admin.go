package dto

import "seckill/internal/repository"

type AdminGoodsUpsertRequest struct {
	ProductName string  `json:"product_name" binding:"required,min=1,max=255"`
	Description string  `json:"description" binding:"max=500"`
	Stock       int     `json:"stock" binding:"gte=0"`
	Price       float64 `json:"price" binding:"gte=0"`
	Status      uint8   `json:"status" binding:"oneof=0 1"`
}

type AdminUserStatusRequest struct {
	Status uint8 `json:"status" binding:"oneof=0 1"`
}

type AdminStatsResponse struct {
	OrderStats   *repository.OrderStats         `json:"order_stats"`
	UserStats    *repository.UserStats          `json:"user_stats"`
	GoodsStats   *repository.GoodsStats         `json:"goods_stats"`
	SalesRanking []repository.GoodsSalesRanking `json:"sales_ranking"`
}

type PageResponse struct {
	Items    interface{} `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
