package database

import "fmt"

type schemaStatement struct {
	Table string
	Name  string
	Kind  string
	SQL   string
}

// EnsureSchema 补齐后台管理所需字段，兼容旧库结构
func EnsureSchema() error {
	statements := []schemaStatement{
		{Table: "seckill_activities", Name: "seckill_activities", Kind: "table", SQL: `
CREATE TABLE IF NOT EXISTS seckill_activities (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '活动ID',
    goods_id BIGINT UNSIGNED NOT NULL COMMENT '商品ID',
    title VARCHAR(255) NOT NULL COMMENT '活动标题',
    start_time DATETIME(3) NOT NULL COMMENT '开始时间',
    end_time DATETIME(3) NOT NULL COMMENT '结束时间',
    status TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '活动状态: 0-未开始, 1-进行中, 2-已结束, 3-停用',
    max_buy_limit INT UNSIGNED NOT NULL COMMENT '每用户最大购买数量',
    warmup_status TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '预热状态: 0-未预热, 1-已预热',
    is_default TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '是否默认活动: 0-否, 1-是',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_activity_goods_id (goods_id),
    KEY idx_activity_goods_default (goods_id, is_default),
    KEY idx_activity_status_time (status, start_time, end_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='秒杀活动表'`},
		{Table: "admin_users", Name: "admin_users", Kind: "table", SQL: `
CREATE TABLE IF NOT EXISTS admin_users (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '管理员ID',
    username VARCHAR(50) NOT NULL COMMENT '管理员账号',
    password VARCHAR(255) NOT NULL COMMENT '密码哈希',
    status TINYINT UNSIGNED NOT NULL DEFAULT 1 COMMENT '管理员状态: 0-禁用, 1-启用',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_admin_username (username),
    KEY idx_admin_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='管理员表'`},
		{Table: "goods", Name: "description", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN description VARCHAR(500) NOT NULL DEFAULT '' AFTER product_name"},
		{Table: "goods", Name: "status", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN status TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER price"},
		{Table: "goods", Name: "created_at", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER version"},
		{Table: "goods", Name: "updated_at", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER created_at"},
		{Table: "users", Name: "email", Kind: "column", SQL: "ALTER TABLE users ADD COLUMN email VARCHAR(255) NOT NULL DEFAULT '' COMMENT '邮箱' AFTER username"},
		{Table: "users", Name: "status", Kind: "column", SQL: "ALTER TABLE users ADD COLUMN status TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER password"},
		{Table: "seckill_activities", Name: "is_default", Kind: "column", SQL: "ALTER TABLE seckill_activities ADD COLUMN is_default TINYINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '是否默认活动: 0-否, 1-是' AFTER warmup_status"},
		{Table: "orders", Name: "activity_id", Kind: "column", SQL: "ALTER TABLE orders ADD COLUMN activity_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '秒杀活动ID' AFTER goods_id"},
		{Table: "goods", Name: "idx_goods_status", Kind: "index", SQL: "CREATE INDEX idx_goods_status ON goods (status)"},
		{Table: "users", Name: "idx_users_email", Kind: "index", SQL: "CREATE INDEX idx_users_email ON users (email)"},
		{Table: "users", Name: "idx_users_status", Kind: "index", SQL: "CREATE INDEX idx_users_status ON users (status)"},
		{Table: "users", Name: "idx_users_created_at", Kind: "index", SQL: "CREATE INDEX idx_users_created_at ON users (created_at)"},
		{Table: "users", Name: "idx_users_status_created_at", Kind: "index", SQL: "CREATE INDEX idx_users_status_created_at ON users (status, created_at)"},
		{Table: "seckill_activities", Name: "idx_activity_goods_default", Kind: "index", SQL: "CREATE INDEX idx_activity_goods_default ON seckill_activities (goods_id, is_default)"},
		{Table: "orders", Name: "idx_activity_id", Kind: "index", SQL: "CREATE INDEX idx_activity_id ON orders (activity_id)"},
		{Table: "orders", Name: "idx_orders_create_time", Kind: "index", SQL: "CREATE INDEX idx_orders_create_time ON orders (create_time)"},
		{Table: "orders", Name: "idx_orders_status_create_time", Kind: "index", SQL: "CREATE INDEX idx_orders_status_create_time ON orders (status, create_time)"},
	}

	for _, stmt := range statements {
		exists, err := schemaObjectExists(stmt.Table, stmt.Name, stmt.Kind)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := DB.Exec(stmt.SQL).Error; err != nil {
			return fmt.Errorf("apply schema migration failed: %w", err)
		}
	}

	return nil
}

func schemaObjectExists(table, name, kind string) (bool, error) {
	var count int64

	switch kind {
	case "table":
		err := DB.Raw(`
			SELECT COUNT(*)
			FROM INFORMATION_SCHEMA.TABLES
			WHERE TABLE_SCHEMA = DATABASE()
			  AND TABLE_NAME = ?
		`, table).Scan(&count).Error
		if err != nil {
			return false, fmt.Errorf("check table exists failed: %w", err)
		}
	case "column":
		err := DB.Raw(`
			SELECT COUNT(*)
			FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE()
			  AND TABLE_NAME = ?
			  AND COLUMN_NAME = ?
		`, table, name).Scan(&count).Error
		if err != nil {
			return false, fmt.Errorf("check column exists failed: %w", err)
		}
	case "index":
		err := DB.Raw(`
			SELECT COUNT(*)
			FROM INFORMATION_SCHEMA.STATISTICS
			WHERE TABLE_SCHEMA = DATABASE()
			  AND TABLE_NAME = ?
			  AND INDEX_NAME = ?
		`, table, name).Scan(&count).Error
		if err != nil {
			return false, fmt.Errorf("check index exists failed: %w", err)
		}
	default:
		return false, fmt.Errorf("unsupported schema object kind: %s", kind)
	}

	return count > 0, nil
}
