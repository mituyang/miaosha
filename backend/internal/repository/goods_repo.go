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

// DecrStockBatch 批量扣减库存
// 返回实际扣减的数量
func (r *GoodsRepository) DecrStockBatch(id uint64, count int) (int64, error) {
	// 先查询当前库存
	var goods model.Goods
	if err := r.db.First(&goods, id).Error; err != nil {
		return 0, err
	}

	// 计算实际可扣减数量
	actualCount := count
	if int(goods.Stock) < count {
		actualCount = int(goods.Stock)
	}

	if actualCount == 0 {
		return 0, nil
	}

	// 扣减库存
	result := r.db.Model(&model.Goods{}).
		Where("id = ? AND stock >= ?", id, actualCount).
		Update("stock", gorm.Expr("stock - ?", actualCount))

	if result.Error != nil {
		return 0, result.Error
	}

	if result.RowsAffected == 0 {
		return 0, nil
	}

	return int64(actualCount), nil
}

// DecrStockSimple 简单扣减库存 (不用乐观锁)
func (r *GoodsRepository) DecrStockSimple(id uint64) (int64, error) {
	result := r.db.Model(&model.Goods{}).
		Where("id = ? AND stock > 0", id).
		Update("stock", gorm.Expr("stock - 1"))
	return result.RowsAffected, result.Error
}

// DecrStockWithTx 在事务中扣减库存
func (r *GoodsRepository) DecrStockWithTx(tx *gorm.DB, id uint64) (int64, error) {
	result := tx.Model(&model.Goods{}).
		Where("id = ? AND stock > 0", id).
		Update("stock", gorm.Expr("stock - 1"))
	return result.RowsAffected, result.Error
}

// DecrStockBatchWithTx 在事务中批量扣减库存
// 返回实际扣减的数量
func (r *GoodsRepository) DecrStockBatchWithTx(tx *gorm.DB, id uint64, count int) (int64, error) {
	// 先查询当前库存（加行锁）
	var goods model.Goods
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&goods, id).Error; err != nil {
		return 0, err
	}

	// 计算实际可扣减数量
	actualCount := min(count, int(goods.Stock))
	if actualCount == 0 {
		return 0, nil
	}

	// 扣减库存
	result := tx.Model(&model.Goods{}).
		Where("id = ?", id).
		Update("stock", gorm.Expr("stock - ?", actualCount))

	if result.Error != nil {
		return 0, result.Error
	}

	return int64(actualCount), nil
}
