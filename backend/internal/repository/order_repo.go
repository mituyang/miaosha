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
	GoodsName     string `json:"goods_name"`
	ActivityTitle string `json:"activity_title"`
}

type AdminOrderItem struct {
	model.Order
	GoodsName     string `json:"goods_name"`
	ActivityTitle string `json:"activity_title"`
	Username      string `json:"username"`
}

type OrderFilter struct {
	Keyword string
	Status  *uint8
}

// AfterFind 查询后将 ID 转为字符串
func (o *OrderWithGoods) AfterFind(tx *gorm.DB) error {
	return o.Order.AfterFind(tx)
}

// AfterFind 查询后将 ID 转为字符串
func (o *AdminOrderItem) AfterFind(tx *gorm.DB) error {
	return o.Order.AfterFind(tx)
}

// FindByUserID 查询用户订单列表
func (r *OrderRepository) FindByUserID(userID uint64) ([]OrderWithGoods, error) {
	var orders []OrderWithGoods
	err := r.db.Table("orders").
		Select("orders.*, goods.product_name AS goods_name, seckill_activities.title AS activity_title").
		Joins("LEFT JOIN goods ON orders.goods_id = goods.id").
		Joins("LEFT JOIN seckill_activities ON orders.activity_id = seckill_activities.id").
		Where("orders.user_id = ?", userID).
		Order("orders.create_time DESC").
		Find(&orders).Error

	// 手动调用 AfterFind 设置 IDStr
	for i := range orders {
		orders[i].AfterFind(nil)
	}
	return orders, err
}

// List 查询订单列表
func (r *OrderRepository) List(filter OrderFilter) ([]AdminOrderItem, error) {
	orders, _, err := r.ListPage(filter, 1, 1000)
	return orders, err
}

