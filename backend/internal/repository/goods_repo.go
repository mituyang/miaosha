package repository

import (
	"strings"

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

type GoodsFilter struct {
	Keyword string
	Status  *uint8
}

// List 查询商品列表
func (r *GoodsRepository) List(filter GoodsFilter) ([]model.Goods, error) {
	goods, _, err := r.ListPage(filter, 1, 1000)
	return goods, err
}

// ListPage 分页查询商品列表
func (r *GoodsRepository) ListPage(filter GoodsFilter, page, pageSize int) ([]model.Goods, int64, error) {
	var goods []model.Goods
	query := r.db.Model(&model.Goods{})

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		query = query.Where("product_name LIKE ?", "%"+keyword+"%")
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&goods).Error
	return goods, total, err
}

// ListOnSale 查询上架商品
func (r *GoodsRepository) ListOnSale() ([]model.Goods, error) {
	return r.List(GoodsFilter{Status: uint8Ptr(model.GoodsStatusOnSale)})
}

// Create 创建商品
func (r *GoodsRepository) Create(goods *model.Goods) error {
	return r.db.Create(goods).Error
}

// Update 更新商品
func (r *GoodsRepository) Update(goods *model.Goods) error {
	return r.db.Model(&model.Goods{}).Where("id = ?", goods.ID).Updates(map[string]interface{}{
		"product_name": goods.ProductName,
		"description":  goods.Description,
		"stock":        goods.Stock,
		"price":        goods.Price,
		"status":       goods.Status,
	}).Error
}

// Delete 删除商品
func (r *GoodsRepository) Delete(id uint64) error {
	return r.db.Delete(&model.Goods{}, id).Error
}

// HasOrders 判断商品是否已有关联订单
func (r *GoodsRepository) HasOrders(id uint64) (bool, error) {
	var count int64
	if err := r.db.Table("orders").Where("goods_id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type GoodsStats struct {
	TotalGoods  int64 `json:"total_goods"`
	OnSaleGoods int64 `json:"on_sale_goods"`
	TotalStock  int64 `json:"total_stock"`
}

// GetStats 查询商品统计
func (r *GoodsRepository) GetStats() (*GoodsStats, error) {
	var stats GoodsStats
	err := r.db.Raw(`
		SELECT
			COUNT(*) AS total_goods,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS on_sale_goods,
			COALESCE(SUM(stock), 0) AS total_stock
		FROM goods
	`, model.GoodsStatusOnSale).Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
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

// IncrStockBatch 批量增加库存 (订单取消时返还)
func (r *GoodsRepository) IncrStockBatch(id uint64, count int) error {
	return r.db.Model(&model.Goods{}).
		Where("id = ?", id).
		Update("stock", gorm.Expr("stock + ?", count)).Error
}

func uint8Ptr(v uint8) *uint8 {
	return &v
}
