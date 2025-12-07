package model

import (
	"time"
)

// SeckillGoods 秒杀商品表
type SeckillGoods struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	GoodsName string    `gorm:"type:varchar(255);not null" json:"goods_name"`
	Stock     int       `gorm:"not null;default:0" json:"stock"`   // 库存数量
	StartTime time.Time `gorm:"not null" json:"start_time"`        // 秒杀开始时间
	EndTime   time.Time `gorm:"not null" json:"end_time"`          // 秒杀结束时间
	Version   int       `gorm:"not null;default:0" json:"version"` // 乐观锁版本号
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SeckillOrder 秒杀订单表
type SeckillOrder struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	OrderID   string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"order_id"` // 订单号
	UserID    string    `gorm:"type:varchar(50);index;not null" json:"user_id"`        // 用户名
	GoodsID   int64     `gorm:"index;not null" json:"goods_id"`
	Status    int8      `gorm:"not null;default:0" json:"status"` // 0-待支付, 1-已支付, 2-已取消
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (SeckillGoods) TableName() string {
	return "seckill_goods"
}

func (SeckillOrder) TableName() string {
	return "seckill_order"
}

// User 用户表
type User struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Username  string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"type:varchar(255);not null" json:"-"` // 密码不返回给前端
	Email     string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"email"`
	Nickname  string    `gorm:"type:varchar(100)" json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