// ListPage 分页查询订单列表
func (r *OrderRepository) ListPage(filter OrderFilter, page, pageSize int) ([]AdminOrderItem, int64, error) {
	var orders []AdminOrderItem
	query := r.db.Table("orders").
		Joins("LEFT JOIN goods ON orders.goods_id = goods.id").
		Joins("LEFT JOIN seckill_activities ON orders.activity_id = seckill_activities.id").
		Joins("LEFT JOIN users ON orders.user_id = users.id")

	if filter.Status != nil {
		query = query.Where("orders.status = ?", *filter.Status)
	}
	if filter.Keyword != "" {
		likeKeyword := "%" + filter.Keyword + "%"
		query = query.Where(
			"CAST(orders.id AS CHAR) LIKE ? OR CAST(orders.user_id AS CHAR) LIKE ? OR goods.product_name LIKE ? OR seckill_activities.title LIKE ? OR users.username LIKE ?",
			likeKeyword, likeKeyword, likeKeyword, likeKeyword, likeKeyword,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Select("orders.*, goods.product_name AS goods_name, seckill_activities.title AS activity_title, users.username AS username").
		Order("orders.id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&orders).Error
	for i := range orders {
		orders[i].AfterFind(nil)
	}
	return orders, total, err
}

// GetDetail 查询订单详情
func (r *OrderRepository) GetDetail(orderID uint64) (*AdminOrderItem, error) {
	var order AdminOrderItem
	err := r.db.Table("orders").
		Select("orders.*, goods.product_name AS goods_name, seckill_activities.title AS activity_title, users.username AS username").
		Joins("LEFT JOIN goods ON orders.goods_id = goods.id").
		Joins("LEFT JOIN seckill_activities ON orders.activity_id = seckill_activities.id").
		Joins("LEFT JOIN users ON orders.user_id = users.id").
		Where("orders.id = ?", orderID).
		First(&order).Error
	if err != nil {
		return nil, err
	}
	order.AfterFind(nil)
	return &order, nil
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

	// 先查询哪些订单是待支付状态（可以被取消）
	var unpaidOrders []model.Order
	err := r.db.Select("id").Where("id IN ? AND status = ?", orderIDs, model.OrderStatusUnpaid).Find(&unpaidOrders).Error
	if err != nil {
		return nil, err
	}

	if len(unpaidOrders) == 0 {
		return nil, nil
	}

	// 提取待取消的订单ID
	unpaidIDs := make([]uint64, len(unpaidOrders))
	for i, o := range unpaidOrders {
		unpaidIDs[i] = o.ID
	}

	// 批量更新这些订单状态
	result := r.db.Model(&model.Order{}).
		Where("id IN ? AND status = ?", unpaidIDs, model.OrderStatusUnpaid).
		Updates(map[string]interface{}{
			"status":      model.OrderStatusCancelled,
			"cancel_time": cancelTime,
		})

	if result.Error != nil {
		return nil, result.Error
	}

	// 返回实际取消的订单ID（即之前查到的待支付订单）
	return unpaidIDs, nil
}

type OrderStats struct {
	TotalOrders      int64   `json:"total_orders"`
	PaidOrders       int64   `json:"paid_orders"`
	UnpaidOrders     int64   `json:"unpaid_orders"`
	CancelledOrders  int64   `json:"cancelled_orders"`
	TotalSales       float64 `json:"total_sales"`
	TodayPaidOrders  int64   `json:"today_paid_orders"`
	TodaySalesAmount float64 `json:"today_sales_amount"`
}

type GoodsSalesRanking struct {
	GoodsID      uint64  `json:"goods_id"`
	GoodsName    string  `json:"goods_name"`
	SoldQuantity int64   `json:"sold_quantity"`
	OrderCount   int64   `json:"order_count"`
	SalesAmount  float64 `json:"sales_amount"`
}

// GetStats 查询订单统计
func (r *OrderRepository) GetStats() (*OrderStats, error) {
	var stats OrderStats
	err := r.db.Raw(`
		SELECT
			COUNT(*) AS total_orders,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS paid_orders,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS unpaid_orders,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS cancelled_orders,
			COALESCE(SUM(CASE WHEN status = ? THEN pay_amount ELSE 0 END), 0) AS total_sales,
			COALESCE(SUM(CASE WHEN status = ? AND DATE(pay_time) = CURDATE() THEN 1 ELSE 0 END), 0) AS today_paid_orders,
			COALESCE(SUM(CASE WHEN status = ? AND DATE(pay_time) = CURDATE() THEN pay_amount ELSE 0 END), 0) AS today_sales_amount
		FROM orders
	`,
		model.OrderStatusPaid,
		model.OrderStatusUnpaid,
		model.OrderStatusCancelled,
		model.OrderStatusPaid,
		model.OrderStatusPaid,
		model.OrderStatusPaid,
	).Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// GetSalesRanking 查询商品销量排行
func (r *OrderRepository) GetSalesRanking(limit int) ([]GoodsSalesRanking, error) {
	var ranking []GoodsSalesRanking
	query := `
		SELECT
			orders.goods_id AS goods_id,
			COALESCE(goods.product_name, '已删除商品') AS goods_name,
			COALESCE(SUM(CASE WHEN orders.status = ? THEN orders.quantity ELSE 0 END), 0) AS sold_quantity,
			COALESCE(SUM(CASE WHEN orders.status = ? THEN 1 ELSE 0 END), 0) AS order_count,
			COALESCE(SUM(CASE WHEN orders.status = ? THEN orders.pay_amount ELSE 0 END), 0) AS sales_amount
		FROM orders
		LEFT JOIN goods ON orders.goods_id = goods.id
		GROUP BY orders.goods_id, goods.product_name
		HAVING sold_quantity > 0 OR order_count > 0
		ORDER BY sold_quantity DESC, sales_amount DESC
	`

	args := []interface{}{
		model.OrderStatusPaid,
		model.OrderStatusPaid,
		model.OrderStatusPaid,
	}
	if limit > 0 {
		query += "\n\t\tLIMIT ?"
		args = append(args, limit)
	}

	err := r.db.Raw(query, args...).Scan(&ranking).Error

	return ranking, err
}
