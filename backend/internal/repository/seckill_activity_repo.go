package repository

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"seckill/internal/model"
)

type SeckillActivityRepository struct {
	db *gorm.DB
}

func NewSeckillActivityRepository(db *gorm.DB) *SeckillActivityRepository {
	return &SeckillActivityRepository{db: db}
}

type SeckillActivityWithGoods struct {
	model.SeckillActivity
	GoodsName        string  `json:"goods_name"`
	GoodsDescription string  `json:"goods_description"`
	GoodsPrice       float64 `json:"goods_price"`
	GoodsStatus      uint8   `json:"goods_status"`
	GoodsStock       uint    `json:"goods_stock"`
}

type SeckillActivityFilter struct {
	Keyword string
	Status  *uint8
}

func (r *SeckillActivityRepository) GetByID(id uint64) (*model.SeckillActivity, error) {
	var activity model.SeckillActivity
	if err := r.db.First(&activity, id).Error; err != nil {
		return nil, err
	}
	return &activity, nil
}

func (r *SeckillActivityRepository) GetWithGoods(id uint64) (*SeckillActivityWithGoods, error) {
	var activity SeckillActivityWithGoods
	err := r.baseWithGoodsQuery().
		Where("seckill_activities.id = ?", id).
		First(&activity).Error
	if err != nil {
		return nil, err
	}
	return &activity, nil
}

func (r *SeckillActivityRepository) FindDefaultByGoodsID(goodsID uint64) (*model.SeckillActivity, error) {
	var activity model.SeckillActivity
	err := r.db.Where("goods_id = ? AND is_default = 1", goodsID).
		Order("id ASC").
		First(&activity).Error
	if err != nil {
		return nil, err
	}
	return &activity, nil
}

func (r *SeckillActivityRepository) ListByGoodsID(goodsID uint64) ([]model.SeckillActivity, error) {
	var activities []model.SeckillActivity
	err := r.db.Where("goods_id = ?", goodsID).Find(&activities).Error
	return activities, err
}

func (r *SeckillActivityRepository) ListPage(filter SeckillActivityFilter, page, pageSize int) ([]SeckillActivityWithGoods, int64, error) {
	var activities []SeckillActivityWithGoods
	query := r.baseWithGoodsQuery()

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("seckill_activities.title LIKE ? OR goods.product_name LIKE ?", likeKeyword, likeKeyword)
	}
	if filter.Status != nil {
		query = query.Where("seckill_activities.status = ?", *filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("seckill_activities.id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&activities).Error
	return activities, total, err
}

func (r *SeckillActivityRepository) ListPublic(now time.Time) ([]SeckillActivityWithGoods, error) {
	var activities []SeckillActivityWithGoods
	err := r.baseWithGoodsQuery().
		Where("goods.status = ?", model.GoodsStatusOnSale).
		Where("seckill_activities.status IN ?", []int{
			int(model.SeckillActivityStatusPending),
			int(model.SeckillActivityStatusRunning),
		}).
		Where("seckill_activities.end_time >= ?", now).
		Order("seckill_activities.start_time ASC, seckill_activities.id ASC").
		Find(&activities).Error
	return activities, err
}

func (r *SeckillActivityRepository) ListWarmupCandidates() ([]SeckillActivityWithGoods, error) {
	var activities []SeckillActivityWithGoods
	err := r.baseWithGoodsQuery().
		Where("seckill_activities.status IN ?", []int{
			int(model.SeckillActivityStatusPending),
			int(model.SeckillActivityStatusRunning),
		}).
		Order("seckill_activities.id ASC").
		Find(&activities).Error
	return activities, err
}

func (r *SeckillActivityRepository) Create(activity *model.SeckillActivity) error {
	return r.db.Create(activity).Error
}

func (r *SeckillActivityRepository) Update(activity *model.SeckillActivity) error {
	return r.db.Model(&model.SeckillActivity{}).
		Where("id = ?", activity.ID).
		Updates(map[string]interface{}{
			"goods_id":      activity.GoodsID,
			"title":         activity.Title,
			"start_time":    activity.StartTime,
			"end_time":      activity.EndTime,
			"status":        activity.Status,
			"max_buy_limit": activity.MaxBuyLimit,
			"warmup_status": activity.WarmupStatus,
			"is_default":    activity.IsDefault,
			"updated_at":    time.Now(),
		}).Error
}

func (r *SeckillActivityRepository) UpdateStatus(id uint64, status uint8) error {
	return r.db.Model(&model.SeckillActivity{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"warmup_status": model.SeckillActivityWarmupPending,
			"updated_at":    time.Now(),
		}).Error
}

func (r *SeckillActivityRepository) UpdateWarmupStatus(id uint64, status uint8) error {
	return r.db.Model(&model.SeckillActivity{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"warmup_status": status,
			"updated_at":    time.Now(),
		}).Error
}

func (r *SeckillActivityRepository) HasOverlappingEnabled(goodsID, excludeID uint64, startTime, endTime time.Time) (bool, error) {
	var count int64
	query := r.db.Model(&model.SeckillActivity{}).
		Where("goods_id = ?", goodsID).
		Where("status IN ?", []int{int(model.SeckillActivityStatusPending), int(model.SeckillActivityStatusRunning)}).
		Where("start_time < ? AND end_time > ?", endTime, startTime)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *SeckillActivityRepository) baseWithGoodsQuery() *gorm.DB {
	return r.db.Table("seckill_activities").
		Select(`
			seckill_activities.*,
			goods.product_name AS goods_name,
			goods.description AS goods_description,
			goods.price AS goods_price,
			goods.status AS goods_status,
			goods.stock AS goods_stock
		`).
		Joins("LEFT JOIN goods ON seckill_activities.goods_id = goods.id")
}
