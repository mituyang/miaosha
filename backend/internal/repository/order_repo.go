package repository

import (
	"time"

	"gorm.io/gorm"

	"seckill/internal/model"
)

// 批量操作常量
const (
	DefaultBatchSize = 100 // 默认批量插入大小
)

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// GetDB 获取数据库连接（用于事务）
func (r *OrderRepository) GetDB() *gorm.DB {
	return r.db
}

// Create 创建订单
func (r *OrderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

// CreateWithTx 在事务中创建订单
func (r *OrderRepository) CreateWithTx(tx *gorm.DB, order *model.Order) error {
	return tx.Create(order).Error
}

// ExistsByUserAndGoods 检查用户是否有该商品的有效订单（未取消）
// 使用 EXISTS 替代 COUNT，找到一条就返回，性能更好
func (r *OrderRepository) ExistsByUserAndGoods(userID, goodsID uint64) (bool, error) {
	var exists bool
	err := r.db.Raw(
		"SELECT EXISTS(SELECT 1 FROM orders WHERE user_id = ? AND goods_id = ? AND status != ? LIMIT 1)",
		userID, goodsID, model.OrderStatusCancelled,
	).Scan(&exists).Error
	return exists, err
}

// OrderWithGoods 订单带商品信息
type OrderWithGoods struct {
	model.Order
	GoodsName string `json:"goods_name"`
}

// AfterFind 查询后将 ID 转为字符串
func (o *OrderWithGoods) AfterFind(tx *gorm.DB) error {
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

// CancelOrder 取消订单（CAS 操作，只有待支付状态才能取消）
// 返回影响行数，0 表示订单不存在或状态不是待支付
func (r *OrderRepository) CancelOrder(orderID uint64, cancelTime time.Time) (int64, error) {
	result := r.db.Model(&model.Order{}).
		Where("id = ? AND status = ?", orderID, model.OrderStatusUnpaid).
		Updates(map[string]interface{}{
			"status":      model.OrderStatusCancelled,
			"cancel_time": cancelTime,
		})
	return result.RowsAffected, result.Error
}

// Pay 支付订单
func (r *OrderRepository) Pay(orderID uint64, payTime time.Time) error {
	return r.db.Model(&model.Order{}).Where("id = ?", orderID).Updates(map[string]interface{}{
		"status":   model.OrderStatusPaid,
		"pay_time": payTime,
	}).Error
}

// BatchCreate 批量创建订单
func (r *OrderRepository) BatchCreate(orders []*model.Order) error {
	if len(orders) == 0 {
		return nil
	}
	return r.db.CreateInBatches(orders, DefaultBatchSize).Error
}

// BatchCreateWithTx 在事务中批量创建订单
func (r *OrderRepository) BatchCreateWithTx(tx *gorm.DB, orders []*model.Order) error {
	if len(orders) == 0 {
		return nil
	}
	return tx.CreateInBatches(orders, DefaultBatchSize).Error
}

// FindExpiredUnpaidOrders 查询超时的待支付订单（MySQL 兜底扫描）
// threshold: 写入时间阈值，早于此时间的订单视为超时
func (r *OrderRepository) FindExpiredUnpaidOrders(threshold time.Time, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Model(&model.Order{}).
		Where("status = ? AND write_time < ?", model.OrderStatusUnpaid, threshold).
		Limit(limit).
		Find(&orders).Error
	return orders, err
}

// BatchCancelOrders 批量取消订单（CAS 操作，只有待支付状态才能取消）
// 返回实际取消的订单ID列表
func (r *OrderRepository) BatchCancelOrders(orderIDs []uint64, cancelTime time.Time) ([]uint64, error) {
	if len(orderIDs) == 0 {
		return nil, nil
	}

	result := r.db.Model(&model.Order{}).
		Where("id IN ? AND status = ?", orderIDs, model.OrderStatusUnpaid).
		Updates(map[string]interface{}{
			"status":      model.OrderStatusCancelled,
			"cancel_time": cancelTime,
		})

	if result.Error != nil {
		return nil, result.Error
	}

	// 查询实际被取消的订单（只查 status=已取消 的，不用精确匹配时间）
	if result.RowsAffected == 0 {
		return nil, nil
	}

	var cancelledOrders []model.Order
	err := r.db.Where("id IN ? AND status = ?", orderIDs, model.OrderStatusCancelled).Find(&cancelledOrders).Error
	if err != nil {
		return nil, err
	}

	cancelledIDs := make([]uint64, len(cancelledOrders))
	for i, o := range cancelledOrders {
		cancelledIDs[i] = o.ID
	}
	return cancelledIDs, nil
}
