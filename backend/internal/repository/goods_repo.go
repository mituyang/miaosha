package repository

import (
	"gorm.io/gorm"

	"seckill/internal/model"
)

type GoodsRepository struct {
	db *gorm.DB
}

func NewGoodsRepository(db *gorm.DB) *GoodsRepository {
	return &GoodsRepository{db: db}
}

// GetByID 根据ID查询商品
func (r *GoodsRepository) GetByID(id uint64) (*model.Goods, error) {
	var goods model.Goods
	if err := r.db.First(&goods, id).Error; err != nil {
		return nil, err
	}
	return &goods, nil
}

// GetAll 查询所有商品
func (r *GoodsRepository) GetAll() ([]model.Goods, error) {
	var goods []model.Goods
	if err := r.db.Find(&goods).Error; err != nil {
		return nil, err
	}
	return goods, nil
}

// DecrStock 扣减库存 (乐观锁)
// 返回影响行数，0 表示扣减失败 (库存不足或版本冲突)
func (r *GoodsRepository) DecrStock(id uint64, version uint) (int64, error) {
	result := r.db.Model(&model.Goods{}).
		Where("id = ? AND version = ? AND stock > 0", id, version).
		Updates(map[string]interface{}{
			"stock":   gorm.Expr("stock - 1"),
			"version": gorm.Expr("version + 1"),
		})

	return result.RowsAffected, result.Error
}

// IncrStock 增加库存 (订单取消时返还)
func (r *GoodsRepository) IncrStock(id uint64) error {
	return r.db.Model(&model.Goods{}).
		Where("id = ?", id).
		Update("stock", gorm.Expr("stock + 1")).Error
}
