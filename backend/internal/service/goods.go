package service

import (
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/database"
)

type GoodsService struct {
	goodsRepo *repository.GoodsRepository
}

func NewGoodsService() *GoodsService {
	return &GoodsService{
		goodsRepo: repository.NewGoodsRepository(database.DB),
	}
}

// ListOnSaleGoods 查询上架商品
func (s *GoodsService) ListOnSaleGoods() ([]model.Goods, error) {
	return s.goodsRepo.ListOnSale()
}
