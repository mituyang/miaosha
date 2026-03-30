package repository

import (
	"strings"

	"seckill/internal/model"
	"seckill/pkg/database"
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// Create 创建用户
func (r *UserRepository) Create(user *model.User) error {
	return database.DB.Create(user).Error
}

// FindByUsername 根据用户名查找用户
func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := database.DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID 根据 ID 查询用户
func (r *UserRepository) GetByID(userID uint64) (*model.User, error) {
	var user model.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

type UserFilter struct {
	Keyword string
	Status  *uint8
}

type AdminUserItem struct {
	ID        uint64 `json:"id"`
	Username  string `json:"username"`
	Status    uint8  `json:"status"`
	CreatedAt string `json:"created_at"`
}

// List 查询用户列表
func (r *UserRepository) List(filter UserFilter) ([]AdminUserItem, error) {
	users, _, err := r.ListPage(filter, 1, 1000)
	return users, err
}

// ListPage 分页查询用户列表
func (r *UserRepository) ListPage(filter UserFilter, page, pageSize int) ([]AdminUserItem, int64, error) {
	var users []AdminUserItem
	query := database.DB.Table("users")

	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		query = query.Where("username LIKE ?", "%"+keyword+"%")
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Select("id, username, status, DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s') AS created_at").
		Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&users).Error
	return users, total, err
}

// UpdateStatus 更新用户状态
func (r *UserRepository) UpdateStatus(userID uint64, status uint8) error {
	return database.DB.Model(&model.User{}).Where("id = ?", userID).Update("status", status).Error
}

type UserStats struct {
	TotalUsers    int64 `json:"total_users"`
	EnabledUsers  int64 `json:"enabled_users"`
	DisabledUsers int64 `json:"disabled_users"`
}

// GetStats 查询用户统计
func (r *UserRepository) GetStats() (*UserStats, error) {
	var stats UserStats
	err := database.DB.Raw(`
		SELECT
			COUNT(*) AS total_users,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS enabled_users,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS disabled_users
		FROM users
	`, model.UserStatusEnabled, model.UserStatusDisabled).Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}
