package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"seckill/internal/config"
)

var Client *redis.Client

func Init(cfg *config.RedisConfig) error {
	Client = redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx := context.Background()
	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect redis: %w", err)
	}

	return nil
}

func Close() error {
	if Client != nil {
		return Client.Close()
	}
	return nil
}

// FlushAll 清空所有数据
func FlushAll(ctx context.Context) error {
	return Client.FlushAll(ctx).Err()
}
