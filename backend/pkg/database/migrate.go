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
		{Table: "goods", Name: "description", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN description VARCHAR(500) NOT NULL DEFAULT '' AFTER product_name"},
		{Table: "goods", Name: "status", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN status TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER price"},
		{Table: "goods", Name: "created_at", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER version"},
		{Table: "goods", Name: "updated_at", Kind: "column", SQL: "ALTER TABLE goods ADD COLUMN updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER created_at"},
		{Table: "users", Name: "status", Kind: "column", SQL: "ALTER TABLE users ADD COLUMN status TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER password"},
		{Table: "goods", Name: "idx_goods_status", Kind: "index", SQL: "CREATE INDEX idx_goods_status ON goods (status)"},
		{Table: "users", Name: "idx_users_status", Kind: "index", SQL: "CREATE INDEX idx_users_status ON users (status)"},
		{Table: "users", Name: "idx_users_created_at", Kind: "index", SQL: "CREATE INDEX idx_users_created_at ON users (created_at)"},
		{Table: "users", Name: "idx_users_status_created_at", Kind: "index", SQL: "CREATE INDEX idx_users_status_created_at ON users (status, created_at)"},
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
