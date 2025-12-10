package redis

import (
	"context"
	"strconv"
)

// InitStock 初始化商品库存到 Redis (秒杀开始前调用)
func InitStock(ctx context.Context, goodsID uint64, stock int) error {
	stockKey := StockKey(goodsID)
	return Client.Set(ctx, stockKey, stock, 0).Err()
}

// GetStock 获取当前库存
func GetStock(ctx context.Context, goodsID uint64) (int, error) {
	stockKey := StockKey(goodsID)
	val, err := Client.Get(ctx, stockKey).Result()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}

// ClearSeckillData 清理秒杀数据 (活动结束后调用)
func ClearSeckillData(ctx context.Context, goodsID uint64) error {
	stockKey := StockKey(goodsID)
	boughtKey := BoughtKey(goodsID)
	return Client.Del(ctx, stockKey, boughtKey).Err()
}

// IncrStock 增加库存 (订单取消时返还)
func IncrStock(ctx context.Context, goodsID uint64) error {
	stockKey := StockKey(goodsID)
	return Client.Incr(ctx, stockKey).Err()
}

// ClearUserBought 清除用户购买记录 (订单取消时允许重新抢购)
func ClearUserBought(ctx context.Context, goodsID, userID uint64) error {
	boughtKey := BoughtKey(goodsID)
	return Client.SRem(ctx, boughtKey, userID).Err()
}
