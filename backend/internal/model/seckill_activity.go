package model

import "time"

const (
	SeckillActivityStatusPending  uint8 = 0
	SeckillActivityStatusRunning  uint8 = 1
	SeckillActivityStatusEnded    uint8 = 2
	SeckillActivityStatusDisabled uint8 = 3
)

const (
	SeckillActivityWarmupPending uint8 = 0
	SeckillActivityWarmupDone    uint8 = 1
)

// SeckillActivity 秒杀活动模型
type SeckillActivity struct {
	ID           uint64    `gorm:"primaryKey"`
	GoodsID      uint64    `gorm:"not null;index:idx_activity_goods_id;index:idx_activity_goods_default,priority:1"`
	Title        string    `gorm:"type:varchar(255);not null"`
	StartTime    time.Time `gorm:"not null;index:idx_activity_status_time,priority:2"`
	EndTime      time.Time `gorm:"not null;index:idx_activity_status_time,priority:3"`
	Status       uint8     `gorm:"not null;default:0;index:idx_activity_status_time,priority:1"`
	MaxBuyLimit  uint      `gorm:"not null"`
	WarmupStatus uint8     `gorm:"not null;default:0"`
	IsDefault    uint8     `gorm:"not null;default:0;index:idx_activity_goods_default,priority:2"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName 表名
func (SeckillActivity) TableName() string {
	return "seckill_activities"
}
