package model

import "time"

const (
	UserStatusDisabled uint8 = 0
	UserStatusEnabled  uint8 = 1
)

// User 用户模型
type User struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Username  string    `gorm:"type:varchar(50);not null;uniqueIndex:uk_username"`
	Password  string    `gorm:"type:varchar(255);not null"`
	Status    uint8     `gorm:"not null;default:1;index"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName 表名
func (User) TableName() string {
	return "users"
}
