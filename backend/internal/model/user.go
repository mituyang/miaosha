package model

import "time"

// User 用户模型
type User struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Username  string    `gorm:"type:varchar(50);not null;uniqueIndex:uk_username"`
	Password  string    `gorm:"type:varchar(255);not null"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName 表名
func (User) TableName() string {
	return "users"
}
