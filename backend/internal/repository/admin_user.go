package repository

import (
	"seckill/internal/model"
	"seckill/pkg/database"
)

type AdminUserRepository struct{}

func NewAdminUserRepository() *AdminUserRepository {
	return &AdminUserRepository{}
}

// Create 创建管理员
func (r *AdminUserRepository) Create(admin *model.AdminUser) error {
	return database.DB.Create(admin).Error
}

// FindByUsername 根据账号查询管理员
func (r *AdminUserRepository) FindByUsername(username string) (*model.AdminUser, error) {
	var admin model.AdminUser
	if err := database.DB.Where("username = ?", username).First(&admin).Error; err != nil {
		return nil, err
	}
	return &admin, nil
}

// UpdatePasswordAndStatus 更新密码和状态
func (r *AdminUserRepository) UpdatePasswordAndStatus(adminID uint64, password string, status uint8) error {
	return database.DB.Model(&model.AdminUser{}).Where("id = ?", adminID).Updates(map[string]interface{}{
		"password": password,
		"status":   status,
	}).Error
}
