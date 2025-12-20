package database

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"seckill/internal/config"
)

var DB *gorm.DB

func Init(cfg *config.MySQLConfig) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// 连接池配置
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)

	// 连接生命周期配置（防止连接泄漏）
	connMaxLifetime := time.Hour // 默认1小时
	if cfg.ConnMaxLifetimeSec > 0 {
		connMaxLifetime = time.Duration(cfg.ConnMaxLifetimeSec) * time.Second
	}
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	connMaxIdleTime := 10 * time.Minute // 默认10分钟
	if cfg.ConnMaxIdleTimeSec > 0 {
		connMaxIdleTime = time.Duration(cfg.ConnMaxIdleTimeSec) * time.Second
	}
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	DB = db
	return nil
}

func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
