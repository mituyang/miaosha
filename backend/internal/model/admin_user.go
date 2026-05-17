package model

import "time"

const (
	AdminUserStatusDisabled uint8 = 0
	AdminUserStatusEnabled  uint8 = 1
)

// AdminUser 管理员模型
type AdminUser struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	Username  string    `gorm:"type:varchar(50);not null;uniqueIndex:uk_admin_username"`
	Password  string    `gorm:"type:varchar(255);not null"`
	Status    uint8     `gorm:"not null;default:1;index:idx_admin_status"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName 表名
func (AdminUser) TableName() string {
	return "admin_users"
}
