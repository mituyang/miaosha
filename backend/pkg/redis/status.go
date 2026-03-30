package redis

import (
	"context"

	redisLib "github.com/redis/go-redis/v9"
)

// SetGoodsOnSale 设置商品上下架状态
func SetGoodsOnSale(ctx context.Context, goodsID uint64, onSale bool) error {
	value := "0"
	if onSale {
		value = "1"
	}
	return Client.Set(ctx, GoodsStatusKey(goodsID), value, 0).Err()
}

// IsGoodsOnSale 查询商品是否上架，缓存缺失时按未上架处理
func IsGoodsOnSale(ctx context.Context, goodsID uint64) (bool, error) {
	value, err := Client.Get(ctx, GoodsStatusKey(goodsID)).Result()
	if err != nil {
		if err == redisLib.Nil {
			return false, nil
		}
		return false, err
	}
	return value == "1", nil
}

// ClearGoodsOnSale 清理商品上下架状态缓存
func ClearGoodsOnSale(ctx context.Context, goodsID uint64) error {
	return Client.Del(ctx, GoodsStatusKey(goodsID)).Err()
}

// SetUserEnabled 设置用户状态
func SetUserEnabled(ctx context.Context, userID uint64, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	return Client.Set(ctx, UserStatusKey(userID), value, 0).Err()
}

// IsUserEnabled 查询用户是否启用，缓存缺失时默认启用以兼容旧数据
func IsUserEnabled(ctx context.Context, userID uint64) (bool, error) {
	value, err := Client.Get(ctx, UserStatusKey(userID)).Result()
	if err != nil {
		if err == redisLib.Nil {
			return true, nil
		}
		return false, err
	}
	return value == "1", nil
}
