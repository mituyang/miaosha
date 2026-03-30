package model

import "time"

const (
	GoodsStatusOffShelf uint8 = 0
	GoodsStatusOnSale   uint8 = 1
)

// Goods 商品模型
type Goods struct {
	ID          uint64    `gorm:"primaryKey"`
	ProductName string    `gorm:"type:varchar(255);not null"`
	Description string    `gorm:"type:varchar(500);not null;default:''"`
	Stock       uint      `gorm:"not null;default:0"`
	Price       float64   `gorm:"type:decimal(10,2);not null;default:0"`
	Status      uint8     `gorm:"not null;default:1;index"`
	Version     uint      `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName 表名
func (Goods) TableName() string {
	return "goods"
}
