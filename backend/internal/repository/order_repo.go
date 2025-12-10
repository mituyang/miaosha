package repository

import (
	"gorm.io/gorm"

	"seckill/internal/model"
)

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// Create 创建订单
func (r *OrderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

// ExistsByUserAndGoods 检查用户是否有该商品的有效订单（未取消）
func (r *OrderRepository) ExistsByUserAndGoods(userID, goodsID uint64) (bool, error) {
	var count int64
	err := r.db.Model(&model.Order{}).
		Where("user_id = ? AND goods_id = ? AND status != 2", userID, goodsID).
		Count(&count).Error
	return count > 0, err
}

// OrderWithGoods 订单带商品信息
type OrderWithGoods struct {
	model.Order
	GoodsName string `json:"goods_name"`
}

// AfterFind 查询后将 ID 转为字符串
func (o *OrderWithGoods) AfterFind(tx interface{}) error {
	return o.Order.AfterFind(tx)
}

// FindByUserID 查询用户订单列表
func (r *OrderRepository) FindByUserID(userID uint64) ([]OrderWithGoods, error) {
	var orders []OrderWithGoods
	err := r.db.Table("orders").
		Select("orders.*, goods.product_name as goods_name").
		Joins("LEFT JOIN goods ON orders.goods_id = goods.id").
		Where("orders.user_id = ?", userID).
		Order("orders.create_time DESC").
		Find(&orders).Error

	// 手动调用 AfterFind 设置 IDStr
	for i := range orders {
		orders[i].AfterFind(nil)
	}
	return orders, err
}

// FindByIDAndUserID 根据订单ID和用户ID查询订单
func (r *OrderRepository) FindByIDAndUserID(orderID, userID uint64) (*model.Order, error) {
	var order model.Order
	err := r.db.Where("id = ? AND user_id = ?", orderID, userID).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// UpdateStatus 更新订单状态
func (r *OrderRepository) UpdateStatus(orderID uint64, status uint8) error {
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Update("status", status).Error
}

// Pay 支付订单
func (r *OrderRepository) Pay(orderID uint64) error {
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(map[string]interface{}{
		"status":   1,
		"pay_time": r.db.NowFunc(),
	}).Error
}
