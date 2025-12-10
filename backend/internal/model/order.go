package model

import (
	"strconv"
	"time"
)

// Order 订单模型
type Order struct {
	ID         uint64     `gorm:"primaryKey" json:"-"`
	IDStr      string     `gorm:"-" json:"ID"` // JSON 返回字符串格式的 ID
	UserID     uint64     `gorm:"not null;index" json:"UserID"`
	GoodsID    uint64     `gorm:"not null" json:"GoodsID"`
	PayAmount  float64    `gorm:"type:decimal(10,2);not null;default:0" json:"PayAmount"`
	Status     uint8      `gorm:"not null;default:0" json:"Status"` // 0-未支付, 1-已支付, 2-已取消
	CreateTime time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"CreateTime"`
	PayTime    *time.Time `gorm:"default:null" json:"PayTime"`
}

// TableName 表名
func (Order) TableName() string {
	return "orders"
}

// AfterFind GORM hook: 查询后将 ID 转为字符串
func (o *Order) AfterFind(tx interface{}) error {
	o.IDStr = strconv.FormatUint(o.ID, 10)
	return nil
}
