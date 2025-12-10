package model

// Goods 商品模型
type Goods struct {
	ID          uint64  `gorm:"primaryKey"`
	ProductName string  `gorm:"type:varchar(255);not null"`
	Stock       uint    `gorm:"not null;default:0"`
	Price       float64 `gorm:"type:decimal(10,2);not null;default:0"`
	Version     uint    `gorm:"not null;default:0"`
}

// TableName 表名
func (Goods) TableName() string {
	return "goods"
}
